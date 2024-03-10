package internal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"

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
	controller.annotatedImage = []byte(msg.AnnotatedImage)
	controller.rawImage = []byte(msg.RawImage)
	controller.broadcastImages()

	body := makeLLMRequest(controller.llmURL, controller.prompt, msg.RawImage)
	if body == nil {
		return
	}
	defer body.Close()
	scanner := bufio.NewScanner(body)
	scanner.Split(bufio.ScanLines)
	waitForFirstLine := true
	for scanner.Scan() {
		if waitForFirstLine {
			controller.sseCh <- SSEEvent{
				EventType: "llm_response_start",
				Data:      []byte{},
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
		Data:      []byte{},
	}
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
		Data:      []byte{},
	}
}

func makeLLMRequest(llmURL, prompt, image string) io.ReadCloser {
	llmReq := llmRequest{
		Model:  "llava",
		Prompt: prompt,
		Images: []string{image},
	}
	payload, err := json.Marshal(llmReq)
	if err != nil {
		log.Printf("error trying to marshal JSON for LLM request: %v", err)
		return nil
	}

	req, err := http.NewRequest(http.MethodPost, llmURL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("error creating request to %s: %v", llmURL, err)
		return nil
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("error making request to %s: %v", llmURL, err)
		return nil
	}
	log.Printf("response status code %d", res.StatusCode)
	return res.Body
}
