package agent

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonpo/cclient3/internal/display"
)

// newTabMsgChan creates a channel that rewrites display messages to include
// a tab ID prefix, then forwards them to the shared AgentMsgChan.
// The caller should close the returned channel when done.
func newTabMsgChan(tabID string, dest chan tea.Msg) chan tea.Msg {
	ch := make(chan tea.Msg, 100)

	go func() {
		for msg := range ch {
			switch m := msg.(type) {
			case display.TextDeltaMsg:
				dest <- display.TabTextDeltaMsg{TabID: tabID, Text: m.Text}
			case display.ThinkingDeltaMsg:
				dest <- display.TabThinkingDeltaMsg{TabID: tabID, Thinking: m.Thinking}
			case display.StreamDoneMsg:
				dest <- display.TabStreamDoneMsg{TabID: tabID}
			case display.ErrorMsg:
				dest <- display.TabErrorMsg{TabID: tabID, Err: m.Err}
			default:
				// Pass through unrecognized messages (shouldn't happen)
				dest <- msg
			}
		}
	}()

	return ch
}
