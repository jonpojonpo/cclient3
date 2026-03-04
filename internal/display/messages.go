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
	ID       string
	Task     string
	Model    string
	Provider string // e.g. "anthropic", "ollama"
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

// --- Tab-routed messages (from sub-agents streaming into their own tabs) ---

// TabTextDeltaMsg routes a text delta to a specific agent tab.
type TabTextDeltaMsg struct {
	TabID string
	Text  string
}

// TabThinkingDeltaMsg routes a thinking delta to a specific agent tab.
type TabThinkingDeltaMsg struct {
	TabID    string
	Thinking string
}

// TabToolStartMsg notifies that a tool execution started in a sub-agent tab.
type TabToolStartMsg struct {
	TabID string
	Name  string
	ID    string
}

// TabToolResultMsg delivers a tool result to a sub-agent tab.
type TabToolResultMsg struct {
	TabID  string
	Name   string
	ID     string
	Result tools.ToolResult
}

// TabStreamDoneMsg signals that a streaming turn completed in a sub-agent tab.
type TabStreamDoneMsg struct {
	TabID string
}

// TabErrorMsg delivers an error to a sub-agent tab.
type TabErrorMsg struct {
	TabID string
	Err   error
}

// --- Bash tab messages ---

// BashOutputMsg delivers new output lines from a bash session to a tab.
type BashOutputMsg struct {
	TabID string
	Lines []string
}

// --- Ensemble tab messages ---

// EnsembleAgentInfo describes an agent in the ensemble roster.
type EnsembleAgentInfo struct {
	Name        string
	Model       string
	Provider    string
	Personality string
	Color       string
}

// EnsembleStartMsg creates the ensemble tab and shows the agent roster.
type EnsembleStartMsg struct {
	TabID  string
	Agents []EnsembleAgentInfo
	Prompt string
}

// EnsembleUserMsg shows a user message in the ensemble transcript.
type EnsembleUserMsg struct {
	TabID string
	Text  string
}

// EnsembleSpeakerMsg signals a new agent is about to stream.
type EnsembleSpeakerMsg struct {
	TabID   string
	Speaker string
	Color   string
	Model   string
}

// EnsembleTextDeltaMsg streams text from an agent into the ensemble tab.
type EnsembleTextDeltaMsg struct {
	TabID   string
	Speaker string
	Text    string
	Color   string
}

// EnsembleTurnDoneMsg signals an agent finished their response.
type EnsembleTurnDoneMsg struct {
	TabID   string
	Speaker string
}

// EnsembleRoundDoneMsg signals all agents finished a round, waiting for user.
type EnsembleRoundDoneMsg struct {
	TabID string
}

// EnsembleDoneMsg signals the ensemble session ended.
type EnsembleDoneMsg struct {
	TabID   string
	Summary string
}

// EnsembleErrorMsg delivers an error in the ensemble tab.
type EnsembleErrorMsg struct {
	TabID string
	Err   error
}
