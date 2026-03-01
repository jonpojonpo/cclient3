package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type GrepTool struct{}

func NewGrepTool() *GrepTool { return &GrepTool{} }

func (t *GrepTool) Name() string { return "grep" }

func (t *GrepTool) Description() string {
	return "Search file contents using regex. Uses ripgrep if available, falls back to built-in search."
}

func (t *GrepTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Regex pattern to search for"
			},
			"path": {
				"type": "string",
				"description": "File or directory to search in (default: current directory)"
			},
			"include": {
				"type": "string",
				"description": "Glob filter for files (e.g. '*.go')"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GrepTool) Execute(ctx context.Context, input json.RawMessage) ToolResult {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Include string `json:"include"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}

	searchPath := params.Path
	if searchPath == "" {
		var err error
		searchPath, err = os.Getwd()
		if err != nil {
			return ToolResult{Error: fmt.Sprintf("getwd: %v", err), IsError: true}
		}
	}

	// Try ripgrep first
	if rgPath, err := exec.LookPath("rg"); err == nil {
		return t.ripgrep(ctx, rgPath, params.Pattern, searchPath, params.Include)
	}

	// Fallback to built-in
	return t.builtinGrep(ctx, params.Pattern, searchPath, params.Include)
}

func (t *GrepTool) ripgrep(ctx context.Context, rgPath, pattern, searchPath, include string) ToolResult {
	args := []string{"-n", "--no-heading", "--color=never", "-e", pattern}
	if include != "" {
		args = append(args, "--glob", include)
	}
	args = append(args, searchPath)

	cmd := exec.CommandContext(ctx, rgPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// rg returns exit code 1 for no matches
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return ToolResult{Output: "(no matches)"}
		}
		if stderr.Len() > 0 {
			return ToolResult{Error: stderr.String(), IsError: true}
		}
	}

	output := stdout.String()
	if output == "" {
		return ToolResult{Output: "(no matches)"}
	}
	return ToolResult{Output: output}
}

func (t *GrepTool) builtinGrep(ctx context.Context, pattern, searchPath, include string) ToolResult {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid regex: %v", err), IsError: true}
	}

	var results strings.Builder
	count := 0

	filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if count > 1000 {
			return filepath.SkipAll
		}

		if include != "" {
			if matched, _ := filepath.Match(include, info.Name()); !matched {
				return nil
			}
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				fmt.Fprintf(&results, "%s:%d:%s\n", path, i+1, line)
				count++
			}
		}
		return nil
	})

	if results.Len() == 0 {
		return ToolResult{Output: "(no matches)"}
	}
	return ToolResult{Output: results.String()}
}
