package llamacpp

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

Non-streaming response

{"content":".\n\nThis picture features a person wearing a light blue top. The individual is facing away from the camera, and their back is visible. The person appears to be standing still. The background is blurred and indistinct, which makes it difficult to discern any specific details.\n\nThere is no text present in the image. The style of the photograph is candid and casual, capturing a moment without any posing or staging. The focus is on the person, with the background intentionally out of focus to draw attention to the subject. ","id_slot":0,"stop":true,"model":"/models/llava.gguf","tokens_predicted":115,"tokens_evaluated":4,"generation_settings":{"n_ctx":4096,"n_predict":1000,"model":"/models/llava.gguf","seed":4294967295,"temperature":0.800000011920929,"dynatemp_range":0.0,"dynatemp_exponent":1.0,"top_k":40,"top_p":0.949999988079071,"min_p":0.05000000074505806,"tfs_z":1.0,"typical_p":1.0,"repeat_last_n":64,"repeat_penalty":1.0,"presence_penalty":0.0,"frequency_penalty":0.0,"penalty_prompt_tokens":[],"use_penalty_prompt_tokens":false,"mirostat":0,"mirostat_tau":5.0,"mirostat_eta":0.10000000149011612,"penalize_nl":false,"stop":[],"n_keep":0,"n_discard":0,"ignore_eos":false,"stream":false,"logit_bias":[],"n_probs":0,"min_keep":0,"grammar":"","samplers":["top_k","tfs_z","typical_p","top_p","min_p","temperature"]},"prompt":"describe this picture","truncated":false,"stopped_eos":true,"stopped_word":false,"stopped_limit":false,"stopping_word":"","tokens_cached":118,"timings":{"prompt_n":4,"prompt_ms":281.629,"prompt_per_token_ms":70.40725,"prompt_per_second":14.203082779117207,"predicted_n":115,"predicted_ms":25897.761,"predicted_per_token_ms":225.19792173913044,"predicted_per_second":4.4405383152620805}}


Streaming response

data: {"content":"This","stop":false,"id_slot":0,"multimodal":false}

data: {"content":"","id_slot":0,"stop":true,"model":"/models/llava.gguf","tokens_predicted":160,"tokens_evaluated":4,"generation_settings":{"n_ctx":4096,"n_predict":1000,"model":"/models/llava.gguf","seed":4294967295,"temperature":0.800000011920929,"dynatemp_range":0.0,"dynatemp_exponent":1.0,"top_k":40,"top_p":0.949999988079071,"min_p":0.05000000074505806,"tfs_z":1.0,"typical_p":1.0,"repeat_last_n":64,"repeat_penalty":1.0,"presence_penalty":0.0,"frequency_penalty":0.0,"penalty_prompt_tokens":[],"use_penalty_prompt_tokens":false,"mirostat":0,"mirostat_tau":5.0,"mirostat_eta":0.10000000149011612,"penalize_nl":false,"stop":[],"n_keep":0,"n_discard":0,"ignore_eos":false,"stream":true,"logit_bias":[],"n_probs":0,"min_keep":0,"grammar":"","samplers":["top_k","tfs_z","typical_p","top_p","min_p","temperature"]},"prompt":"describe this picture","truncated":false,"stopped_eos":true,"stopped_word":false,"stopped_limit":false,"stopping_word":"","tokens_cached":163,"timings":{"prompt_n":4,"prompt_ms":285.201,"prompt_per_token_ms":71.30025,"prompt_per_second":14.025196265090234,"predicted_n":160,"predicted_ms":39395.18,"predicted_per_token_ms":246.219875,"predicted_per_second":4.061410558347493}}

*/

// Completion sends a request to the /completion endpoint
//
//		image, _ := os.ReadFile("example.jpg")
//		client, _ := NewClient("http://localhost:8080")
//		payload := client.NewPayload("describe the contents of the image")
//		payload.AddImage(image)
//		payload.SetStream(true)
//		ch := make(chan CompletionResponse)
//		client.Completion(context.Background(), ch, *payload)
//		for resp := range ch {
//	   // check if resp.Err != nil
//		  fmt.Print(resp.Content)
//		}
func (client Client) Completion(ctx context.Context, ch chan<- CompletionResponse, payload CompletionPayload) {
	completionURL, err := client.url.Parse("/completion")
	if err != nil {
		ch <- CompletionResponse{
			Err: fmt.Errorf("error constructing completion URL: %w", err),
		}
		close(ch)
	}

	var b bytes.Buffer
	if err := json.NewEncoder(&b).Encode(payload); err != nil {
		ch <- CompletionResponse{
			Err: fmt.Errorf("error converting completion payload to JSON: %w", err),
		}
		close(ch)
	}

	var cancelRequest context.CancelFunc
	if client.requestTimeout > 0 {
		ctx, cancelRequest = context.WithTimeout(ctx, client.requestTimeout)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, completionURL.String(), bytes.NewReader(b.Bytes()))
	if err != nil {
		ch <- CompletionResponse{
			Err: fmt.Errorf("error creating completion request: %w", err),
		}
		close(ch)
		if cancelRequest != nil {
			cancelRequest()
		}
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		ch <- CompletionResponse{
			Err: fmt.Errorf("error making completion request: %w", err),
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
			parseCompletionResponse(ch, line, payload.Stream)
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
					ch <- CompletionResponse{}
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

func parseCompletionResponse(ch chan<- CompletionResponse, text string, stream bool) {
	type llamacppResponse struct {
		Content string `json:"content"`
	}

	if stream {
		if !strings.HasPrefix(text, "data: ") {
			return
		}
		text = text[len("data: "):]
		var parsed llamacppResponse
		if err := json.NewDecoder(strings.NewReader(text)).Decode(&parsed); err != nil {
			ch <- CompletionResponse{
				Err: fmt.Errorf("error decoding completion response JSON: %w", err),
			}
			return
		}
		ch <- CompletionResponse{
			Content: parsed.Content,
		}
		return
	}

	// non-streaming request
	var parsed llamacppResponse
	if json.NewDecoder(strings.NewReader(text)).Decode(&parsed) != nil {
		// ignore parsing errors
		return
	}
	ch <- CompletionResponse{
		Content: parsed.Content,
	}
}
