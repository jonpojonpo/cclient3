package api

import (
	"os"
	"strings"
	"testing"
)

// testCallback records events for assertions.
type testCallback struct {
	messageStarted  bool
	textDeltas      []string
	thinkingDeltas  []string
	jsonDeltas      []string
	blockStarts     []ResponseBlock
	blockStops      []int
	messageStopped  bool
	errors          []error
	usage           *Usage
}

func (c *testCallback) OnMessageStart(msg Response) { c.messageStarted = true }
func (c *testCallback) OnContentBlockStart(index int, block ResponseBlock) {
	c.blockStarts = append(c.blockStarts, block)
}
func (c *testCallback) OnTextDelta(index int, text string) {
	c.textDeltas = append(c.textDeltas, text)
}
func (c *testCallback) OnThinkingDelta(index int, thinking string) {
	c.thinkingDeltas = append(c.thinkingDeltas, thinking)
}
func (c *testCallback) OnInputJSONDelta(index int, partialJSON string) {
	c.jsonDeltas = append(c.jsonDeltas, partialJSON)
}
func (c *testCallback) OnContentBlockStop(index int) {
	c.blockStops = append(c.blockStops, index)
}
func (c *testCallback) OnMessageDelta(delta MessageDelta, usage *Usage) {
	c.usage = usage
}
func (c *testCallback) OnMessageStop() { c.messageStopped = true }
func (c *testCallback) OnError(err error) {
	c.errors = append(c.errors, err)
}

func TestParseSSEStream_Basic(t *testing.T) {
	data, err := os.ReadFile("../../testdata/stream_basic.txt")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	cb := &testCallback{}
	err = ParseSSEStream(strings.NewReader(string(data)), cb)
	if err != nil {
		t.Fatalf("ParseSSEStream: %v", err)
	}

	if !cb.messageStarted {
		t.Error("expected message_start")
	}
	if !cb.messageStopped {
		t.Error("expected message_stop")
	}
	if len(cb.textDeltas) != 2 {
		t.Errorf("expected 2 text deltas, got %d", len(cb.textDeltas))
	}
	if cb.textDeltas[0] != "Hello" {
		t.Errorf("first delta: got %q, want %q", cb.textDeltas[0], "Hello")
	}
	if cb.textDeltas[1] != " world!" {
		t.Errorf("second delta: got %q, want %q", cb.textDeltas[1], " world!")
	}
	if len(cb.blockStarts) != 1 {
		t.Errorf("expected 1 block start, got %d", len(cb.blockStarts))
	}
	if len(cb.blockStops) != 1 {
		t.Errorf("expected 1 block stop, got %d", len(cb.blockStops))
	}
	if len(cb.errors) > 0 {
		t.Errorf("unexpected errors: %v", cb.errors)
	}
}

func TestParseSSEStream_ToolUse(t *testing.T) {
	data, err := os.ReadFile("../../testdata/stream_tool_use.txt")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	cb := &testCallback{}
	err = ParseSSEStream(strings.NewReader(string(data)), cb)
	if err != nil {
		t.Fatalf("ParseSSEStream: %v", err)
	}

	if len(cb.textDeltas) != 1 {
		t.Errorf("expected 1 text delta, got %d", len(cb.textDeltas))
	}
	if cb.textDeltas[0] != "Let me check." {
		t.Errorf("text delta: got %q", cb.textDeltas[0])
	}
	if len(cb.jsonDeltas) != 2 {
		t.Errorf("expected 2 json deltas, got %d", len(cb.jsonDeltas))
	}

	fullJSON := strings.Join(cb.jsonDeltas, "")
	if fullJSON != `{"command": "ls"}` {
		t.Errorf("assembled JSON: got %q", fullJSON)
	}

	if len(cb.blockStarts) != 2 {
		t.Errorf("expected 2 block starts, got %d", len(cb.blockStarts))
	}
	if cb.blockStarts[1].Type != "tool_use" {
		t.Errorf("second block type: got %q, want 'tool_use'", cb.blockStarts[1].Type)
	}
	if cb.blockStarts[1].Name != "bash" {
		t.Errorf("tool name: got %q, want 'bash'", cb.blockStarts[1].Name)
	}
}
