package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Test that the AlertsController makes a request to the LLM whenever an MQTT
// message is received
func TestMQTTToLLM(t *testing.T) {
	controller, llm, svr := instantiateController(t)
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
	controller, llm, svr := instantiateController(t)
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

	newPrompt := "my new prompt"
	req := httptest.NewRequest(http.MethodPost, "/api/prompt", strings.NewReader(fmt.Sprintf(`{"prompt":"%s"}`, newPrompt)))
	w := httptest.NewRecorder()
	controller.PromptHandler(w, req)

	// prompt in controller should now be set to newPrompt
	if controller.getPrompt() != newPrompt {
		t.Errorf(`prompt was expected to be "%s" but was "%s" instead`, newPrompt, controller.getPrompt())
	}

	// wait for request to be received by LLM
	<-llm.requestReceived

	// ensure that newPrompt gets sent to the LLM
	if llm.req.Prompt != newPrompt {
		t.Errorf(`LLM prompt was expected to be "%s" but was "%s" instead`, newPrompt, llm.req.Prompt)
	}
}

func instantiateController(t *testing.T) (*AlertsController, *mockLLM, *httptest.Server) {
	llm := mockLLM{
		t:               t,
		requestReceived: make(chan struct{}),
	}
	svr := httptest.NewServer(http.HandlerFunc(llm.handler))
	controller := NewAlertsController(
		make(chan SSEEvent, 10),
		svr.URL,
		"dummy",
		"300m",
		"",
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

	w.Write([]byte(`{"reponse":"dummy"}`))
}
