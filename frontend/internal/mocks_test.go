package internal_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kwkoo/threat-detection-frontend/internal"
)

type mockLLMReq struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Images []string `json:"images"`
}

type mockShortPrompt struct {
	Id     int    `json:"id"`
	Prompt string `json:"prompt"`
}

type mocks struct {
	t   *testing.T
	llm struct {
		httpServer      *httptest.Server
		req             mockLLMReq
		requestReceived chan struct{} // channel is closed when a request is received
	}
	sseClient struct {
		ch     chan internal.SSEEvent
		prompt string
	}
}

func newMocks(t *testing.T) *mocks {
	m := mocks{
		t: t,
		llm: struct {
			httpServer      *httptest.Server
			req             mockLLMReq
			requestReceived chan struct{} // channel is closed when a request is received
		}{},
		sseClient: struct {
			ch     chan internal.SSEEvent
			prompt string
		}{
			ch: make(chan internal.SSEEvent, 100),
		},
	}
	m.llm.httpServer = httptest.NewServer(http.HandlerFunc(m.llmHandler))
	m.resetRequestReceivedChannel()
	return &m
}

func (m *mocks) llmHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if m.llm.requestReceived == nil {
			return
		}
		close(m.llm.requestReceived)
		m.llm.requestReceived = nil
	}()
	m.t.Log("LLM handler called")
	var req mockLLMReq
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.t.Errorf("could not decode incoming mockLLMReq: %v", err)
	}
	m.llm.req = req

	w.Write([]byte(`{"response":"dummy"}`))
}

func (m *mocks) resetRequestReceivedChannel() {
	m.llm.requestReceived = make(chan struct{})
}

func (m *mocks) consumeSSEEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-m.sseClient.ch:
			if event.EventType == "prompt" {
				m.sseClient.prompt = string(event.Data)
			}
		}
	}
}

func (m *mocks) unmarshalSSEClientPrompt() (mockShortPrompt, error) {
	var sp mockShortPrompt
	r := strings.NewReader(m.sseClient.prompt)
	if err := json.NewDecoder(r).Decode(&sp); err != nil {
		return mockShortPrompt{}, err
	}
	return sp, nil
}

/*
 * MQTT Message
 */
type mockMQTTMessage struct {
	payload []byte
}

func newMockMQTTMessage(s string) mockMQTTMessage {
	return mockMQTTMessage{[]byte(s)}
}

func (m mockMQTTMessage) Payload() []byte {
	return m.payload
}

func (m mockMQTTMessage) Duplicate() bool {
	return false
}

func (m mockMQTTMessage) Qos() byte {
	return 0
}

func (m mockMQTTMessage) Retained() bool {
	return false
}

func (m mockMQTTMessage) Topic() string {
	return ""
}

func (m mockMQTTMessage) MessageID() uint16 {
	return 0
}

func (m mockMQTTMessage) Ack() {}
