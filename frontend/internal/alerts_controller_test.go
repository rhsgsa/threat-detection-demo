package internal_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kwkoo/threat-detection-frontend/internal"
)

// Test that the AlertsController makes a request to ollama whenever an MQTT
// message is received
func TestMQTTToOllama(t *testing.T) {
	m := newMocks(t, "")
	defer m.close()

	// simulate alert coming in from MQTT
	m.controller.MQTTHandler(nil, newMockMQTTMessage(`{"annotated_image":"dummyannogtated","raw_image":"dummy","timestamp":1234}`))
	// wait for request to be received by ollama
	m.waitForOllamaRequest()

	if m.ollama.req.Images == nil || len(m.ollama.req.Images) == 0 {
		t.Error("ollama did not receive any images")
	} else if m.ollama.req.Images[0] != "dummy" {
		t.Errorf(`ollama received an image ("%s") different from what was expected`, m.ollama.req.Images[0])
	} else {
		t.Log("ollama received the raw image correctly")
	}
}

// Test that the AlertsController makes a request to ollama and openai
// whenever a REST call is made to change the prompt
// Also test that the appropriate SSE events are sent
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

	m := newMocks(t, promptsFilename)
	defer m.close()

	prompts := getPrompts(t, m.controller)
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
	m.controller.MQTTHandler(nil, mockMQTTMessage{[]byte(`{"annotated_image":"dummy","raw_image":"dummy","timestamp":1234}`)})
	// wait for request to be received by ollama
	t.Log("waiting for request to be received by ollama...")
	<-m.ollama.requestReceived
	t.Log("ollama request received")
	m.resetOllamaRequestReceivedChannel()

	if abort := setPrompt(t, m.controller, fmt.Sprintf(`{"id":%d}`, newPromptID), false); abort {
		return
	}

	// wait for request to be received by ollama
	m.waitForOllamaRequest()

	// ensure that newPrompt gets sent to ollama
	// Note - ollama is supposed to get the descriptive prompt - however,
	// we are using the default prompts so the descriptive prompts are set to
	// the short prompts; that's why we're ok to compare ollama's prompt with
	// the short prompt
	if m.ollama.req.Prompt == "descriptiveprompt2" {
		t.Log("ollama prompt was set correctly")
	} else {
		t.Errorf(`ollama prompt was expected to be "descriptiveprompt2" but was "%s" instead`, m.ollama.req.Prompt)
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

	// ensure that ollama_response SSE events are received
	if !m.sseEventsExist("ollama_response") {
		t.Errorf(`did not receive expected ollama_response SSE events`)
	}

	// ensure that openai_response SSE events are received
	if !m.sseEventsExist("openai_response") {
		t.Errorf(`did not receive expected openai_response SSE events`)
	}
}

// Test that the set prompt REST API responds appropriately
func TestSetPromptREST(t *testing.T) {
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

	m := newMocks(t, promptsFilename)
	defer m.close()

	// expect an error because we do not have any pending alerts
	if abort := setPrompt(t, m.controller, `{"id":2}`, true); abort {
		return
	}

	// simulate alert coming in from MQTT
	m.controller.MQTTHandler(nil, mockMQTTMessage{[]byte(`{"annotated_image":"dummy","raw_image":"dummy","timestamp":1234}`)})
	// wait for request to be received by ollama
	m.waitForOllamaRequest()

	// happy case
	setPrompt(t, m.controller, `{"id":1}`, false)

	// invalid JSON
	setPrompt(t, m.controller, `abc`, true)

	// missing required field
	setPrompt(t, m.controller, `{"prompt":2}`, true)

	// wrong type
	setPrompt(t, m.controller, `{"id":"2"}`, true)
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

// returns true if subsequent tests should be aborted
func setPrompt(t *testing.T, controller *internal.AlertsController, body string, errorExpected bool) bool {
	req := httptest.NewRequest(http.MethodPost, "/api/prompt", strings.NewReader(body))
	w := httptest.NewRecorder()
	controller.PromptHandler(w, req)

	statusCode := w.Result().StatusCode
	bodyBytes, err := io.ReadAll(w.Result().Body)
	w.Result().Body.Close()
	if err != nil {
		t.Errorf("error reading body after setting prompt: %v", err)
		return true
	}
	responseBody := strings.TrimSpace(string(bodyBytes))

	if errorExpected {
		if statusCode >= 200 && statusCode <= 300 {
			t.Errorf(`we expected an error after setting the prompt with payload '%s' but got status code = %d and body = '%s'`, body, statusCode, responseBody)
			return true
		}

		t.Logf(`received expected error with status code = %d and body = '%s'`, statusCode, responseBody)
		return false
	}

	// error not expected
	if statusCode < 200 || statusCode >= 300 {
		t.Errorf(`unexpected error after setting the prompt with payload '%s' but got status code = %d and body = '%s'`, body, statusCode, responseBody)
		return true
	}

	t.Logf(`successfully set prompt with payload '%s' - received response '%s'`, body, responseBody)
	return false
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
