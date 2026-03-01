package display

import "github.com/jonpo/cclient3/internal/tools"

// Bubbletea message types for the TUI.

type TextDeltaMsg struct {
	Text string
}

type ThinkingDeltaMsg struct {
	Thinking string
}

type ToolStartMsg struct {
	ID   string
	Name string
}

type ToolResultMsg struct {
	Result tools.ToolCallResult
}

type StreamDoneMsg struct{}

type ErrorMsg struct {
	Err error
}

type TokenUpdateMsg struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

type SpinnerTickMsg struct{}

type SetThemeMsg struct {
	Name string
}

type SetModelMsg struct {
	Name string
}

type ClearMsg struct{}

type StatusMsg struct {
	Text string
}

type ModelsListMsg struct {
	Models []string
}

type QuitMsg struct{}

// ContextTrimMsg is sent when old messages are dropped to fit the context window.
type ContextTrimMsg struct {
	Dropped int
}

// ConfirmRequestMsg asks the user to confirm a potentially risky action.
type ConfirmRequestMsg struct {
	Prompt  string // human-readable description
	Command string // the exact command/action to confirm
}

// SubAgentStartMsg is sent when a sub-agent is spawned.
type SubAgentStartMsg struct {
	ID    string
	Task  string
	Model string
}

// SubAgentStepMsg is sent each time a sub-agent calls a tool.
type SubAgentStepMsg struct {
	ID       string
	ToolName string
}

// SubAgentDoneMsg is sent when a sub-agent finishes.
type SubAgentDoneMsg struct {
	ID      string
	IsError bool
}
