package api

import "encoding/json"

// --- Request types ---

// CacheControl marks a block for prompt caching.
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type Request struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature,omitempty"`
	System      interface{}     `json:"system,omitempty"` // string or []SystemBlock
	Messages    []Message       `json:"messages"`
	Tools       []ToolDef       `json:"tools,omitempty"`
	Stream      bool            `json:"stream"`
}

// SystemBlock is a content block for the system prompt (used when cache_control is needed).
type SystemBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []ContentBlock
}

type ContentBlock struct {
	Type string `json:"type"`

	// text
	Text string `json:"text,omitempty"`

	// thinking
	Thinking string `json:"thinking,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`

	// prompt caching
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

type ToolDef struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	InputSchema  interface{}   `json:"input_schema"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// --- Response types ---

type Response struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Role         string          `json:"role"`
	Content      []ResponseBlock `json:"content"`
	Model        string          `json:"model"`
	StopReason   string          `json:"stop_reason"`
	StopSequence *string         `json:"stop_sequence"`
	Usage        Usage           `json:"usage"`
}

type ResponseBlock struct {
	Type string `json:"type"`

	// text block
	Text string `json:"text,omitempty"`

	// thinking block
	Thinking string `json:"thinking,omitempty"`

	// tool_use block
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// --- Streaming event types ---

type StreamEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"-"`
}

type MessageStartEvent struct {
	Type    string   `json:"type"`
	Message Response `json:"message"`
}

type ContentBlockStartEvent struct {
	Type         string       `json:"type"`
	Index        int          `json:"index"`
	ContentBlock ResponseBlock `json:"content_block"`
}

type ContentBlockDeltaEvent struct {
	Type  string     `json:"type"`
	Index int        `json:"index"`
	Delta DeltaBlock `json:"delta"`
}

type DeltaBlock struct {
	Type string `json:"type"`

	// text_delta
	Text string `json:"text,omitempty"`

	// thinking_delta
	Thinking string `json:"thinking,omitempty"`

	// input_json_delta
	PartialJSON string `json:"partial_json,omitempty"`
}

type ContentBlockStopEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

type MessageDeltaEvent struct {
	Type  string       `json:"type"`
	Delta MessageDelta `json:"delta"`
	Usage *Usage       `json:"usage,omitempty"`
}

type MessageDelta struct {
	StopReason   string  `json:"stop_reason,omitempty"`
	StopSequence *string `json:"stop_sequence,omitempty"`
}

type MessageStopEvent struct {
	Type string `json:"type"`
}

// --- Models endpoint types ---

// ModelInfo represents a model from the /models endpoint.
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
	Type        string `json:"type"`
}

type ModelsResponse struct {
	Data    []ModelInfo `json:"data"`
	HasMore bool        `json:"has_more"`
	FirstID string      `json:"first_id"`
	LastID  string      `json:"last_id"`
}
