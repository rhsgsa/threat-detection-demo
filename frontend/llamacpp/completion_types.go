package llamacpp

import "encoding/base64"

type CompletionPayload struct {
	Prompt      string   `json:"prompt"`
	ImageData   []Image  `json:"image_data,omitempty"`
	Stop        []string `json:"stop,omitempty"`
	Stream      bool     `json:"stream"`
	Temperature float32  `json:"temperature"`
}

type Image struct {
	Data string `json:"data"`
}

// This should usually not be called directly since it just creates a basic
// struct - most users should call Client.NewCompletionPayload() instead.
func NewCompletionPayload(prompt string) *CompletionPayload {
	payload := &CompletionPayload{
		Prompt:      prompt,
		Temperature: 0.8,
	}
	return payload
}

func (payload *CompletionPayload) AddBase64Image(s string) {
	if payload.ImageData == nil {
		payload.ImageData = make([]Image, 1)
	}
	payload.ImageData = append(payload.ImageData, Image{Data: s})
}

func (payload *CompletionPayload) AddImage(b []byte) {
	payload.AddBase64Image(base64.StdEncoding.EncodeToString(b))
}

func (payload *CompletionPayload) SetStops(s []string) {
	payload.Stop = s
}

func (payload *CompletionPayload) AddStop(s string) {
	if payload.Stop == nil {
		payload.Stop = make([]string, 1)
	}
	payload.Stop = append(payload.Stop, s)
}

func (payload *CompletionPayload) SetStream(b bool) {
	payload.Stream = b
}

func (payload *CompletionPayload) SetTemperature(t float32) {
	payload.Temperature = t
}

type CompletionResponse struct {
	Err     error
	Content string
}
