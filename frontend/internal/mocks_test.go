package internal_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kwkoo/threat-detection-frontend/internal"
)

type mockLlavaReq struct {
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
	llava      struct {
		httpServer      *httptest.Server
		req             mockLlavaReq
		requestReceived chan struct{} // channel is closed when a request is received
	}
	openai struct {
		httpServer *httptest.Server
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
		llava: struct {
			httpServer      *httptest.Server
			req             mockLlavaReq
			requestReceived chan struct{} // channel is closed when a request is received
		}{},
		openai: struct{ httpServer *httptest.Server }{},
		sseClient: struct {
			ch     chan internal.SSEEvent
			prompt string
			events []internal.SSEEvent
		}{
			ch:     make(chan internal.SSEEvent, 100),
			events: []internal.SSEEvent{},
		},
	}
	m.llava.httpServer = httptest.NewServer(http.HandlerFunc(m.llavaHandler))
	m.openai.httpServer = httptest.NewServer(http.HandlerFunc(m.openaiHandler))
	m.controller = internal.NewAlertsController(
		m.sseClient.ch,
		m.llava.httpServer.URL,
		promptsFile,
		"/mnt/models",
		"dummy prompt",
		m.openai.httpServer.URL,
	)
	m.resetLlavaRequestReceivedChannel()
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
	m.llava.httpServer.Close()
	m.openai.httpServer.Close()
	close(m.sseClient.ch)
	m.wg.Wait()
}

func (m *mocks) llavaHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if m.llava.requestReceived == nil {
			return
		}
		close(m.llava.requestReceived)
		m.llava.requestReceived = nil
	}()
	m.t.Log("llava handler called")
	var req mockLlavaReq
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.t.Errorf("could not decode incoming mockLlavaReq: %v", err)
	}
	m.llava.req = req

	w.Write([]byte(`{"response":"dummy llava response"}`))
}

func (m *mocks) openaiHandler(w http.ResponseWriter, r *http.Request) {
	m.t.Logf("openai handler called with URL %s", r.URL)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		m.t.Errorf("error received trying to read body of openai request: %v", err)
		http.Error(w, "error reading request body", http.StatusPreconditionFailed)
		return
	}
	m.t.Logf("openai handler received body %s", string(body))
	writeSSEEvent(w, `{"id":"cmpl-dfdfa582006c4fd89e52adf0d0f32317","object":"chat.completion.chunk","created":1713497907,"model":"/mnt/models","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null,"content_filter_results":{"hate":{"filtered":false},"self_harm":{"filtered":false},"sexual":{"filtered":false},"violence":{"filtered":false}}}]}`)
	writeSSEEvent(w, `{"id":"cmpl-dfdfa582006c4fd89e52adf0d0f32317","object":"chat.completion.chunk","created":1713497907,"model":"/mnt/models","choices":[{"index":0,"delta":{"content":" Medium threat"},"finish_reason":null,"content_filter_results":{"hate":{"filtered":false},"self_harm":{"filtered":false},"sexual":{"filtered":false},"violence":{"filtered":false}}}]}`)
	writeSSEEvent(w, `{"id":"cmpl-dfdfa582006c4fd89e52adf0d0f32317","object":"chat.completion.chunk","created":1713497907,"model":"/mnt/models","choices":[{"index":0,"delta":{},"finish_reason":"stop","content_filter_results":{"hate":{"filtered":false},"self_harm":{"filtered":false},"sexual":{"filtered":false},"violence":{"filtered":false}}}]}`)
}

func (m *mocks) waitForLlavaRequest() {
	if m.llava.requestReceived == nil {
		m.t.Error("llava requestReceived channel is nil")
		return
	}
	m.t.Log("waiting for request to be received by llava...")
	<-m.llava.requestReceived
	m.t.Log("llava request received")
}

func (m *mocks) resetLlavaRequestReceivedChannel() {
	if m.llava.requestReceived != nil {
		close(m.llava.requestReceived)
	}
	m.llava.requestReceived = make(chan struct{})
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

/*
func (m *mocks) sseEventExists(eventType string, substring string) bool {
	for _, event := range m.sseClient.events {
		if event.EventType == eventType && strings.Contains(string(event.Data), substring) {
			return true
		}
	}
	return false
}
*/

func (m *mocks) sseEventsExist(eventType string) bool {
	for _, event := range m.sseClient.events {
		if event.EventType == eventType {
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

func writeSSEEvent(w io.Writer, data string) {
	w.Write([]byte("data: "))
	w.Write([]byte(data))
	w.Write([]byte("\n\n"))
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
