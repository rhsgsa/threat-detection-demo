package prompts

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

type PromptsContainer struct {
	mux            sync.RWMutex
	selectedPrompt int
	promptsMap     map[int]PromptItem
	promptsList    []PromptItem
}

type PromptItem struct {
	ID          int    `json:"id"`
	Short       string `json:"short"`
	Descriptive string `json:"descriptive"`
}

func (item PromptItem) GetSSEBytes() []byte {
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

func NewPromptsContainerFromFile(promptsFile string) (*PromptsContainer, error) {
	if promptsFile == "" {
		log.Print("no prompts file provided - will use hardcoded prompts")
		prompts := PromptsContainer{
			promptsMap: make(map[int]PromptItem),
		}
		prompts.addPromptFromLine("Please describe this image")
		prompts.addPromptFromLine("Is this person a threat?")
		return &prompts, nil
	}

	f, err := os.Open(promptsFile)
	if err != nil {
		return nil, fmt.Errorf("error trying to open prompts file %s: %w", promptsFile, err)
	}
	defer f.Close()
	return NewPromptsContainer(f)
}

func NewPromptsContainer(r io.Reader) (*PromptsContainer, error) {
	prompts := PromptsContainer{
		promptsMap: make(map[int]PromptItem),
	}
	lines, err := readLinesFromStream(r)
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		if err := prompts.addPromptFromLine(line); err != nil {
			return nil, err
		}
	}
	if len(prompts.promptsMap) == 0 {
		return nil, errors.New("did not add any prompts")
	}
	return &prompts, nil
}

func (prompts *PromptsContainer) StreamShortPrompts(w io.Writer) error {
	type shortPrompt struct {
		ID     int    `json:"id"`
		Prompt string `json:"prompt"`
	}
	shortList := make([]shortPrompt, len(prompts.promptsList))
	for i, item := range prompts.promptsList {
		shortList[i] = shortPrompt{ID: item.ID, Prompt: item.Short}
	}
	if err := json.NewEncoder(w).Encode(shortList); err != nil {
		return fmt.Errorf("error streaming short prompts: %w", err)
	}
	return nil
}

func (prompts *PromptsContainer) SetSelectedPrompt(id int) error {
	prompts.mux.Lock()
	defer prompts.mux.Unlock()
	if _, ok := prompts.promptsMap[id]; !ok {
		return fmt.Errorf("selected prompt ID %d does not exist", id)
	}
	prompts.selectedPrompt = id
	return nil
}

func (prompts *PromptsContainer) GetSelectedPromptItem() (*PromptItem, error) {
	prompts.mux.RLock()
	defer prompts.mux.RUnlock()
	p, ok := prompts.promptsMap[prompts.selectedPrompt]
	if !ok {
		return nil, fmt.Errorf("currently selected prompt ID is %d - but it does not exist in allPrompts", prompts.selectedPrompt)
	}
	return &p, nil
}

func (prompts *PromptsContainer) addPromptFromLine(line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		log.Print("not adding line as prompt because it is blank")
		return nil
	}
	parts := strings.Split(line, "|")
	if len(parts) == 0 {
		log.Print("not adding line as prompt because it is blank")
		return nil
	}
	var descriptive string
	if len(parts) > 1 {
		descriptive = parts[1]
	}
	p, err := newPromptItem(len(prompts.promptsMap), parts[0], descriptive)
	if err != nil {
		return err
	}
	prompts.promptsMap[p.ID] = *p
	prompts.promptsList = append(prompts.promptsList, *p)
	return nil
}

func newPromptItem(id int, short, descriptive string) (*PromptItem, error) {
	short = strings.TrimSpace(short)
	descriptive = strings.TrimSpace(descriptive)
	if short == "" {
		return nil, errors.New("short prompt is not set")
	}
	if descriptive == "" {
		descriptive = short
	}
	return &PromptItem{ID: id, Short: short, Descriptive: descriptive}, nil
}

func readLinesFromStream(r io.Reader) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading prompts from stream: %w", err)
	}
	return lines, nil
}
