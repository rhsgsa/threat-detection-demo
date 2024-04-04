package internal_test

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

	"github.com/kwkoo/threat-detection-frontend/internal"
)

// Test that the AlertsController makes a request to the LLM whenever an MQTT
// message is received
func TestMQTTToLLM(t *testing.T) {
	m := newMocks(t)
	defer m.llm.httpServer.Close()
	controller := instantiateController(m, "")

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

	// simulate alert coming in from MQTT
	controller.MQTTHandler(nil, newMockMQTTMessage(`{"annotated_image":"dummyannogtated","raw_image":"dummy","timestamp":1234}`))
	// wait for request to be received by LLM
	t.Log("waiting for request to be received by LLM...")
	<-m.llm.requestReceived
	t.Log("LLM request received")

	if m.llm.req.Images == nil || len(m.llm.req.Images) == 0 {
		t.Error("LLM did not receive any images")
	} else if m.llm.req.Images[0] != "dummy" {
		t.Errorf(`LLM received an image ("%s") different from what was expected`, m.llm.req.Images[0])
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

	m := newMocks(t)
	defer m.llm.httpServer.Close()
	controller := instantiateController(m, promptsFilename)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		controller.LLMChannelProcessor(ctx)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		m.consumeSSEEvents(ctx)
		wg.Done()
	}()

	defer func() {
		time.Sleep(time.Second) // sleep to allow LLM HTTP client requests to complete
		cancel()
		close(m.sseClient.ch)
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
	<-m.llm.requestReceived
	t.Log("LLM request received")
	m.resetRequestReceivedChannel()

	req := httptest.NewRequest(http.MethodPost, "/api/prompt", strings.NewReader(fmt.Sprintf(`{"id":%d}`, newPromptID)))
	w := httptest.NewRecorder()
	controller.PromptHandler(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("did not get 200 status code after setting prompt - got %d instead", w.Result().StatusCode)
		return
	}

	// wait for request to be received by LLM
	t.Log("waiting for request to be received by LLM...")
	<-m.llm.requestReceived
	t.Log("LLM request received")

	// ensure that newPrompt gets sent to the LLM
	// Note - the LLM is supposed to get the descriptive prompt - however,
	// we are using the default prompts so the descriptive prompts are set to
	// the short prompts; that's why we're ok to compare the LLM prompt with
	// the short prompt
	if m.llm.req.Prompt == "descriptiveprompt2" {
		t.Log("LLM prompt was set correctly")
	} else {
		t.Errorf(`LLM prompt was expected to be "descriptiveprompt2" but was "%s" instead`, m.llm.req.Prompt)
	}

	// pause to allow mockSSEClient to consume the new prompt event
	time.Sleep(time.Second)

	shortPrompt, err := m.unmarshalSSEClientPrompt()
	if err != nil {
		t.Errorf("error unmarshalling SSE client prompt: %v", err)
		return
	}
	if shortPrompt.Id != 2 {
		t.Errorf("expected short prompt ID to be 2 but got %d instead", shortPrompt.Id)
	}
	if shortPrompt.Prompt != "short2" {
		t.Errorf(`expected short prompt to be "short2" but got "%s" instead`, shortPrompt.Prompt)
	}
}

type shortPrompt struct {
	ID     int    `json:"id"`
	Prompt string `json:"prompt"`
}

func getPrompts(t *testing.T, controller *internal.AlertsController) map[int]shortPrompt {
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

func instantiateController(m *mocks, promptsFile string) *internal.AlertsController {
	log.SetOutput((os.Stdout))
	controller := internal.NewAlertsController(
		m.sseClient.ch,
		m.llm.httpServer.URL,
		"dummy",
		"300m",
		promptsFile,
	)
	return controller
}

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
