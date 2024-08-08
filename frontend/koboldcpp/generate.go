package koboldcpp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

/*
event: message
data: {"token": " the", "finish_reason": "null"}

event: message
data: {"token": " urban", "finish_reason": "length"}
*/

// Generate sends a request to the /api/extra/generate/stream endpoint
//
//			image, _ := os.ReadFile("example.jpg")
//			client, _ := NewClient("http://localhost:5001")
//			payload := client.NewPayload("describe the contents of the image")
//			payload.AddImage(image)
//			ch := make(chan Response)
//			client.Generate(context.Background(), ch, *payload)
//			for resp := range ch {
//		      if resp.Err != nil {
//	            log.Fatal(resp.Err)
//	          }
//			  fmt.Print(resp.Content)
//			}
func (client Client) Generate(ctx context.Context, ch chan<- Response, payload Payload) {
	endpointURL, err := client.url.Parse("/api/extra/generate/stream")
	if err != nil {
		ch <- Response{
			Err: fmt.Errorf("error constructing completion URL: %w", err),
		}
		close(ch)
	}

	var b bytes.Buffer
	if err := json.NewEncoder(&b).Encode(payload); err != nil {
		ch <- Response{
			Err: fmt.Errorf("error converting completion payload to JSON: %w", err),
		}
		close(ch)
	}

	var cancelRequest context.CancelFunc
	if client.requestTimeout > 0 {
		ctx, cancelRequest = context.WithTimeout(ctx, client.requestTimeout)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL.String(), bytes.NewReader(b.Bytes()))
	if err != nil {
		ch <- Response{
			Err: fmt.Errorf("error creating generate request: %w", err),
		}
		close(ch)
		if cancelRequest != nil {
			cancelRequest()
		}
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		ch <- Response{
			Err: fmt.Errorf("error making generate request: %w", err),
		}
		close(ch)
		if cancelRequest != nil {
			cancelRequest()
		}
	}

	ctx, bodyReaderCancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		// read lines here
		scanner := bufio.NewScanner(res.Body)
		for scanner.Scan() {
			line := scanner.Text()
			parseResponse(ch, line)
			if client.debugFile != nil {
				client.debugFile.WriteString(line)
				client.debugFile.WriteString("\n")
			}
		}
		bodyReaderCancel()
		res.Body.Close()
		wg.Done()
	}()

	if client.keepAlive > 0 {
		wg.Add(1)
		go func() {
			ticker := time.NewTicker(client.keepAlive)
		Loop:
			for {
				select {
				case <-ctx.Done():
					break Loop
				case <-ticker.C:
					ch <- Response{}
				}
			}
			ticker.Stop()
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
		if cancelRequest != nil {
			cancelRequest()
		}
	}()
}

func parseResponse(ch chan<- Response, text string) {
	type koboldResponse struct {
		Token string `json:"token"`
	}

	if !strings.HasPrefix(text, "data: ") {
		return
	}

	text = text[len("data: "):]
	var parsed koboldResponse
	if err := json.NewDecoder(strings.NewReader(text)).Decode(&parsed); err != nil {
		ch <- Response{
			Err: fmt.Errorf("error decoding generate response JSON: %w", err),
		}
		return
	}
	ch <- Response{
		Content: parsed.Token,
	}
}
