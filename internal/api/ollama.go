package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaProvider implements Provider for local Ollama instances via the
// OpenAI-compatible /v1/chat/completions endpoint.
type OllamaProvider struct {
	endpoint string // e.g. "http://localhost:11434"
	http     *http.Client
}

// NewOllamaProvider creates an Ollama provider. Defaults to localhost:11434.
func NewOllamaProvider(endpoint string) *OllamaProvider {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	return &OllamaProvider{
		endpoint: strings.TrimSuffix(endpoint, "/"),
		http:     &http.Client{Timeout: 300 * time.Second},
	}
}

func (o *OllamaProvider) Name() string { return "ollama" }

// --- OpenAI-compat wire types ---

type oaRequest struct {
	Model         string           `json:"model"`
	Messages      []oaMessage      `json:"messages"`
	Stream        bool             `json:"stream"`
	Tools         []oaTool         `json:"tools,omitempty"`
	StreamOptions *oaStreamOptions `json:"stream_options,omitempty"`
}

type oaStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type oaMessage struct {
	Role       string       `json:"role"`
	Content    interface{}  `json:"content"` // string or nil
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type oaTool struct {
	Type     string     `json:"type"` // "function"
	Function oaFunction `json:"function"`
}

type oaFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type oaToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function oaToolCallFunc `json:"function"`
}

type oaToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaStreamChunk struct {
	Choices []struct {
		Delta struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// toOpenAI translates an Anthropic Request to an OpenAI-compat request.
func toOpenAI(req *Request, stream bool) oaRequest {
	var msgs []oaMessage

	// System is interface{} — handle string, []SystemBlock, or nil
	switch sys := req.System.(type) {
	case string:
		if sys != "" {
			msgs = append(msgs, oaMessage{Role: "system", Content: sys})
		}
	case []SystemBlock:
		for _, s := range sys {
			msgs = append(msgs, oaMessage{Role: "system", Content: s.Text})
		}
	}

	for _, m := range req.Messages {
		switch content := m.Content.(type) {
		case string:
			msgs = append(msgs, oaMessage{Role: m.Role, Content: content})

		case []ContentBlock:
			var textParts []string
			var toolCalls []oaToolCall
			var toolResults []oaMessage

			for _, b := range content {
				switch b.Type {
				case "text":
					textParts = append(textParts, b.Text)
				case "thinking":
					// not supported by OpenAI format
				case "tool_use":
					toolCalls = append(toolCalls, oaToolCall{
						ID:   b.ID,
						Type: "function",
						Function: oaToolCallFunc{
							Name:      b.Name,
							Arguments: string(b.Input),
						},
					})
				case "tool_result":
					toolResults = append(toolResults, oaMessage{
						Role:       "tool",
						ToolCallID: b.ToolUseID,
						Content:    b.Content,
					})
				}
			}

			if len(toolResults) > 0 {
				msgs = append(msgs, toolResults...)
				continue
			}

			msg := oaMessage{Role: m.Role}
			if len(textParts) > 0 {
				msg.Content = strings.Join(textParts, "\n")
			}
			if len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
				if msg.Content == nil {
					msg.Content = ""
				}
			}
			msgs = append(msgs, msg)
		}
	}

	var tools []oaTool
	for _, t := range req.Tools {
		schema, _ := json.Marshal(t.InputSchema)
		tools = append(tools, oaTool{
			Type: "function",
			Function: oaFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  schema,
			},
		})
	}

	return oaRequest{Model: req.Model, Messages: msgs, Stream: stream, Tools: tools}
}

func (o *OllamaProvider) doPost(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", o.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama unreachable at %s: %w", o.endpoint, err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(b))
	}
	return resp, nil
}

func (o *OllamaProvider) StreamMessage(ctx context.Context, req *Request, cb StreamCallback) error {
	oaReq := toOpenAI(req, true)
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
		args strings.Builder
	}
	pending := map[int]*pendingTool{}
	textIndex := 0
	textStarted := false
	var totalIn, totalOut int

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
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

func (o *OllamaProvider) SendMessage(ctx context.Context, req *Request) (*Response, error) {
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

// ListModels queries Ollama's /api/tags for locally pulled models.
func (o *OllamaProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", o.endpoint+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama unreachable at %s: %w", o.endpoint, err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	var models []ModelInfo
	for _, m := range result.Models {
		models = append(models, ModelInfo{ID: m.Name, DisplayName: m.Name})
	}
	return models, nil
}
