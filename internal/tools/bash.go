package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type BashTool struct {
	freshTimeout time.Duration   // default timeout for fresh-shell invocations
	sessions     *SessionManager // nil if sessions not enabled
}

func NewBashTool(timeoutSecs int, sm *SessionManager) *BashTool {
	if timeoutSecs <= 0 {
		timeoutSecs = 120
	}
	return &BashTool{
		freshTimeout: time.Duration(timeoutSecs) * time.Second,
		sessions:     sm,
	}
}

func (t *BashTool) Name() string { return "bash" }

func (t *BashTool) Description() string {
	return `Execute bash commands in a fresh shell or a named persistent session.

Named sessions preserve cwd, env vars, and background jobs across calls.
Actions: run (default), list_sessions, read_session, kill_session.

Fresh shell (no session): simple, parallel-safe, 120s timeout.
Named session: idle_timeout resets on each output line — perfect for installs/builds
  that keep printing. Hung processes with no output time out in 30s.
Background mode: use background=true + a session to start servers/watchers,
  then read_session to check their output.`
}

func (t *BashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "Command to execute (required for 'run')"
			},
			"session": {
				"type": "string",
				"description": "Named session for persistent state. Omit for a fresh isolated shell."
			},
			"timeout": {
				"type": "integer",
				"description": "Total seconds before giving up (default: 120 fresh / unlimited session). 0 = unlimited."
			},
			"idle_timeout": {
				"type": "integer",
				"description": "Seconds of no output before timing out (default: 30 for sessions, disabled for fresh). Resets on each output line — ideal for package installs."
			},
			"background": {
				"type": "boolean",
				"description": "Start command in background and return immediately. Requires a named session. Use read_session to retrieve output."
			},
			"max_output_lines": {
				"type": "integer",
				"description": "Max lines to return (default: 100 for sessions). Older lines stay in session history."
			},
			"action": {
				"type": "string",
				"enum": ["run", "list_sessions", "read_session", "kill_session"],
				"description": "Action (default: run)"
			}
		}
	}`)
}

type bashParams struct {
	Command        string `json:"command"`
	Session        string `json:"session"`
	Timeout        int    `json:"timeout"`
	IdleTimeout    int    `json:"idle_timeout"`
	Background     bool   `json:"background"`
	MaxOutputLines int    `json:"max_output_lines"`
	Action         string `json:"action"`
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage) ToolResult {
	var p bashParams
	if err := json.Unmarshal(input, &p); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}
	if p.MaxOutputLines <= 0 {
		p.MaxOutputLines = 100
	}
	if p.Action == "" {
		p.Action = "run"
	}

	switch p.Action {
	case "list_sessions":
		return t.listSessions()
	case "read_session":
		return t.readSession(p.Session, p.MaxOutputLines)
	case "kill_session":
		return t.killSession(p.Session)
	default: // "run"
		if p.Command == "" {
			return ToolResult{Error: "command is required", IsError: true}
		}
		if p.Session != "" {
			return t.runInSession(ctx, p)
		}
		return t.runFresh(ctx, p.Command, p.Timeout)
	}
}

// runFresh runs in a brand-new subprocess — original behavior, always parallel-safe.
func (t *BashTool) runFresh(ctx context.Context, command string, timeoutSecs int) ToolResult {
	timeout := t.freshTimeout
	if timeoutSecs > 0 {
		timeout = time.Duration(timeoutSecs) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", command)
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
		return ToolResult{Error: fmt.Sprintf("command timed out after %v\n%s", timeout, output), IsError: true}
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
	return ToolResult{Output: output}
}

// runInSession executes in a persistent named session.
func (t *BashTool) runInSession(ctx context.Context, p bashParams) ToolResult {
	if t.sessions == nil {
		return ToolResult{Error: "session manager not available", IsError: true}
	}

	s, err := t.sessions.acquire(p.Session)
	if err != nil {
		return ToolResult{Error: err.Error(), IsError: true}
	}

	// Compute timeouts.
	// Default for sessions: no total timeout, 30s idle timeout.
	// User can override either.
	var totalTimeout time.Duration
	if p.Timeout > 0 {
		totalTimeout = time.Duration(p.Timeout) * time.Second
	}

	idleTimeout := 30 * time.Second
	if p.IdleTimeout > 0 {
		idleTimeout = time.Duration(p.IdleTimeout) * time.Second
	} else if p.Background {
		idleTimeout = 0 // background: don't wait for output at all
	}

	output, timedOut := s.run(ctx, p.Command, p.Background, totalTimeout, idleTimeout, p.MaxOutputLines)

	tag := fmt.Sprintf("[session:%s]", p.Session)
	if p.Background {
		tag += "[background]"
	}

	if timedOut {
		return ToolResult{Output: tag + " " + output, IsError: true}
	}
	return ToolResult{Output: tag + " " + output}
}

func (t *BashTool) listSessions() ToolResult {
	if t.sessions == nil {
		return ToolResult{Output: "(session manager not available)"}
	}
	infos := t.sessions.List()
	if len(infos) == 0 {
		return ToolResult{Output: "No active sessions"}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-16s %-6s %-10s %s\n", "SESSION", "STATUS", "LAST USED", "LAST COMMAND")
	for _, si := range infos {
		status := "alive"
		if !si.Alive {
			status = "dead"
		}
		cmd := si.LastCommand
		if len(cmd) > 50 {
			cmd = cmd[:47] + "..."
		}
		fmt.Fprintf(&b, "%-16s %-6s %-10s %s\n", si.Name, status, si.LastUsedAt.Format("15:04:05"), cmd)
	}
	return ToolResult{Output: b.String()}
}

func (t *BashTool) readSession(name string, maxLines int) ToolResult {
	if t.sessions == nil {
		return ToolResult{Error: "session manager not available", IsError: true}
	}
	if name == "" {
		return ToolResult{Error: "session name required", IsError: true}
	}
	lines, ok := t.sessions.ReadHistory(name, maxLines)
	if !ok {
		return ToolResult{Error: fmt.Sprintf("session %q not found", name), IsError: true}
	}
	if len(lines) == 0 {
		return ToolResult{Output: "(no output yet)"}
	}
	return ToolResult{Output: strings.Join(lines, "\n")}
}

func (t *BashTool) killSession(name string) ToolResult {
	if t.sessions == nil {
		return ToolResult{Error: "session manager not available", IsError: true}
	}
	if name == "" {
		return ToolResult{Error: "session name required", IsError: true}
	}
	if t.sessions.Kill(name) {
		return ToolResult{Output: fmt.Sprintf("session %q terminated", name)}
	}
	return ToolResult{Error: fmt.Sprintf("session %q not found", name), IsError: true}
}
