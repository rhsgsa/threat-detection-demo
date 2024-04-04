package prompts_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kwkoo/threat-detection-frontend/internal/prompts"
)

func TestSimplePrompts(t *testing.T) {
	const input = `line0 dummy
line1
line2 dummy dummy
line3`

	r := strings.NewReader(input)
	container, err := prompts.NewPromptsContainer(r)
	if err != nil {
		t.Errorf("could not instantiate PromptsContainer: %v", err)
		return
	}
	t.Log("successfully instantiated PromptsContainer")

	var buf bytes.Buffer
	if err := container.StreamShortPrompts(&buf); err != nil {
		t.Errorf("error streaming short prompts: %v", err)
		return
	}
	type shortPrompt struct {
		ID     int    `json:"id"`
		Prompt string `json:"prompt"`
	}
	var decodedShortPrompts []shortPrompt
	if err := json.NewDecoder(&buf).Decode(&decodedShortPrompts); err != nil {
		t.Errorf("error decoding short prompts stream: %v", err)
		return
	}
	t.Logf("successfully decoded %d short prompts", len(decodedShortPrompts))
	if len(decodedShortPrompts) != 4 {
		t.Errorf("expected 4 short prompts")
	}

	// the first item should currently be selected
	if abort := checkSelectedPromptItem(t, container, 0, "line0 dummy", "line0 dummy"); abort {
		return
	}

	// set prompt to non-existent ID - we should get an error
	if abort := setPromptAndCheckError(t, container, 99, true); abort {
		return
	}

	// set prompt to 2 - we should not get an error
	if abort := setPromptAndCheckError(t, container, 2, false); abort {
		return
	}

	checkSelectedPromptItem(t, container, 2, "line2 dummy dummy", "line2 dummy dummy")
}

// some prompts have a different descriptive field
func TestDescriptivePrompts(t *testing.T) {
	const input = `line0|the
line1
line2|quick brown
line3
line4|
line5|fox`

	r := strings.NewReader(input)
	container, err := prompts.NewPromptsContainer(r)
	if err != nil {
		t.Errorf("could not instantiate PromptsContainer: %v", err)
		return
	}
	t.Log("successfully instantiated PromptsContainer")

	var buf bytes.Buffer
	if err := container.StreamShortPrompts(&buf); err != nil {
		t.Errorf("error streaming short prompts: %v", err)
		return
	}
	type shortPrompt struct {
		ID     int    `json:"id"`
		Prompt string `json:"prompt"`
	}
	var decodedShortPrompts []shortPrompt
	if err := json.NewDecoder(&buf).Decode(&decodedShortPrompts); err != nil {
		t.Errorf("error decoding short prompts stream: %v", err)
		return
	}
	t.Logf("successfully decoded %d short prompts", len(decodedShortPrompts))
	if len(decodedShortPrompts) != 6 {
		t.Errorf("expected 6 short prompts")
	}

	// the first item should currently be selected
	if abort := checkSelectedPromptItem(t, container, 0, "line0", "the"); abort {
		return
	}

	// set prompt to 1 - we should not get an error
	if abort := setPromptAndCheckError(t, container, 1, false); abort {
		return
	}

	if abort := checkSelectedPromptItem(t, container, 1, "line1", "line1"); abort {
		return
	}

	// set prompt to 2 - we should not get an error
	if abort := setPromptAndCheckError(t, container, 2, false); abort {
		return
	}

	if abort := checkSelectedPromptItem(t, container, 2, "line2", "quick brown"); abort {
		return
	}

	// set prompt to 4 - we should not get an error
	if abort := setPromptAndCheckError(t, container, 4, false); abort {
		return
	}

	checkSelectedPromptItem(t, container, 4, "line4", "line4")
}

// returns true if subsequent tests should be aborted
func checkSelectedPromptItem(t *testing.T, container *prompts.PromptsContainer, expectedID int, expectedShort, expectedDescriptive string) bool {
	selectedPrompt, err := container.GetSelectedPromptItem()
	if err != nil {
		t.Errorf("unexpected error when trying to get selected item: %v", err)
		return true
	}
	if selectedPrompt == nil {
		t.Error("the selected item is nil - this should never happen if err is not nil")
		return true
	}
	if selectedPrompt.ID != expectedID {
		t.Errorf("expected ID to be %d, instead it was %d", expectedID, selectedPrompt.ID)
	}
	if selectedPrompt.Short != expectedShort {
		t.Errorf(`expected Short to be "%s", instead it was "%s"`, expectedShort, selectedPrompt.Short)
	}
	if selectedPrompt.Descriptive != expectedDescriptive {
		t.Errorf(`expected Descriptive to be "%s", instead it was "%s"`, expectedDescriptive, selectedPrompt.Descriptive)
	}
	return false
}

// prompt line has not short field set
func TestNoShort(t *testing.T) {
	const input = `line0|the
line1
|quick brown
line3
line4|
line5|fox`

	r := strings.NewReader(input)
	_, err := prompts.NewPromptsContainer(r)
	if err == nil {
		t.Error("expected an error due to a missing short field but did not get one")
		return
	}
	t.Logf("got an expected error: %v", err)
}

// returns true if subsequent tests should be aborted
func setPromptAndCheckError(t *testing.T, container *prompts.PromptsContainer, id int, errorExpected bool) bool {
	err := container.SetSelectedPrompt(id)
	if err != nil && !errorExpected {
		t.Errorf("got an unexpected error when we tried to set the selected prompt to %d: %v", id, err)
		return true
	}
	if err == nil && errorExpected {
		t.Errorf("expected an error when setting selected prompt to %d but did not get any", id)
		return true
	}
	return false
}
