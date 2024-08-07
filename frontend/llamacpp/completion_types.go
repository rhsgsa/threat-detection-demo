package llamacpp

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/nfnt/resize"
)

const maxImageSize = 640

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
		payload.ImageData = []Image{{Data: s}}
	} else {
		payload.ImageData = append(payload.ImageData, Image{Data: s})
	}
}

func (payload *CompletionPayload) AddImage(b []byte) error {
	image, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("error decoding image: %w", err)
	}
	imageSize := image.Bounds().Size()
	if imageSize.X > maxImageSize || imageSize.Y > maxImageSize {
		var newWidth, newHeight uint
		if imageSize.X > imageSize.Y {
			newWidth = maxImageSize
		} else {
			newHeight = maxImageSize
		}
		resizedImage := resize.Resize(newWidth, newHeight, image, resize.Lanczos3)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, resizedImage, nil); err != nil {
			return fmt.Errorf("error converting resized image to jpeg: %w", err)
		}
		b = buf.Bytes()
	}

	payload.AddBase64Image(base64.StdEncoding.EncodeToString(b))
	return nil
}

func (payload *CompletionPayload) SetStops(s []string) {
	payload.Stop = s
}

func (payload *CompletionPayload) AddStop(s string) {
	if payload.Stop == nil {
		payload.Stop = []string{s}
	} else {
		payload.Stop = append(payload.Stop, s)
	}
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
