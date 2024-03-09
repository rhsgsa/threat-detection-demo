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
	sseCh  chan SSEEvent
	llmURL string
	prompt string
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
	controller.sseCh <- SSEEvent{
		EventType: "annotated_image",
		Data:      []byte(msg.AnnotatedImage),
	}
	controller.sseCh <- SSEEvent{
		EventType: "raw_image",
		Data:      []byte(msg.RawImage),
	}
	controller.sseCh <- SSEEvent{
		EventType: "starting_llm_request",
		Data:      []byte{},
	}

	body := makeLLMRequest(controller.llmURL, controller.prompt, msg.RawImage)
	if body == nil {
		return
	}
	defer body.Close()
	scanner := bufio.NewScanner(body)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		controller.sseCh <- SSEEvent{
			EventType: "llm_response",
			Data:      []byte(scanner.Text()),
		}
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

/*
func publishLLMRequest(client MQTT.Client, topic, prompt, image string) error {
	llmReq := llmRequest{
		Model:  "llava",
		Prompt: prompt,
		Images: []string{image},
	}
	req, err := json.Marshal(llmReq)
	if err != nil {
		log.Printf("error trying to marshal JSON for LLM request: %v", err)
		return err
	}
	token := client.Publish(topic, 1, false, req)
	<-token.Done()
	if token.Error() != nil {
		log.Printf("error trying to publish to responses topic: %v", token.Error())
		return token.Error()
	}
	return nil
}

func (controller *AlertsController) LLMResponseHandler(client MQTT.Client, mqttMessage MQTT.Message) {
	type llmResp struct {
		Error string `json:"error"`
		Done  bool   `json:"done"`
	}
	payload := mqttMessage.Payload()
	var resp llmResp
	if err := json.Unmarshal(payload, &resp); err != nil {
		log.Printf("error trying to unmarshal LLM response: %v", err)
		return
	}
	if resp.Error != "" || resp.Done {
		controller.mux.Unlock()
	}

	controller.sseCh <- SSEEvent{
		EventType: "llm_response",
		Data:      mqttMessage.Payload(),
	}
}
*/
