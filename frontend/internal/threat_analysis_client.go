package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sashabaranov/go-openai"
)

const mockThreatAnalysisOutputFilename = "/tmp/threatanalysis.txt"

type threatAnalysisResponse struct {
	err     error
	content string
	done    bool
}

type threatAnalysisClient struct {
	client    *openai.Client
	model     string
	debugFile *os.File
}

func newThreatAnalysisClient(url string, model string) *threatAnalysisClient {
	if url == "" {
		return nil
	}
	config := openai.DefaultConfig("dummy")
	config.BaseURL = url
	client := openai.NewClientWithConfig(config)
	return &threatAnalysisClient{
		client: client,
		model:  model,
	}
}

// Invoke this function from a goroutine.
func (c *threatAnalysisClient) request(ctx context.Context, prompt string, text string, ch chan<- threatAnalysisResponse) {
	defer close(ch)
	req := openai.ChatCompletionRequest{
		Model:       c.model,
		Temperature: 0,
		N:           1,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: prompt + "\n\n" + text,
			},
		},
	}

	stream, err := c.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		ch <- threatAnalysisResponse{
			err: fmt.Errorf("error creating openai chat completion stream: %w", err),
		}
		return
	}
	defer stream.Close()
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			ch <- threatAnalysisResponse{
				err: fmt.Errorf("error receiving openai stream response: %w", err),
			}
			return
		}
		if c.debugFile != nil {
			json.NewEncoder(c.debugFile).Encode(resp)
		}
		for _, choice := range resp.Choices {
			ch <- threatAnalysisResponse{
				content: choice.Delta.Content,
				done:    choice.FinishReason == openai.FinishReasonStop,
			}
		}
	}
}

func (c *threatAnalysisClient) SaveModelResponses() error {
	f, err := os.Create(mockThreatAnalysisOutputFilename)
	if err != nil {
		return err
	}
	c.debugFile = f
	return nil
}

func (c *threatAnalysisClient) Shutdown() {
	if c.debugFile == nil {
		return
	}
	c.debugFile.Close()
	c.debugFile = nil
}
