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

type alertMessage struct {
	AnnotatedImage string `json:"annotated_image"`
	RawImage       string `json:"raw_image"`
	Timestamp      int64  `json:"timestamp"`
	Prompt         string `json:"prompt"`
}

type AlertsController struct {
	sseCh       chan SSEEvent
	llmURL      string
	prompt      string
	latestAlert struct {
		annotatedImage []byte
		rawImage       []byte
		timestamp      int64
	}
	prompts   []string
	promptMux sync.RWMutex
	llmCh     chan alertMessage
}

func NewAlertsController(ch chan SSEEvent, llmURL string) *AlertsController {
	c := AlertsController{
		sseCh:   ch,
		llmURL:  llmURL,
		prompt:  "Describe this picture",
		prompts: []string{"Describe this picture", "Is this person a threat?"},
		llmCh:   make(chan alertMessage, 10),
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
		w.Write([]byte("OK"))
		return
	}

	streamResponse(w, controller.prompts)
}

// AlertsHandler gets invoked when a message is received on the alerts MQTT topic
func (controller *AlertsController) AlertsHandler(client MQTT.Client, mqttMessage MQTT.Message) {
	var msg alertMessage
	if err := json.Unmarshal(mqttMessage.Payload(), &msg); err != nil {
		log.Printf("error trying to unmarshal alert MQTT message: %v", err)
		return
	}

	msg.Prompt = controller.getPrompt()
	controller.llmCh <- msg
}

func (controller *AlertsController) broadcastImages() {
	controller.sseCh <- SSEEvent{
		EventType: "timestamp",
		Data:      []byte(strconv.FormatInt(controller.latestAlert.timestamp, 10)),
	}
	controller.sseCh <- SSEEvent{
		EventType: "annotated_image",
		Data:      controller.latestAlert.annotatedImage,
	}
	controller.sseCh <- SSEEvent{
		EventType: "raw_image",
		Data:      controller.latestAlert.rawImage,
	}
	controller.sseCh <- SSEEvent{
		EventType: "llm_request_start",
		Data:      nil,
	}
}

// Start this in a goroutine - close the llmCh to exit the goroutine
func (controller *AlertsController) LLMRequester() {
	for alertMsg := range controller.llmCh {
		if alertMsg.AnnotatedImage != "" {
			controller.latestAlert.annotatedImage = []byte(alertMsg.AnnotatedImage)
		}
		if alertMsg.RawImage != "" {
			controller.latestAlert.rawImage = []byte(alertMsg.RawImage)
		}
		controller.latestAlert.timestamp = alertMsg.Timestamp
		controller.broadcastImages()

		controller.sseCh <- SSEEvent{
			EventType: "prompt",
			Data:      []byte(alertMsg.Prompt),
		}

		llmReq := struct {
			Model  string   `json:"model"`
			Prompt string   `json:"prompt"`
			Images []string `json:"images"`
		}{
			Model:  "llava",
			Prompt: alertMsg.Prompt,
			Images: []string{alertMsg.RawImage},
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

func streamResponse(w http.ResponseWriter, v any) {
	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("error converting response to JSON: %v", err)
	}
}
