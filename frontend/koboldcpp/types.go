package koboldcpp

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/nfnt/resize"
)

const maxImageSize = 320

type Payload struct {
	N                int      `json:"n"`
	MaxContextLength int      `json:"max_context_length"`
	MaxLength        int      `json:"max_length"`
	Temperature      float32  `json:"temperature"`
	TopP             float32  `json:"top_p"`
	TopK             int      `json:"top_k"`
	Memory           string   `json:"memory"`
	TrimStop         bool     `json:"trim_stop"`
	Prompt           string   `json:"prompt"`
	RepPen           float32  `json:"rep_pen"`
	RepPenRange      int      `json:"rep_pen_range"`
	RepPenSlope      float32  `json:"rep_pen_slope"`
	Stop             []string `json:"stop_sequence,omitempty"`
	Images           []string `json:"images,omitempty"`
}

func NewPayload(prompt string) *Payload {
	payload := &Payload{
		N:                1,
		MaxContextLength: 4096,
		MaxLength:        200,
		TopP:             0.92,
		TopK:             100,
		Prompt:           prompt,
		Temperature:      0.8,
		TrimStop:         true,
		RepPen:           1.2,
		RepPenRange:      320,
		RepPenSlope:      0.7,
	}
	return payload
}

func (payload *Payload) AddBase64Image(s string) {
	if payload.Images == nil {
		payload.Images = []string{s}
	} else {
		payload.Images = append(payload.Images, s)
	}
}

func (payload *Payload) AddBase64ImageWithResize(s string) error {
	if s == "" {
		return errors.New("could not resize empty image")
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return fmt.Errorf("error decoding base64 image: %w", err)
	}
	return payload.AddImage(b)
}

func (payload *Payload) AddImage(b []byte) error {
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

func (payload *Payload) SetStops(s []string) {
	payload.Stop = s
}

func (payload *Payload) AddStop(s string) {
	if payload.Stop == nil {
		payload.Stop = []string{s}
	} else {
		payload.Stop = append(payload.Stop, s)
	}
}

func (payload *Payload) SetTemperature(t float32) {
	payload.Temperature = t
}

func (payload *Payload) SetMaxContextLength(v int) {
	payload.MaxContextLength = v
}

func (payload *Payload) SetMaxLength(v int) {
	payload.MaxLength = v
}

type Response struct {
	Err     error
	Content string
}
