package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type FileWriteTool struct{}

func NewFileWriteTool() *FileWriteTool { return &FileWriteTool{} }

func (t *FileWriteTool) Name() string { return "file_write" }

func (t *FileWriteTool) Description() string {
	return "Create or overwrite a file with the given content. Creates parent directories as needed."
}

func (t *FileWriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to the file to write (absolute or relative to cwd)"
			},
			"content": {
				"type": "string",
				"description": "Content to write to the file"
			}
		},
		"required": ["path", "content"]
	}`)
}

func (t *FileWriteTool) Execute(ctx context.Context, input json.RawMessage) ToolResult {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}

	path := params.Path
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return ToolResult{Error: fmt.Sprintf("resolve path: %v", err), IsError: true}
		}
		path = abs
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ToolResult{Error: fmt.Sprintf("create directory: %v", err), IsError: true}
	}

	if err := os.WriteFile(path, []byte(params.Content), 0644); err != nil {
		return ToolResult{Error: fmt.Sprintf("write error: %v", err), IsError: true}
	}

	return ToolResult{Output: fmt.Sprintf("Wrote %d bytes to %s", len(params.Content), path)}
}
