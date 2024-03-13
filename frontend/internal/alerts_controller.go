package internal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

const llmRequestTimeoutSeconds = 30
const llmChannelSize = 20

// Alert coming from the image-acquirer via MQTT
type alertMQTT struct {
	AnnotatedImage string `json:"annotated_image"`
	RawImage       string `json:"raw_image"`
	Timestamp      int64  `json:"timestamp"`
	Prompt         string `json:"prompt"`
}

// Alert going to the browsers via SSE
type alertEvent struct {
	annotatedImage []byte
	rawImage       []byte
	timestamp      int64
	prompt         string
}

type AlertsController struct {
	sseCh          chan SSEEvent
	llmURL         string
	prompt         string
	latestAlert    alertEvent
	latestAlertMux sync.RWMutex
	prompts        []string
	promptMux      sync.RWMutex
	llmCh          chan alertEvent
}

// Ensure that ch is a buffered channel - if the channel is not buffered,
// sending events to this channel will fail
func NewAlertsController(ch chan SSEEvent, llmURL, promptsFile string) *AlertsController {
	if cap(ch) < 1 {
		log.Fatal("SSEEvent channel cannot be unbuffered")
	}

	var prompts []string
	if promptsFile == "" {
		log.Print("no prompts file provided - will use hardcoded prompts")
		prompts = []string{"Please describe this image", "Is this person a threat?"}
	} else {
		var err error
		prompts, err = readLinesFromFile(promptsFile)
		if err != nil {
			log.Fatalf("could not open prompt file %s: %v", promptsFile, err)
		}
		log.Printf("loaded %d prompts from %s", len(prompts), promptsFile)
	}
	if len(prompts) == 0 {
		log.Fatalf("no prompts defined")
	}
	c := AlertsController{
		sseCh:   ch,
		llmURL:  llmURL,
		prompt:  prompts[0],
		prompts: prompts,
		llmCh:   make(chan alertEvent, llmChannelSize),
	}
	return &c
}

func readLinesFromFile(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, nil
}

// PromptHandler gets invoked when a REST call is made to list the available prompts or to set the prompt
func (controller *AlertsController) PromptHandler(w http.ResponseWriter, r *http.Request) {
	// get prompts
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		streamResponse(w, controller.prompts)
		return
	}

	// set prompts
	in := struct {
		Prompt string `json:"prompt"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, fmt.Sprintf("error decoding HTTP request body for prompt endpoint: %v", err), http.StatusInternalServerError)
		return
	}
	controller.setPrompt(in.Prompt)
	event := controller.getLatestAlert()
	event.prompt = in.Prompt
	select {
	case controller.llmCh <- event:
		w.Write([]byte("OK"))
	default:
		msg := "LLM channel is full"
		log.Print(msg)
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

// AlertsHandler gets invoked when a message is received on the alerts MQTT topic
func (controller *AlertsController) AlertsHandler(client MQTT.Client, mqttMessage MQTT.Message) {
	var msg alertMQTT
	if err := json.Unmarshal(mqttMessage.Payload(), &msg); err != nil {
		log.Printf("error trying to unmarshal alert MQTT message: %v", err)
		return
	}

	log.Print("received alert MQTT message")

	event := alertEvent{
		annotatedImage: []byte(msg.AnnotatedImage),
		rawImage:       []byte(msg.RawImage),
		timestamp:      msg.Timestamp,
		prompt:         controller.getPrompt(),
	}

	select {
	case controller.llmCh <- event:
		log.Print("added alertEvent to LLM channel")
	default:
		msg := "LLM channel is full"
		log.Print(msg)
	}

}

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
				Data:      []byte(event.prompt),
			})

			llmReq := struct {
				Model  string   `json:"model"`
				Prompt string   `json:"prompt"`
				Images []string `json:"images"`
			}{
				Model:  "llava",
				Prompt: event.prompt,
				Images: []string{string(event.rawImage)},
			}

			payload, err := json.Marshal(llmReq)
			if err != nil {
				log.Printf("error trying to marshal JSON for LLM request: %v", err)
				continue
			}
			controller.llmRequest(ctx, payload)
		}
	}
}

func (controller *AlertsController) llmRequest(ctx context.Context, payload []byte) {
	ctx, cancel := context.WithTimeout(ctx, llmRequestTimeoutSeconds*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controller.llmURL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("error creating request to %s: %v", controller.llmURL, err)
		return
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("error making request to %s: %v", controller.llmURL, err)
		return
	}
	log.Printf("LLM response status code %d", res.StatusCode)
	if res.StatusCode != 200 {
		log.Print("skipping processing of LLM response")
		return
	}
	defer res.Body.Close()

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

func (controller *AlertsController) getPrompt() string {
	controller.promptMux.RLock()
	defer controller.promptMux.RUnlock()
	return controller.prompt
}

func (controller *AlertsController) setPrompt(newprompt string) {
	controller.promptMux.Lock()
	controller.prompt = newprompt
	controller.promptMux.Unlock()
}

func (controller *AlertsController) getLatestAlert() alertEvent {
	controller.latestAlertMux.RLock()
	dup := alertEvent{
		annotatedImage: make([]byte, len(controller.latestAlert.annotatedImage)),
		rawImage:       make([]byte, len(controller.latestAlert.rawImage)),
		timestamp:      controller.latestAlert.timestamp,
		prompt:         controller.latestAlert.prompt,
	}
	copy(dup.annotatedImage, controller.latestAlert.annotatedImage)
	copy(dup.rawImage, controller.latestAlert.rawImage)
	controller.latestAlertMux.RUnlock()
	return dup
}

func (controller *AlertsController) setLatestAlert(newAlert alertEvent) {
	controller.latestAlertMux.Lock()
	controller.latestAlert = newAlert
	controller.latestAlertMux.Unlock()
}

func streamResponse(w http.ResponseWriter, v any) {
	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("error converting response to JSON: %v", err)
	}
}
