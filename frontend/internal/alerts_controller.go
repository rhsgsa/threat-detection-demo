package internal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

type alertMessage struct {
	AnnotatedImage string `json:"annotated_image"`
	RawImage       string `json:"raw_image"`
}

type llmRequest struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Images []string `json:"images"`
}

type AlertsController struct {
	sseCh          chan SSEEvent
	llmURL         string
	prompt         string
	annotatedImage []byte
	rawImage       []byte
	llmMux         sync.Mutex
	prompts        []string
	promptMux      sync.RWMutex
}

func NewAlertsController(ch chan SSEEvent, llmURL string) *AlertsController {
	c := AlertsController{
		sseCh:   ch,
		llmURL:  llmURL,
		prompt:  "Describe this picture",
		prompts: []string{"Describe this picture", "Is this person a threat?"},
	}
	return &c
}

// PromptHandler gets invoked when a REST call is made to list the available prompts or to set the prompt
func (controller *AlertsController) PromptHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		// todo: extract prompt from json body and set prompt here
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

	controller.makeLLMRequest([]byte(msg.AnnotatedImage), []byte(msg.RawImage))
}

func (controller *AlertsController) broadcastImages() {
	controller.sseCh <- SSEEvent{
		EventType: "annotated_image",
		Data:      controller.annotatedImage,
	}
	controller.sseCh <- SSEEvent{
		EventType: "raw_image",
		Data:      controller.rawImage,
	}
	controller.sseCh <- SSEEvent{
		EventType: "llm_request_start",
		Data:      nil,
	}
}

func (controller *AlertsController) makeLLMRequest(annotatedImage, rawImage []byte) {
	controller.llmMux.Lock()
	defer controller.llmMux.Unlock()
	if annotatedImage != nil {
		controller.annotatedImage = annotatedImage
	}
	if rawImage != nil {
		controller.rawImage = rawImage
	}
	controller.broadcastImages()
	prompt := controller.getPrompt()
	controller.sseCh <- SSEEvent{
		EventType: "prompt",
		Data:      []byte(prompt),
	}
	llmReq := llmRequest{
		Model:  "llava",
		Prompt: prompt,
		Images: []string{string(controller.rawImage)},
	}
	payload, err := json.Marshal(llmReq)
	if err != nil {
		log.Printf("error trying to marshal JSON for LLM request: %v", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, controller.llmURL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("error creating request to %s: %v", controller.llmURL, err)
		return
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("error making request to %s: %v", controller.llmURL, err)
		return
	}
	log.Printf("response status code %d", res.StatusCode)

	defer res.Body.Close()
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
	controller.sseCh <- SSEEvent{
		EventType: "llm_response_stop",
		Data:      nil,
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
