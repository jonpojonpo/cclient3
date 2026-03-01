package tools

import (
	"context"
	"encoding/json"
)

// ToolResult is what a tool returns after execution.
type ToolResult struct {
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
}

// Tool is the interface all tools must implement.
type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) ToolResult
}

// ToolCall represents a tool invocation from the API response.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolCallResult pairs a tool call with its result.
type ToolCallResult struct {
	Call   ToolCall
	Result ToolResult
}
