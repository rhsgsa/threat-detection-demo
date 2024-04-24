package internal_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kwkoo/threat-detection-frontend/internal"
)

type mockOllamaReq struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Images []string `json:"images"`
}

type mockShortPrompt struct {
	Id     int    `json:"id"`
	Prompt string `json:"prompt"`
}

type mocks struct {
	t          *testing.T
	wg         sync.WaitGroup     // for goroutines
	ctx        context.Context    // for goroutines
	cancel     context.CancelFunc // for goroutines
	controller *internal.AlertsController
	ollama     struct {
		httpServer      *httptest.Server
		req             mockOllamaReq
		requestReceived chan struct{} // channel is closed when a request is received
	}
	sseClient struct {
		ch     chan internal.SSEEvent
		prompt string
		events []internal.SSEEvent
	}
}

func newMocks(t *testing.T, promptsFile string) *mocks {
	m := mocks{
		t: t,
		ollama: struct {
			httpServer      *httptest.Server
			req             mockOllamaReq
			requestReceived chan struct{} // channel is closed when a request is received
		}{},
		sseClient: struct {
			ch     chan internal.SSEEvent
			prompt string
			events []internal.SSEEvent
		}{
			ch:     make(chan internal.SSEEvent, 100),
			events: []internal.SSEEvent{},
		},
	}
	m.ollama.httpServer = httptest.NewServer(http.HandlerFunc(m.ollamaHandler))
	m.controller = internal.NewAlertsController(
		m.sseClient.ch,
		m.ollama.httpServer.URL,
		"dummy-model",
		"-1s", // keepalive
		promptsFile,
		"",
		"",
		"",
	)
	m.resetOllamaRequestReceivedChannel()
	m.launchGoroutines()
	return &m
}

func (m *mocks) launchGoroutines() {
	m.ctx, m.cancel = context.WithCancel(context.Background())

	// this goroutine processes alertEvents published by the MQTTHandler and
	// PromptHandler
	m.wg.Add(1)
	go func() {
		m.controller.LLMChannelProcessor(m.ctx)
		m.wg.Done()
	}()

	// this goroutine simulates an SSE browser client
	m.wg.Add(1)
	go func() {
		m.consumeSSEEvents(m.ctx)
		m.wg.Done()
	}()
}

// stops goroutines
func (m *mocks) close() {
	time.Sleep(time.Second) // sleep to allow LLM HTTP client requests to complete
	m.cancel()
	m.ollama.httpServer.Close()
	close(m.sseClient.ch)
	m.wg.Wait()
}

func (m *mocks) ollamaHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if m.ollama.requestReceived == nil {
			return
		}
		close(m.ollama.requestReceived)
		m.ollama.requestReceived = nil
	}()
	m.t.Log("ollama handler called")
	var req mockOllamaReq
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.t.Errorf("could not decode incoming mockOllamaReq: %v", err)
	}
	m.ollama.req = req

	w.Write([]byte(`{"response":"dummy"}`))
}

func (m *mocks) waitForOllamaRequest() {
	if m.ollama.requestReceived == nil {
		m.t.Error("ollama requestReceived channel is nil")
		return
	}
	m.t.Log("waiting for request to be received by ollama...")
	<-m.ollama.requestReceived
	m.t.Log("ollama request received")
}

func (m *mocks) resetOllamaRequestReceivedChannel() {
	if m.ollama.requestReceived != nil {
		close(m.ollama.requestReceived)
	}
	m.ollama.requestReceived = make(chan struct{})
}

func (m *mocks) consumeSSEEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-m.sseClient.ch:
			m.sseClient.events = append(m.sseClient.events, event)
			if event.EventType == "prompt" {
				m.sseClient.prompt = string(event.Data)
			}
		}
	}
}

func (m *mocks) sseEventExists(eventType string, substring string) bool {
	for _, event := range m.sseClient.events {
		if event.EventType == eventType && strings.Contains(string(event.Data), substring) {
			return true
		}
	}
	return false
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
