package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jonpo/cclient3/internal/tools"
)

// PreToolUseHook can inspect and optionally block a tool call.
// Return non-empty string to deny (the string is the deny reason).
type PreToolUseHook func(call tools.ToolCall) string

type HookRegistry struct {
	preToolUse map[string][]PreToolUseHook // tool name -> hooks
}

func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		preToolUse: make(map[string][]PreToolUseHook),
	}
}

func (h *HookRegistry) RegisterPreToolUse(toolName string, hook PreToolUseHook) {
	h.preToolUse[toolName] = append(h.preToolUse[toolName], hook)
}

// CheckPreToolUse runs all hooks for a tool call. Returns deny reason or "".
func (h *HookRegistry) CheckPreToolUse(call tools.ToolCall) string {
	hooks, ok := h.preToolUse[call.Name]
	if !ok {
		return ""
	}
	for _, hook := range hooks {
		if reason := hook(call); reason != "" {
			return reason
		}
	}
	return ""
}

var (
	reWhitespace   = regexp.MustCompile(`\s+`)
	cmdPrefixes    = []string{"sudo ", "env ", "nohup ", "bash -c ", "sh -c "}
)

// normalizeCommand strips common evasion techniques from a command string.
// Removes backslash-escaping within words, collapses whitespace, strips
// leading sudo/env/nohup prefixes, and lowercases everything.
func normalizeCommand(cmd string) string {
	cmd = strings.ToLower(strings.TrimSpace(cmd))

	// Remove backslash escapes within words (r\m -> rm)
	cmd = strings.ReplaceAll(cmd, "\\", "")

	// Remove single/double quotes used to break up commands ('r'm -> rm)
	cmd = strings.ReplaceAll(cmd, "'", "")
	cmd = strings.ReplaceAll(cmd, "\"", "")

	// Collapse whitespace
	cmd = reWhitespace.ReplaceAllString(cmd, " ")

	// Strip common command prefixes (sudo, env, nohup, etc.)
	for {
		trimmed := cmd
		for _, prefix := range cmdPrefixes {
			trimmed = strings.TrimPrefix(trimmed, prefix)
		}
		if trimmed == cmd {
			break
		}
		cmd = trimmed
	}

	return cmd
}

// DefaultBashSafetyHook blocks dangerous bash commands.
func DefaultBashSafetyHook() PreToolUseHook {
	dangerous := []string{
		"rm -rf /",
		"rm -rf /*",
		"dd if=",
		"mkfs",
		":(){ :|:& };:",
		"> /dev/sda",
		"chmod -r 777 /",
		"chmod 777 /",
		"mv / ",
		"rm -rf ~",
	}

	// Regex patterns for more complex evasions
	dangerousPatterns := []*regexp.Regexp{
		// Pipe from network to shell
		regexp.MustCompile(`(curl|wget)\s+.*\|\s*(ba)?sh`),
		// Recursive force-remove on root-like paths
		regexp.MustCompile(`rm\s+(-[a-z]*r[a-z]*\s+(-[a-z]+\s+)*|(-[a-z]+\s+)*-[a-z]*r[a-z]*\s+)/`),
	}

	return func(call tools.ToolCall) string {
		var params struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(call.Input, &params); err != nil {
			return ""
		}
		cmd := normalizeCommand(params.Command)

		for _, d := range dangerous {
			if strings.Contains(cmd, d) {
				return fmt.Sprintf("BLOCKED: dangerous command detected (%s)", d)
			}
		}
		for _, p := range dangerousPatterns {
			if p.MatchString(cmd) {
				return fmt.Sprintf("BLOCKED: dangerous command pattern detected (%s)", p.String())
			}
		}
		return ""
	}
}
