package agent

import (
	"encoding/json"
	"fmt"
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

// DefaultBashSafetyHook blocks dangerous bash commands.
func DefaultBashSafetyHook() PreToolUseHook {
	dangerous := []string{
		"rm -rf /",
		"rm -rf /*",
		"dd if=",
		"mkfs",
		":(){ :|:& };:",
		"> /dev/sda",
		"chmod -R 777 /",
		"mv / ",
	}

	// Patterns for piped execution: "wget ... | sh", "curl ... | bash" etc.
	pipePatterns := []struct {
		prefix string
		suffix string
	}{
		{"wget ", "| sh"},
		{"wget ", "| bash"},
		{"curl ", "| sh"},
		{"curl ", "| bash"},
	}

	return func(call tools.ToolCall) string {
		var params struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(call.Input, &params); err != nil {
			return ""
		}
		cmd := strings.ToLower(strings.TrimSpace(params.Command))
		for _, d := range dangerous {
			if strings.Contains(cmd, d) {
				return fmt.Sprintf("BLOCKED: dangerous command detected (%s)", d)
			}
		}
		for _, p := range pipePatterns {
			if strings.Contains(cmd, p.prefix) && strings.Contains(cmd, p.suffix) {
				return fmt.Sprintf("BLOCKED: dangerous pipe pattern (%s...%s)", p.prefix, p.suffix)
			}
		}
		return ""
	}
}
