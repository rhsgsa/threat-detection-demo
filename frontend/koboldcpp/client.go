package koboldcpp

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"
)

const mockOutputFilename = "/tmp/koboldcpp.txt"

type Client struct {
	url            *url.URL
	requestTimeout time.Duration
	promptPrefix   string
	promptSuffix   string
	stop           []string
	keepAlive      time.Duration // send empty responses to clients; does not affect connection to koboldcpp
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

func WithKeepAlive(t time.Duration) func(*Client) {
	return func(c *Client) {
		c.keepAlive = t
	}
}

func (client Client) NewPayload(prompt string) *Payload {
	payload := NewPayload(client.promptPrefix + prompt + client.promptSuffix)
	payload.SetStops(client.stop)
	return payload
}

// Used to store responses from the LLM - useful for capturing mock data.
func (c *Client) SaveModelResponses() error {
	f, err := os.Create(mockOutputFilename)
	if err != nil {
		return err
	}
	c.debugFile = f
	return nil
}

func (c *Client) Shutdown() {
	if c.debugFile == nil {
		return
	}
	c.debugFile.Close()
	c.debugFile = nil
}
