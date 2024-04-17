package internal

// The AlertsController is the heart of the frontend. It processes JSON coming
// from an MQTT topic, broadcasts SSEEvents through to the SSEBroacaster, and
// sends REST calls to the LLM.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/kwkoo/threat-detection-frontend/internal/prompts"
	"github.com/sashabaranov/go-openai"
)

const llmRequestTimeoutSeconds = 60
const llmChannelSize = 3

// Alert coming from the image-acquirer via MQTT
type alertMQTT struct {
	AnnotatedImage string `json:"annotated_image"`
	RawImage       string `json:"raw_image"`
	Timestamp      int64  `json:"timestamp"`
}

// Alert going to the browsers via SSE
type alertEvent struct {
	annotatedImage []byte
	rawImage       []byte
	timestamp      int64
	prompt         prompts.PromptItem
}

type AlertsController struct {
	sseCh          chan SSEEvent
	ollamaURL      string
	ollamaModel    string
	keepAlive      string
	prompts        *prompts.PromptsContainer
	openAIModel    string
	openAIPrompt   string
	openAIURL      string
	latestAlert    alertEvent
	latestAlertMux sync.RWMutex
	llmCh          chan alertEvent
}

// Ensure that ch is a buffered channel - if the channel is not buffered,
// sending events to this channel will fail
func NewAlertsController(ch chan SSEEvent, ollamaURL, ollamaModel, keepAlive, promptsFile, openAIModel, openAIPrompt, openAIURL string) *AlertsController {
	if cap(ch) < 1 {
		log.Fatal("SSEEvent channel cannot be unbuffered")
	}

	prompts, err := prompts.NewPromptsContainerFromFile(promptsFile)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("alerts controller initializing with ollamaURL=%s, ollamaModel=%s, keepAlive=%s, openAIModel=%s, openAIPrompt=%s, openAIURL=%s", ollamaURL, ollamaModel, keepAlive, openAIModel, openAIPrompt, openAIURL)

	if openAIURL == "" {
		log.Print("openAIURL is not set so we will not call it - will stream Ollama responses to client")
	}

	c := AlertsController{
		sseCh:        ch,
		ollamaURL:    ollamaURL,
		ollamaModel:  ollamaModel,
		keepAlive:    keepAlive,
		prompts:      prompts,
		openAIModel:  openAIModel,
		openAIPrompt: openAIPrompt,
		openAIURL:    openAIURL,
		llmCh:        make(chan alertEvent, llmChannelSize),
	}
	return &c
}

// PromptHandler gets invoked when a REST call is made to list the available prompts or to set the prompt
func (controller *AlertsController) PromptHandler(w http.ResponseWriter, r *http.Request) {
	// get prompts
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		controller.prompts.StreamShortPrompts(w)
		return
	}

	// set prompts
	in := struct {
		ID *int `json:"id"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, fmt.Sprintf("error decoding HTTP request body for prompt endpoint: %v", err), http.StatusPreconditionFailed)
		return
	}
	if in.ID == nil {
		http.Error(w, `required field "id" missing`, http.StatusPreconditionRequired)
		return
	}
	newID := *in.ID
	if err := controller.prompts.SetSelectedPrompt(newID); err != nil {
		http.Error(w, fmt.Sprintf("error setting prompt to %d: %v", newID, err), http.StatusPreconditionFailed)
		return
	}
	event := controller.getLatestAlert()

	// if we don't have a latest alert, we don't have to pass it to the LLMChannelProcessor
	if event.annotatedImage == nil && event.rawImage == nil && event.timestamp == 0 {
		http.Error(w, "prompt set - but we do not have any pending alerts", http.StatusFailedDependency)
		return
	}
	selectedPrompt, err := controller.prompts.GetSelectedPromptItem()
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting selected prompt: %v", err), http.StatusPreconditionFailed)
		return
	}
	if selectedPrompt == nil {
		http.Error(w, "could not get selected prompt", http.StatusPreconditionFailed)
		return
	}
	event.prompt = *selectedPrompt
	select {
	case controller.llmCh <- event:
		w.Write([]byte(fmt.Sprintf("prompt set to %d", newID)))
	default:
		msg := "LLM channel is full"
		log.Print(msg)
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

// MQTTHandler gets invoked when a message is received on the alerts MQTT topic
func (controller *AlertsController) MQTTHandler(_ MQTT.Client, mqttMessage MQTT.Message) {
	var msg alertMQTT
	if err := json.Unmarshal(mqttMessage.Payload(), &msg); err != nil {
		log.Printf("error trying to unmarshal alert MQTT message: %v", err)
		return
	}

	log.Print("received alert MQTT message")

	currentPrompt, err := controller.prompts.GetSelectedPromptItem()
	if err != nil {
		log.Printf("could not get currently selected prompt: %v", err)
		return
	}
	if currentPrompt == nil {
		log.Print("could not get currently selected prompt")
		return
	}
	event := alertEvent{
		annotatedImage: []byte(msg.AnnotatedImage),
		rawImage:       []byte(msg.RawImage),
		timestamp:      msg.Timestamp,
		prompt:         *currentPrompt,
	}

	select {
	case controller.llmCh <- event:
		log.Print("added alertEvent to LLM channel")
	default:
		msg := "LLM channel is full"
		log.Print(msg)
	}
}

// REST endpoint that returns the size of the output channels
func (controller *AlertsController) StatusHandler(w http.ResponseWriter, r *http.Request) {
	status := struct {
		SSEChannel int `json:"sse_channel"`
		LLMChannel int `json:"llm_channel"`
	}{
		SSEChannel: len(controller.sseCh),
		LLMChannel: len(controller.llmCh),
	}
	json.NewEncoder(w).Encode(&status)
}

func (controller *AlertsController) broadcastImages() {
	latestAlert := controller.getLatestAlert()
	controller.sseCh <- SSEEvent{
		EventType: "timestamp",
		Data:      []byte(strconv.FormatInt(latestAlert.timestamp, 10)),
	}
	controller.sseCh <- SSEEvent{
		EventType: "annotated_image",
		Data:      latestAlert.annotatedImage,
	}
	controller.sseCh <- SSEEvent{
		EventType: "raw_image",
		Data:      latestAlert.rawImage,
	}
	controller.sseCh <- SSEEvent{
		EventType: "llm_request_start",
		Data:      nil,
	}
}

// Start this in a goroutine - cancel the Context to terminate the goroutine
func (controller *AlertsController) LLMChannelProcessor(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-controller.llmCh:
			if !ok {
				log.Print("LLM channel processor could not read from LLM channel")
				return
			}
			controller.setLatestAlert(event)
			controller.broadcastImages()

			controller.sendToSSECh(SSEEvent{
				EventType: "prompt",
				Data:      []byte(event.prompt.GetSSEBytes()),
			})

			ollamaReq := struct {
				Model     string   `json:"model"`
				KeepAlive string   `json:"keep_alive"`
				Stream    bool     `json:"stream"`
				Prompt    string   `json:"prompt"`
				Images    []string `json:"images,omitempty"`
			}{
				Model:     controller.ollamaModel,
				KeepAlive: controller.keepAlive,
				Stream:    controller.openAIURL == "",
				Prompt:    event.prompt.Descriptive,
			}
			if event.rawImage != nil {
				ollamaReq.Images = []string{string(event.rawImage)}
			}

			payload, err := json.Marshal(ollamaReq)
			if err != nil {
				log.Printf("error trying to marshal JSON for LLM request: %v", err)
				continue
			}
			controller.ollamaRequest(ctx, payload)
		}
	}
}

func (controller *AlertsController) ollamaRequest(parentCtx context.Context, payload []byte) {
	ctx, cancel := context.WithTimeout(parentCtx, llmRequestTimeoutSeconds*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controller.ollamaURL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("error creating request to %s: %v", controller.ollamaURL, err)
		return
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("error making request to %s: %v", controller.ollamaURL, err)
		return
	}
	log.Printf("LLM response status code %d", res.StatusCode)
	if res.StatusCode != 200 {
		log.Print("skipping processing of LLM response")
		return
	}
	defer res.Body.Close()

	if controller.openAIURL != "" {
		// parse res.Body as JSON - grab response field
		ollamaResponse := struct {
			Response string `json:"response"`
		}{}
		if err := json.NewDecoder(res.Body).Decode(&ollamaResponse); err != nil {
			log.Printf("error trying to decode ollama response: %v", err)
			return
		}

		// make request to OpenAI API here, passing it the prompt and the response from Ollama
		if err := controller.openAIRequest(parentCtx, ollamaResponse.Response); err != nil {
			log.Printf("error making openai request: %v", err)
			return
		}
		return
	}

	// openAIURL not defined - stream Ollama responses to SSE clients
	scanner := bufio.NewScanner(res.Body)
	scanner.Split(bufio.ScanLines)
	waitForFirstLine := true
	for scanner.Scan() {
		if waitForFirstLine {
			controller.sendToSSECh(SSEEvent{
				EventType: "llm_response_start",
				Data:      nil,
			})
			waitForFirstLine = false
		}
		controller.sendToSSECh(SSEEvent{
			EventType: "llm_response",
			Data:      []byte(scanner.Text()),
		})
	}
	controller.sendToSSECh(SSEEvent{
		EventType: "llm_response_stop",
		Data:      nil,
	})
}

func (controller *AlertsController) openAIRequest(ctx context.Context, text string) error {
	config := openai.DefaultConfig("dummy")
	config.BaseURL = controller.openAIURL
	client := openai.NewClientWithConfig(config)
	req := openai.ChatCompletionRequest{
		Model:       controller.openAIModel,
		Temperature: 0,
		N:           1,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "system",
				Content: "You are in charge of monitoring security camera feeds - your job is to determine if the situation on camera constitutes a threat. Do not repeat yourself",
			},
			{
				Role:    "user",
				Content: controller.openAIPrompt + "\n\n" + text,
			},
		},
	}
	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return fmt.Errorf("error creating openai chat completion stream: %w", err)
	}
	defer stream.Close()
	var buf bytes.Buffer
	isFirstLine := true
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error receiving openai stream response: %w", err)
		}
		for _, choice := range resp.Choices {
			buf.Reset()
			message := struct {
				Response string `json:"response"`
			}{
				Response: choice.Delta.Content,
			}
			if err := json.NewEncoder(&buf).Encode(&message); err != nil {
				log.Printf("error converting openai stream response to json: %v", err)
				continue
			}
			if isFirstLine {
				isFirstLine = false
				defer controller.sendToSSECh(SSEEvent{
					EventType: "llm_response_stop",
					Data:      nil,
				})
				controller.sendToSSECh(SSEEvent{
					EventType: "llm_response_start",
					Data:      nil,
				})
			}
			controller.sendToSSECh((SSEEvent{
				EventType: "llm_response",
				Data:      buf.Bytes(),
			}))
		}
	}
}

func (controller *AlertsController) sendToSSECh(event SSEEvent) error {
	select {
	case controller.sseCh <- event:
		return nil
	default:
		msg := fmt.Sprintf("SSE channel is full - could not send %s", event.EventType)
		log.Print(msg)
		return errors.New(msg)
	}
}

func (controller *AlertsController) getLatestAlert() alertEvent {
	controller.latestAlertMux.RLock()
	defer controller.latestAlertMux.RUnlock()

	if controller.latestAlert.annotatedImage == nil && controller.latestAlert.rawImage == nil && controller.latestAlert.timestamp == 0 {
		return alertEvent{}
	}

	dup := alertEvent{
		timestamp: controller.latestAlert.timestamp,
		prompt:    controller.latestAlert.prompt,
	}
	if controller.latestAlert.annotatedImage != nil {
		dup.annotatedImage = make([]byte, len(controller.latestAlert.annotatedImage))
		copy(dup.annotatedImage, controller.latestAlert.annotatedImage)
	}
	if controller.latestAlert.rawImage != nil {
		dup.rawImage = make([]byte, len(controller.latestAlert.rawImage))
		copy(dup.rawImage, controller.latestAlert.rawImage)
	}

	return dup
}

func (controller *AlertsController) setLatestAlert(newAlert alertEvent) {
	controller.latestAlertMux.Lock()
	controller.latestAlert = newAlert
	controller.latestAlertMux.Unlock()
}
