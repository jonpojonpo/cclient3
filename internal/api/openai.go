package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// OpenAIProvider implements Provider for OpenAI's chat-completions API.
type OpenAIProvider struct {
	apiKey   string
	endpoint string // e.g. "https://api.openai.com"
	http     *http.Client
}

// NewOpenAIProvider creates an OpenAI provider.
func NewOpenAIProvider(apiKey, endpoint string) *OpenAIProvider {
	if endpoint == "" {
		endpoint = "https://api.openai.com"
	}
	return &OpenAIProvider{
		apiKey:   apiKey,
		endpoint: strings.TrimSuffix(endpoint, "/"),
		http:     &http.Client{Timeout: 300 * time.Second},
	}
}

func (o *OpenAIProvider) Name() string { return "openai" }

func (o *OpenAIProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
}

func (o *OpenAIProvider) doPost(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", o.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	o.setHeaders(req)

	resp, err := o.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai unreachable at %s: %w", o.endpoint, err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, o.parseError(resp)
	}
	return resp, nil
}

func (o *OpenAIProvider) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var apiErr APIError
	apiErr.StatusCode = resp.StatusCode
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}
	return &apiErr
}

func (o *OpenAIProvider) StreamMessage(ctx context.Context, req *Request, cb StreamCallback) error {
	oaReq := toOpenAI(req, true)
	oaReq.StreamOptions = &oaStreamOptions{IncludeUsage: true}

	body, err := json.Marshal(oaReq)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	resp, err := o.doPost(ctx, "/v1/chat/completions", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	cb.OnMessageStart(Response{})

	type pendingTool struct {
		id   string
		name string
	}

	pending := map[int]*pendingTool{}
	textIndex := 0
	textStarted := false
	var totalIn, totalOut int

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := line[6:]
		if data == "[DONE]" {
			break
		}

		var chunk oaStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Usage != nil {
			totalIn = chunk.Usage.PromptTokens
			totalOut = chunk.Usage.CompletionTokens
		}

		for _, choice := range chunk.Choices {
			d := choice.Delta
			if d.Content != "" {
				if !textStarted {
					cb.OnContentBlockStart(textIndex, ResponseBlock{Type: "text"})
					textStarted = true
				}
				cb.OnTextDelta(textIndex, d.Content)
			}

			for _, tc := range d.ToolCalls {
				idx := tc.Index
				if _, exists := pending[idx]; !exists {
					toolIdx := textIndex + 1 + idx
					pending[idx] = &pendingTool{id: tc.ID, name: tc.Function.Name}
					cb.OnContentBlockStart(toolIdx, ResponseBlock{
						Type: "tool_use",
						ID:   tc.ID,
						Name: tc.Function.Name,
					})
				}
				if tc.ID != "" {
					pending[idx].id = tc.ID
				}
				if tc.Function.Name != "" {
					pending[idx].name = tc.Function.Name
				}
				toolIdx := textIndex + 1 + idx
				cb.OnInputJSONDelta(toolIdx, tc.Function.Arguments)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}

	cb.OnMessageDelta(MessageDelta{}, &Usage{InputTokens: totalIn, OutputTokens: totalOut})
	cb.OnMessageStop()
	return nil
}

func (o *OpenAIProvider) SendMessage(ctx context.Context, req *Request) (*Response, error) {
	oaReq := toOpenAI(req, false)
	body, err := json.Marshal(oaReq)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	resp, err := o.doPost(ctx, "/v1/chat/completions", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var oaResp struct {
		Choices []struct {
			Message struct {
				Content   string       `json:"content"`
				ToolCalls []oaToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&oaResp); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	var blocks []ResponseBlock
	if len(oaResp.Choices) > 0 {
		msg := oaResp.Choices[0].Message
		if msg.Content != "" {
			blocks = append(blocks, ResponseBlock{Type: "text", Text: msg.Content})
		}
		for _, tc := range msg.ToolCalls {
			blocks = append(blocks, ResponseBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: json.RawMessage(tc.Function.Arguments),
			})
		}
	}

	return &Response{
		Content: blocks,
		Usage: Usage{
			InputTokens:  oaResp.Usage.PromptTokens,
			OutputTokens: oaResp.Usage.CompletionTokens,
		},
	}, nil
}

// ListModels queries OpenAI's /v1/models endpoint and returns all model IDs.
func (o *OpenAIProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", o.endpoint+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	o.setHeaders(req)

	resp, err := o.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai unreachable at %s: %w", o.endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, o.parseError(resp)
	}

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
			Object  string `json:"object"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}

	sort.SliceStable(result.Data, func(i, j int) bool {
		return result.Data[i].Created > result.Data[j].Created
	})

	models := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, ModelInfo{
			ID:          m.ID,
			DisplayName: m.ID,
			CreatedAt:   fmt.Sprintf("%d", m.Created),
			Type:        m.Object,
		})
	}
	return models, nil
}
