package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type BashTool struct {
	timeout time.Duration
}

func NewBashTool(timeoutSecs int) *BashTool {
	if timeoutSecs <= 0 {
		timeoutSecs = 120
	}
	return &BashTool{timeout: time.Duration(timeoutSecs) * time.Second}
}

func (t *BashTool) Name() string { return "bash" }

func (t *BashTool) Description() string {
	return "Execute a bash command. Each invocation runs in a fresh shell (enables parallel execution). Returns stdout and stderr."
}

func (t *BashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The bash command to execute"
			}
		},
		"required": ["command"]
	}`)
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage) ToolResult {
	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}
	if params.Command == "" {
		return ToolResult{Error: "command is required", IsError: true}
	}

	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", params.Command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if ctx.Err() == context.DeadlineExceeded {
		return ToolResult{Error: fmt.Sprintf("command timed out after %v\n%s", t.timeout, output), IsError: true}
	}

	if err != nil {
		if output == "" {
			output = err.Error()
		}
		return ToolResult{Output: output, IsError: true}
	}

	if output == "" {
		output = "(no output)"
	}

	lang := DetectLangFromCommand(params.Command)
	if lang == "" {
		lang = DetectLangFromContent(output)
	}

	return ToolResult{Output: output, Lang: lang}
}
