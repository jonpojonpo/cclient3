package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type GlobTool struct{}

func NewGlobTool() *GlobTool { return &GlobTool{} }

func (t *GlobTool) Name() string { return "glob" }

func (t *GlobTool) Description() string {
	return "Find files matching a glob pattern. Supports ** for recursive matching."
}

func (t *GlobTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Glob pattern (e.g. '**/*.go', 'src/**/*.ts')"
			},
			"path": {
				"type": "string",
				"description": "Base directory to search in (default: current directory)"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GlobTool) Execute(ctx context.Context, input json.RawMessage) ToolResult {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}

	base := params.Path
	if base == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			return ToolResult{Error: fmt.Sprintf("getwd: %v", err), IsError: true}
		}
	}

	var matches []string

	if strings.Contains(params.Pattern, "**") {
		// Walk for recursive glob
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip errors
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			rel, _ := filepath.Rel(base, path)
			matched, _ := filepath.Match(flattenPattern(params.Pattern), rel)
			if matched {
				matches = append(matches, path)
			}
			// Also try matching with just the filename for patterns like **/*.go
			if !matched && !info.IsDir() {
				basePat := filepath.Base(params.Pattern)
				if m, _ := filepath.Match(basePat, info.Name()); m {
					matches = append(matches, path)
				}
			}
			return nil
		})
		if err != nil && err != context.Canceled {
			return ToolResult{Error: fmt.Sprintf("walk error: %v", err), IsError: true}
		}
	} else {
		pattern := filepath.Join(base, params.Pattern)
		var err error
		matches, err = filepath.Glob(pattern)
		if err != nil {
			return ToolResult{Error: fmt.Sprintf("glob error: %v", err), IsError: true}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}

	sort.Strings(unique)

	if len(unique) == 0 {
		return ToolResult{Output: "(no matches)"}
	}

	return ToolResult{Output: strings.Join(unique, "\n")}
}

// flattenPattern removes ** from patterns for simple matching.
func flattenPattern(pattern string) string {
	return strings.ReplaceAll(pattern, "**/", "")
}
