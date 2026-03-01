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
