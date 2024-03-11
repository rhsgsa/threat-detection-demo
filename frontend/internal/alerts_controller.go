package internal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

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

func NewAlertsController(ch chan SSEEvent, llmURL string) *AlertsController {
	c := AlertsController{
		sseCh:   ch,
		llmURL:  llmURL,
		prompt:  "Describe this picture",
		prompts: []string{"Describe this picture", "Is this person a threat?"},
		llmCh:   make(chan alertEvent, 10),
	}
	return &c
}

func (controller *AlertsController) Shutdown() {
	close(controller.llmCh)
}

// PromptHandler gets invoked when a REST call is made to list the available prompts or to set the prompt
func (controller *AlertsController) PromptHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
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
		controller.llmCh <- event
		w.Write([]byte("OK"))
		return
	}

	streamResponse(w, controller.prompts)
}

// AlertsHandler gets invoked when a message is received on the alerts MQTT topic
func (controller *AlertsController) AlertsHandler(client MQTT.Client, mqttMessage MQTT.Message) {
	var msg alertMQTT
	if err := json.Unmarshal(mqttMessage.Payload(), &msg); err != nil {
		log.Printf("error trying to unmarshal alert MQTT message: %v", err)
		return
	}

	event := alertEvent{
		annotatedImage: []byte(msg.AnnotatedImage),
		rawImage:       []byte(msg.RawImage),
		timestamp:      msg.Timestamp,
		prompt:         controller.getPrompt(),
	}
	controller.llmCh <- event
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

// Start this in a goroutine - close the llmCh to exit the goroutine
func (controller *AlertsController) LLMRequester() {
	for event := range controller.llmCh {
		controller.setLatestAlert(event)
		controller.broadcastImages()

		controller.sseCh <- SSEEvent{
			EventType: "prompt",
			Data:      []byte(event.prompt),
		}

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

		req, err := http.NewRequest(http.MethodPost, controller.llmURL, bytes.NewReader(payload))
		if err != nil {
			log.Printf("error creating request to %s: %v", controller.llmURL, err)
			continue
		}

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("error making request to %s: %v", controller.llmURL, err)
			continue
		}
		log.Printf("response status code %d", res.StatusCode)

		scanner := bufio.NewScanner(res.Body)
		scanner.Split(bufio.ScanLines)
		waitForFirstLine := true
		for scanner.Scan() {
			if waitForFirstLine {
				controller.sseCh <- SSEEvent{
					EventType: "llm_response_start",
					Data:      nil,
				}
				waitForFirstLine = false
			}
			controller.sseCh <- SSEEvent{
				EventType: "llm_response",
				Data:      []byte(scanner.Text()),
			}
		}
		res.Body.Close()
		controller.sseCh <- SSEEvent{
			EventType: "llm_response_stop",
			Data:      nil,
		}
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
