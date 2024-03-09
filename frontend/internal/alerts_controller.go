package internal

import (
	"encoding/json"
	"log"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

type alertMessage struct {
	AnnotatedImage string `json:"annotated_image"`
	RawImage       string `json:"raw_image"`
}

type AlertsController struct {
	alertsCh chan SSEEvent
}

func NewAlertsController(ch chan SSEEvent) *AlertsController {
	c := AlertsController{
		alertsCh: ch,
	}
	return &c
}

func (controller *AlertsController) AlertsHandler(client MQTT.Client, mqttMessage MQTT.Message) {
	var msg alertMessage
	if err := json.Unmarshal(mqttMessage.Payload(), &msg); err != nil {
		log.Printf("error trying to unmarshal alert MQTT message: %v", err)
		return
	}
	controller.alertsCh <- SSEEvent{
		EventType: "annotated_image",
		Data:      []byte(msg.AnnotatedImage),
	}
	controller.alertsCh <- SSEEvent{
		EventType: "raw_image",
		Data:      []byte(msg.RawImage),
	}
}
