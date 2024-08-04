package internal

// The AlertsController is the heart of the frontend. It processes JSON coming
// from an MQTT topic, broadcasts SSEEvents through to the SSEBroacaster, and
// sends REST calls to the LLM.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/kwkoo/threat-detection-frontend/internal/prompts"
	"github.com/kwkoo/threat-detection-frontend/llamacpp"
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

func (s alertEvent) copy() alertEvent {
	d := alertEvent{
		timestamp: s.timestamp,
		prompt:    s.prompt,
	}
	if s.annotatedImage != nil {
		d.annotatedImage = make([]byte, len(s.annotatedImage))
		copy(d.annotatedImage, s.annotatedImage)
	}
	if s.rawImage != nil {
		d.rawImage = make([]byte, len(s.rawImage))
		copy(d.rawImage, s.rawImage)
	}
	return d
}

type AlertsController struct {
	eventsPaused       atomic.Bool
	sseCh              chan SSEEvent
	llavaClient        *llamacpp.Client
	prompts            *prompts.PromptsContainer
	threatAnalysisCl   *threatAnalysisClient
	openAIPrompt       string
	latestAlert        alertEvent
	latestAlertMux     sync.RWMutex
	imageAnalysis      AtomicString
	threatAnalysis     AtomicString
	llmCh              chan alertEvent
	saveModelResponses bool
}

// Ensure that ch is a buffered channel - if the channel is not buffered,
// sending events to this channel will fail
func NewAlertsController(ch chan SSEEvent, llavaURL, promptsFile, openAIModel, openAIPrompt, openAIURL string) *AlertsController {
	if cap(ch) < 1 {
		log.Fatal("SSEEvent channel cannot be unbuffered")
	}

	prompts, err := prompts.NewPromptsContainerFromFile(promptsFile)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("alerts controller initializing with llavaURL=%s, openAIModel=%s, openAIPrompt=%s, openAIURL=%s", llavaURL, openAIModel, openAIPrompt, openAIURL)

	if openAIURL == "" {
		log.Print("openAIURL is not set so we will not call it - will stream Llava responses to client")
	}

	llavaClient, err := llamacpp.NewClient(llavaURL, llamacpp.WithRequestTimeout(llmRequestTimeoutSeconds*time.Second))
	if err != nil {
		log.Fatalf("could not instantiate llava client: %v", err)
	}

	c := AlertsController{
		sseCh:            ch,
		llavaClient:      llavaClient,
		prompts:          prompts,
		threatAnalysisCl: newThreatAnalysisClient(openAIURL, openAIModel),
		openAIPrompt:     openAIPrompt,
		llmCh:            make(chan alertEvent, llmChannelSize),
	}
	return &c
}

func (controller *AlertsController) Shutdown() {
	if controller.llavaClient != nil {
		controller.llavaClient.Shutdown()
		controller.llavaClient = nil
	}
	if controller.threatAnalysisCl != nil {
		controller.threatAnalysisCl.Shutdown()
		controller.threatAnalysisCl = nil
	}
}

// Used to save mock data
func (controller *AlertsController) SaveModelResponses() {
	controller.saveModelResponses = true
	if controller.llavaClient != nil {
		if err := controller.llavaClient.SaveModelResponses(); err != nil {
			log.Printf("could not create mock llava output file: %v", err)
		}
	}

	if controller.threatAnalysisCl != nil {
		if err := controller.threatAnalysisCl.SaveModelResponses(); err != nil {
			log.Printf("could not create mock threat analysis output file: %v", err)
		}
	}
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

// ResumeEventsHandler is called when the user clicks on the "Resume Stream" button in the web UI
func (controller *AlertsController) ResumeEventsHandler(w http.ResponseWriter, r *http.Request) {
	log.Print("resuming event stream")
	controller.eventsPaused.Store(false)
	w.Write([]byte("OK"))
	controller.sseCh <- SSEEvent{
		EventType: "resume_events",
		Data:      nil,
	}
}

// CurrentStateHandler is called when the web UI is first loaded
func (controller *AlertsController) CurrentStateHandler(w http.ResponseWriter, r *http.Request) {
	latestAlert := controller.getLatestAlert()
	resp := struct {
		AnnotatedImage string `json:"annotated_image"`
		RawImage       string `json:"raw_image"`
		Timestamp      int64  `json:"timestamp"`
		Prompt         string `json:"prompt"`
		ImageAnalysis  string `json:"image_analysis"`
		ThreatAnalysis string `json:"threat_analysis"`
		EventsPaused   bool   `json:"events_paused"`
	}{
		AnnotatedImage: string(latestAlert.annotatedImage),
		RawImage:       string(latestAlert.rawImage),
		Timestamp:      latestAlert.timestamp,
		Prompt:         string(latestAlert.prompt.GetJSONBytes()),
		ImageAnalysis:  controller.imageAnalysis.Load(),
		ThreatAnalysis: controller.threatAnalysis.Load(),
		EventsPaused:   controller.eventsPaused.Load(),
	}
	json.NewEncoder(w).Encode(resp)
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

func (controller *AlertsController) broadcastImages(alert alertEvent) {
	controller.sseCh <- SSEEvent{
		EventType: "timestamp",
		Data:      []byte(strconv.FormatInt(alert.timestamp, 10)),
	}
	controller.sseCh <- SSEEvent{
		EventType: "annotated_image",
		Data:      alert.annotatedImage,
	}
	controller.sseCh <- SSEEvent{
		EventType: "raw_image",
		Data:      alert.rawImage,
	}
	controller.sseCh <- SSEEvent{
		EventType: "llm_request_start",
		Data:      nil,
	}
}

// Start this in a goroutine - cancel the Context to terminate the goroutine
func (controller *AlertsController) LLMChannelProcessor(ctx context.Context) {
	oldPromptID := -1
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-controller.llmCh:
			if !ok {
				log.Print("LLM channel processor could not read from LLM channel")
				return
			}
			promptID := event.prompt.ID

			// ignore incoming event if events are paused
			// make an exception for events with a new prompt because that
			// means the user has changed the prompt
			if oldPromptID == promptID && controller.eventsPaused.Load() {
				log.Print("ignoring alert event because events are paused")
				continue
			}

			// pause stream
			controller.eventsPaused.Store(true)

			oldPromptID = promptID
			controller.setLatestAlert(event)
			controller.broadcastImages(event)

			controller.sendToSSECh(SSEEvent{
				EventType: "prompt",
				Data:      []byte(event.prompt.GetJSONBytes()),
			})

			llavaReq := controller.llavaClient.NewCompletionPayload(event.prompt.Descriptive)
			if event.rawImage != nil {
				llavaReq.AddBase64Image(string(event.rawImage))
			}
			llavaReq.SetStream(true)
			controller.llavaRequest(ctx, *llavaReq)
			controller.sseCh <- SSEEvent{
				EventType: "pause_events",
				Data:      nil,
			}
		}
	}
}

func (controller *AlertsController) llavaRequest(parentCtx context.Context, payload llamacpp.CompletionPayload) {
	ch := make(chan llamacpp.CompletionResponse)
	controller.llavaClient.Completion(parentCtx, ch, payload)
	controller.sendToSSECh(SSEEvent{
		EventType: "llava_response_start",
		Data:      nil,
	})
	var b bytes.Buffer
	for resp := range ch {
		if resp.Err != nil {
			log.Printf("received error from llava: %v", resp.Err)
			continue
		}
		controller.sendToSSECh(SSEEvent{
			EventType: "llava_response",
			Data:      []byte(resp.Content),
		})
		b.WriteString(resp.Content)
	}
	controller.sendToSSECh(SSEEvent{
		EventType: "llava_response_stop",
		Data:      nil,
	})

	llmResponse := b.String()
	controller.imageAnalysis.Store(llmResponse)

	if controller.threatAnalysisCl != nil {
		// make request to OpenAI API here, passing it the prompt and the response from Llava
		if err := controller.threatAnalysisRequest(parentCtx, llmResponse); err != nil {
			log.Printf("error making openai request: %v", err)
			return
		}
		return
	}
}

func (controller *AlertsController) threatAnalysisRequest(ctx context.Context, text string) error {
	ctx, cancelRequest := context.WithCancel(ctx)
	ch := make(chan threatAnalysisResponse)
	go func() {
		controller.threatAnalysisCl.request(ctx, controller.openAIPrompt, text, ch)
	}()
	defer cancelRequest()
	defer controller.sendToSSECh(SSEEvent{
		EventType: "openai_response_stop",
		Data:      nil,
	})

	var llmResponse bytes.Buffer
	defer func(resp *bytes.Buffer) {
		controller.threatAnalysis.Store(resp.String())
	}(&llmResponse)

	controller.sendToSSECh(SSEEvent{
		EventType: "openai_response_start",
		Data:      nil,
	})
	for resp := range ch {
		if resp.err != nil {
			return resp.err
		}
		message := struct {
			Model    string `json:"model"`
			Response string `json:"response"`
			Done     bool   `json:"true"`
		}{
			Model:    "openai",
			Response: resp.content,
			Done:     resp.done,
		}
		marshaled, err := json.Marshal(&message)
		if err != nil {
			log.Printf("error converting openai stream response to json: %v", err)
			continue
		}
		llmResponse.WriteString(message.Response)
		controller.sendToSSECh((SSEEvent{
			EventType: "openai_response",
			Data:      marshaled,
		}))
	}
	return nil
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

	return controller.latestAlert.copy()
}

func (controller *AlertsController) setLatestAlert(newAlert alertEvent) {
	controller.latestAlertMux.Lock()
	controller.latestAlert = newAlert.copy()
	controller.latestAlertMux.Unlock()
}
