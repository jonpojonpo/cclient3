package agent

import (
	"strings"
	"testing"

	"github.com/jonpo/cclient3/internal/api"
)

// TestTrimForWindow_NoOrphanedToolResults verifies that trimming never leaves
// a tool_result message whose matching tool_use assistant message was dropped —
// the API rejects such conversations.
func TestTrimForWindow_NoOrphanedToolResults(t *testing.T) {
	c := NewConversation()
	filler := strings.Repeat("x", 4000) // ~1000 tokens per message

	c.AddUser("initial " + filler)
	c.AddAssistant([]api.ContentBlock{{Type: "text", Text: filler}})

	// Several tool-use turns
	for i := 0; i < 6; i++ {
		c.AddUser("question " + filler)
		c.AddAssistant([]api.ContentBlock{
			{Type: "text", Text: filler},
			{Type: "tool_use", ID: "t1", Name: "bash"},
		})
		c.AddToolResults([]api.ContentBlock{
			{Type: "tool_result", ToolUseID: "t1", Content: filler},
		})
	}

	dropped := c.TrimForWindow(8000)
	if dropped == 0 {
		t.Fatal("expected messages to be dropped")
	}

	// Walk the surviving conversation: every user message made of tool_result
	// blocks must be directly preceded by an assistant message with tool_use.
	for i, msg := range c.Messages {
		if !startsWithToolResult(msg) {
			continue
		}
		if i == 0 {
			t.Fatal("conversation starts with an orphaned tool_result")
		}
		prev := c.Messages[i-1]
		blocks, ok := prev.Content.([]api.ContentBlock)
		if !ok || prev.Role != "assistant" {
			t.Fatalf("tool_result at %d not preceded by assistant blocks", i)
		}
		hasToolUse := false
		for _, b := range blocks {
			if b.Type == "tool_use" {
				hasToolUse = true
			}
		}
		if !hasToolUse {
			t.Fatalf("tool_result at %d preceded by assistant message without tool_use", i)
		}
	}
}
