package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

type FileEditTool struct {
	mu      sync.Mutex
	history map[string][]string // path -> previous contents for undo
}

func NewFileEditTool() *FileEditTool {
	return &FileEditTool{history: make(map[string][]string)}
}

func (t *FileEditTool) Name() string { return "file_edit" }

func (t *FileEditTool) Description() string {
	return `Edit files using str_replace. Commands: str_replace (find and replace unique text), insert (insert text at line), undo_edit (undo last edit). All edits require the old_string to be unique in the file.`
}

func (t *FileEditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"enum": ["str_replace", "insert", "undo_edit"],
				"description": "The edit command"
			},
			"path": {
				"type": "string",
				"description": "Absolute path to the file"
			},
			"old_string": {
				"type": "string",
				"description": "Text to find (must be unique). Required for str_replace"
			},
			"new_string": {
				"type": "string",
				"description": "Replacement text. Required for str_replace and insert"
			},
			"insert_line": {
				"type": "integer",
				"description": "Line number to insert after. Required for insert"
			}
		},
		"required": ["command", "path"]
	}`)
}

func (t *FileEditTool) Execute(ctx context.Context, input json.RawMessage) ToolResult {
	var params struct {
		Command    string `json:"command"`
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		InsertLine int    `json:"insert_line"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}

	switch params.Command {
	case "str_replace":
		return t.strReplace(params.Path, params.OldString, params.NewString)
	case "insert":
		return t.insert(params.Path, params.InsertLine, params.NewString)
	case "undo_edit":
		return t.undoEdit(params.Path)
	default:
		return ToolResult{Error: fmt.Sprintf("unknown command: %s", params.Command), IsError: true}
	}
}

func (t *FileEditTool) strReplace(path, oldStr, newStr string) ToolResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("read error: %v", err), IsError: true}
	}

	content := string(data)
	count := strings.Count(content, oldStr)

	if count == 0 {
		return ToolResult{Error: "old_string not found in file", IsError: true}
	}
	if count > 1 {
		return ToolResult{Error: fmt.Sprintf("old_string found %d times — must be unique. Add more surrounding context", count), IsError: true}
	}

	t.pushHistory(path, content)
	newContent := strings.Replace(content, oldStr, newStr, 1)

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return ToolResult{Error: fmt.Sprintf("write error: %v", err), IsError: true}
	}

	return ToolResult{Output: t.snippet(newContent, newStr)}
}

func (t *FileEditTool) insert(path string, line int, text string) ToolResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("read error: %v", err), IsError: true}
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	if line < 0 || line > len(lines) {
		return ToolResult{Error: fmt.Sprintf("insert_line %d out of range (0-%d)", line, len(lines)), IsError: true}
	}

	t.pushHistory(path, content)

	insertLines := strings.Split(text, "\n")
	newLines := make([]string, 0, len(lines)+len(insertLines))
	newLines = append(newLines, lines[:line]...)
	newLines = append(newLines, insertLines...)
	newLines = append(newLines, lines[line:]...)

	newContent := strings.Join(newLines, "\n")
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return ToolResult{Error: fmt.Sprintf("write error: %v", err), IsError: true}
	}

	return ToolResult{Output: fmt.Sprintf("Inserted %d lines after line %d", len(insertLines), line)}
}

func (t *FileEditTool) undoEdit(path string) ToolResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	stack, ok := t.history[path]
	if !ok || len(stack) == 0 {
		return ToolResult{Error: "no edits to undo for this file", IsError: true}
	}

	prev := stack[len(stack)-1]
	t.history[path] = stack[:len(stack)-1]

	if err := os.WriteFile(path, []byte(prev), 0644); err != nil {
		return ToolResult{Error: fmt.Sprintf("write error: %v", err), IsError: true}
	}

	return ToolResult{Output: "Undo successful"}
}

func (t *FileEditTool) pushHistory(path, content string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.history[path] = append(t.history[path], content)
}

func (t *FileEditTool) snippet(content, target string) string {
	lines := strings.Split(content, "\n")
	targetStart := -1
	for i, l := range lines {
		if strings.Contains(l, strings.Split(target, "\n")[0]) {
			targetStart = i
			break
		}
	}
	if targetStart < 0 {
		return "Edit applied successfully"
	}

	start := targetStart - 4
	if start < 0 {
		start = 0
	}
	end := targetStart + strings.Count(target, "\n") + 5
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		fmt.Fprintf(&b, "%6d\t%s\n", i+1, lines[i])
	}
	return b.String()
}
