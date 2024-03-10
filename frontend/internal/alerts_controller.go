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
}

func NewAlertsController(ch chan SSEEvent, llmURL string) *AlertsController {
	c := AlertsController{
		sseCh:  ch,
		llmURL: llmURL,
		prompt: "Describe this picture",
	}
	return &c
}

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
	llmReq := llmRequest{
		Model:  "llava",
		Prompt: controller.prompt,
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
