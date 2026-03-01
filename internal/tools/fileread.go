package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type FileReadTool struct{}

func NewFileReadTool() *FileReadTool { return &FileReadTool{} }

func (t *FileReadTool) Name() string { return "file_read" }

func (t *FileReadTool) Description() string {
	return "Read a file and return its contents with line numbers. Supports optional offset and limit for large files."
}

func (t *FileReadTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Absolute path to the file to read"
			},
			"offset": {
				"type": "integer",
				"description": "Line number to start from (1-based, default 1)"
			},
			"limit": {
				"type": "integer",
				"description": "Max number of lines to read (default: all)"
			}
		},
		"required": ["path"]
	}`)
}

func (t *FileReadTool) Execute(ctx context.Context, input json.RawMessage) ToolResult {
	var params struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}

	data, err := os.ReadFile(params.Path)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("read error: %v", err), IsError: true}
	}

	lines := strings.Split(string(data), "\n")

	offset := params.Offset
	if offset < 1 {
		offset = 1
	}
	start := offset - 1
	if start >= len(lines) {
		return ToolResult{Output: "(offset beyond end of file)"}
	}

	end := len(lines)
	if params.Limit > 0 && start+params.Limit < end {
		end = start + params.Limit
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		fmt.Fprintf(&b, "%6d\t%s\n", i+1, lines[i])
	}

	return ToolResult{Output: b.String()}
}
