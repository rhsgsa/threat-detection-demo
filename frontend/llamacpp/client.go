package llamacpp

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"
)

type Client struct {
	url            *url.URL
	requestTimeout time.Duration
	promptPrefix   string
	promptSuffix   string
	stop           []string
	keepAlive      time.Duration
	debugFile      *os.File
}

// NewClient instantiates a new Client struct.
//
// The Client struct is configured using the functional options pattern.
// https://golang.cafe/blog/golang-functional-options-pattern.html
//
//	client, err := NewClient("http://localhost:8000", WithPromptPrefix("abc"), WithPromptSuffix("def"))
//
// You can also invoke the options after creation:
//
//	client, err := NewClient("http://localhost:8000")
//	WithRequestTimeout(5 * time.Second)(client)
func NewClient(serverURL string, options ...func(*Client)) (*Client, error) {
	if serverURL == "" {
		return nil, errors.New("url cannot be empty")
	}
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing URL: %w", err)
	}
	client := &Client{
		url: u,
	}
	for _, o := range options {
		o(client)
	}
	return client, nil
}

func WithRequestTimeout(t time.Duration) func(*Client) {
	return func(c *Client) {
		c.requestTimeout = t
	}
}

func WithPromptPrefix(s string) func(*Client) {
	return func(c *Client) {
		c.promptPrefix = s
	}
}

func WithPromptSuffix(s string) func(*Client) {
	return func(c *Client) {
		c.promptSuffix = s
	}
}

func WithStop(s string) func(*Client) {
	return func(c *Client) {
		if c.stop == nil {
			c.stop = make([]string, 1)
		}
		c.stop = append(c.stop, s)
	}
}

func WithStops(s []string) func(*Client) {
	return func(c *Client) {
		c.stop = s
	}
}

// Send empty responses to clients; does not affect connection to llama.cpp;
// ignored for non-streaming requests.
func WithKeepAlive(t time.Duration) func(*Client) {
	return func(c *Client) {
		c.keepAlive = t
	}
}

// Used to store responses from the LLM - useful for capturing mock data.
func WithDebugFile(f *os.File) func(*Client) {
	return func(c *Client) {
		c.debugFile = f
	}
}

// Create a CompletionPayload struct with the appropriate prompt prefix,
// prompt suffix, and stops.
func (client Client) NewCompletionPayload(prompt string) *CompletionPayload {
	payload := NewCompletionPayload(client.promptPrefix + prompt + client.promptSuffix)
	payload.SetStops(client.stop)
	return payload
}
