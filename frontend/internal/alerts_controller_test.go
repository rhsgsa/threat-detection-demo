package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Test that the AlertsController makes a request to the LLM whenever an MQTT
// message is received
func TestMQTTToLLM(t *testing.T) {
	controller, llm, svr := instantiateController(t, "")
	defer svr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		controller.LLMChannelProcessor(ctx)
		wg.Done()
	}()

	defer func() {
		time.Sleep(time.Second) // sleep to allow LLM HTTP client requests to complete
		cancel()
		wg.Wait()
	}()

	event := alertEvent{
		annotatedImage: []byte("abcd"),
		rawImage:       []byte("efgh"),
	}
	controller.llmCh <- event

	// wait for request to be received by LLM
	<-llm.requestReceived

	if llm.req.Images == nil || len(llm.req.Images) == 0 {
		t.Error("LLM did not receive any images")
	} else if string(event.rawImage) != llm.req.Images[0] {
		t.Errorf(`LLM received an image ("%s") different from the raw image ("%s")`, llm.req.Images[0], event.rawImage)
	} else {
		t.Log("LLM received the raw image correctly")
	}
}

// Test that the AlertsController makes a request to the LLM whenever a REST
// call is made to change the prompt
func TestSetPrompt(t *testing.T) {
	customPrompts := []string{
		"short0|descriptiveprompt0",
		"short1",
		"short2|descriptiveprompt2",
	}
	promptsFilename, err := createTempPromptFile(t, customPrompts)
	if err != nil {
		t.Errorf("error create prompts file: %v", err)
		return
	}
	controller, llm, svr := instantiateController(t, promptsFilename)
	defer svr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		controller.LLMChannelProcessor(ctx)
		wg.Done()
	}()

	sseClient := mockSSEClient{}
	wg.Add(1)
	go func() {
		sseClient.consumeSSEEvents(ctx, controller.sseCh)
		wg.Done()
	}()

	defer func() {
		time.Sleep(time.Second) // sleep to allow LLM HTTP client requests to complete
		cancel()
		close(controller.sseCh)
		wg.Wait()
	}()

	prompts := getPrompts(t, controller)
	if prompts == nil {
		return
	}
	if len(prompts) != len(customPrompts) {
		t.Errorf("expected to get %d prompts from server but got %d instead", len(customPrompts), len(prompts))
		return
	}
	t.Logf("received prompts from server: %v", prompts)

	// we will be setting the prompt to ID 2 - make sure it exists
	newPromptID := 2
	if _, ok := prompts[newPromptID]; !ok {
		t.Errorf("prompt with ID %d does not exist on server", newPromptID)
	}

	// simulate alert coming in from MQTT
	controller.MQTTHandler(nil, mockMQTTMessage{[]byte(`{"annotated_image":"dummy","raw_image":"dummy","timestamp":1234}`)})
	// wait for request to be received by LLM
	t.Log("waiting for request to be received by LLM...")
	<-llm.requestReceived
	t.Log("LLM request received")
	llm.resetRequestReceivedChannel()

	req := httptest.NewRequest(http.MethodPost, "/api/prompt", strings.NewReader(fmt.Sprintf(`{"id":%d}`, newPromptID)))
	w := httptest.NewRecorder()
	controller.PromptHandler(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("did not get 200 status code after setting prompt - got %d instead", w.Result().StatusCode)
		return
	}

	// prompt in controller should now be set to newPrompt
	if controller.prompts.getSelectedPromptItem().ID != newPromptID {
		t.Errorf(`prompt was expected to be %d but was %d instead`, newPromptID, controller.prompts.getSelectedPromptItem().ID)
	}

	// wait for request to be received by LLM
	t.Log("waiting for request to be received by LLM...")
	<-llm.requestReceived
	t.Log("LLM request received")

	// ensure that newPrompt gets sent to the LLM
	// Note - the LLM is supposed to get the descriptive prompt - however,
	// we are using the default prompts so the descriptive prompts are set to
	// the short prompts; that's why we're ok to compare the LLM prompt with
	// the short prompt
	if llm.req.Prompt == "descriptiveprompt2" {
		t.Log("LLM prompt was set correctly")
	} else {
		t.Errorf(`LLM prompt was expected to be "descriptiveprompt2" but was "%s" instead`, llm.req.Prompt)
	}

	// pause to allow mockSSEClient to consume the new prompt event
	time.Sleep(time.Second)
	if sseClient.prompt == "short2" {
		t.Log("SSE client received the correct short prompt")
	} else {
		t.Errorf(`Expected SSE client prompt to be "short2" - was "%s" instead`, sseClient.prompt)
	}
}

type shortPrompt struct {
	ID     int    `json:"id"`
	Prompt string `json:"prompt"`
}

func getPrompts(t *testing.T, controller *AlertsController) map[int]shortPrompt {
	req := httptest.NewRequest(http.MethodGet, "/api/prompt", nil)
	w := httptest.NewRecorder()
	controller.PromptHandler(w, req)
	dec := json.NewDecoder(w.Body)
	prompts := make(map[int]shortPrompt)
	promptList := []shortPrompt{}
	if err := dec.Decode(&promptList); err != nil {
		t.Errorf("error getting prompts: %v", err)
		return nil
	}
	for _, item := range promptList {
		prompts[item.ID] = item
	}
	return prompts
}

func instantiateController(t *testing.T, promptsFile string) (*AlertsController, *mockLLM, *httptest.Server) {
	log.SetOutput((os.Stdout))
	llm := mockLLM{
		t:               t,
		requestReceived: make(chan struct{}),
	}
	svr := httptest.NewServer(http.HandlerFunc(llm.handler))
	controller := NewAlertsController(
		make(chan SSEEvent, 100),
		svr.URL,
		"dummy",
		"300m",
		promptsFile,
	)
	return controller, &llm, svr
}

type llmReq struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Images []string `json:"images"`
}

type mockLLM struct {
	t               *testing.T
	req             llmReq
	requestReceived chan struct{} // channel is closed when a request is received
}

func (llm *mockLLM) handler(w http.ResponseWriter, r *http.Request) {
	defer close(llm.requestReceived)
	llm.t.Log("LLM handler called")
	var req llmReq
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		llm.t.Errorf("could not decode incoming llmReq: %v", err)
	}
	llm.req = req

	w.Write([]byte(`{"response":"dummy"}`))
}

func (llm *mockLLM) resetRequestReceivedChannel() {
	llm.requestReceived = make(chan struct{})
}

type mockMQTTMessage struct {
	payload []byte
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

func createTempPromptFile(t *testing.T, lines []string) (string, error) {
	f, err := os.CreateTemp(t.TempDir(), "prompts.txt")
	if err != nil {
		return "", fmt.Errorf("could not create temp file for prompts: %v", err)
	}
	defer f.Close()
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			return "", fmt.Errorf("error writing to prompts file: %v", err)
		}
	}
	return f.Name(), nil
}

type mockSSEClient struct {
	prompt string
}

func (m *mockSSEClient) consumeSSEEvents(ctx context.Context, ch chan SSEEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-ch:
			if event.EventType == "prompt" {
				m.prompt = string(event.Data)
			}
		}
	}
}
