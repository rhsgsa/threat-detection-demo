package internal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

type promptsContainer struct {
	mux            sync.RWMutex
	selectedPrompt int
	allPrompts     map[int]promptItem
}

type promptItem struct {
	ID          int    `json:"id"`
	Short       string `json:"short"`
	Descriptive string `json:"descriptive"`
}

func (item promptItem) getSSEBytes() []byte {
	var b bytes.Buffer
	event := struct {
		ID     int    `json:"id"`
		Prompt string `json:"prompt"`
	}{
		ID:     item.ID,
		Prompt: item.Short,
	}
	json.NewEncoder(&b).Encode(&event)
	return b.Bytes()
}

func newPrompts(promptsFile string) (*promptsContainer, error) {
	prompts := promptsContainer{
		allPrompts: make(map[int]promptItem),
	}
	if promptsFile == "" {
		log.Print("no prompts file provided - will use hardcoded prompts")
		prompts.addPromptFromLine("Please describe this image")
		prompts.addPromptFromLine("Is this person a threat?")
		return &prompts, nil
	}
	lines, err := readLinesFromFile(promptsFile)
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		prompts.addPromptFromLine(line)
	}
	if len(prompts.allPrompts) == 0 {
		return nil, fmt.Errorf("could not get prompts from %s", promptsFile)
	}
	return &prompts, nil
}

func (prompts *promptsContainer) streamShortPrompts(w io.Writer) {
	type shortPrompt struct {
		ID     int    `json:"id"`
		Prompt string `json:"prompt"`
	}
	all := make([]shortPrompt, 0, len(prompts.allPrompts))
	for _, item := range prompts.allPrompts {
		all = append(all, shortPrompt{ID: item.ID, Prompt: item.Short})
	}
	json.NewEncoder(w).Encode(all)
}

func (prompts *promptsContainer) setSelectedPrompt(id int) error {
	prompts.mux.Lock()
	defer prompts.mux.Unlock()
	if _, ok := prompts.allPrompts[id]; !ok {
		return fmt.Errorf("selected prompt ID %d does not exist", id)
	}
	prompts.selectedPrompt = id
	return nil
}

func (prompts *promptsContainer) getSelectedPromptItem() *promptItem {
	prompts.mux.RLock()
	defer prompts.mux.RUnlock()
	p, ok := prompts.allPrompts[prompts.selectedPrompt]
	if !ok {
		log.Printf("currently selected prompt ID is %d - but it does not exist in allPrompts", prompts.selectedPrompt)
		return nil
	}
	return &p
}

func (prompts *promptsContainer) addPromptFromLine(line string) error {
	if line == "" {
		return nil
	}
	parts := strings.Split(line, "|")
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 {
		return prompts.addPromptItem(parts[0], parts[0])
	}
	return prompts.addPromptItem(parts[0], parts[1])
}

func (prompts *promptsContainer) addPromptItem(short, descriptive string) error {
	item := promptItem{
		ID:          len(prompts.allPrompts),
		Short:       short,
		Descriptive: descriptive,
	}
	prompts.allPrompts[item.ID] = item
	return nil
}

func readLinesFromFile(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, nil
}
