package agent

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonpo/cclient3/internal/display"
)

func TestTabMsgChan_RewritesTextDelta(t *testing.T) {
	dest := make(chan tea.Msg, 10)
	ch := newTabMsgChan("agent-1", dest)

	ch <- display.TextDeltaMsg{Text: "hello"}
	close(ch)

	select {
	case msg := <-dest:
		tabMsg, ok := msg.(display.TabTextDeltaMsg)
		if !ok {
			t.Fatalf("expected TabTextDeltaMsg, got %T", msg)
		}
		if tabMsg.TabID != "agent-1" {
			t.Fatalf("expected TabID agent-1, got %s", tabMsg.TabID)
		}
		if tabMsg.Text != "hello" {
			t.Fatalf("expected text hello, got %s", tabMsg.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestTabMsgChan_RewritesThinkingDelta(t *testing.T) {
	dest := make(chan tea.Msg, 10)
	ch := newTabMsgChan("agent-2", dest)

	ch <- display.ThinkingDeltaMsg{Thinking: "hmm"}
	close(ch)

	select {
	case msg := <-dest:
		tabMsg, ok := msg.(display.TabThinkingDeltaMsg)
		if !ok {
			t.Fatalf("expected TabThinkingDeltaMsg, got %T", msg)
		}
		if tabMsg.TabID != "agent-2" {
			t.Fatalf("expected TabID agent-2, got %s", tabMsg.TabID)
		}
		if tabMsg.Thinking != "hmm" {
			t.Fatalf("expected thinking hmm, got %s", tabMsg.Thinking)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestTabMsgChan_RewritesStreamDone(t *testing.T) {
	dest := make(chan tea.Msg, 10)
	ch := newTabMsgChan("agent-3", dest)

	ch <- display.StreamDoneMsg{}
	close(ch)

	select {
	case msg := <-dest:
		tabMsg, ok := msg.(display.TabStreamDoneMsg)
		if !ok {
			t.Fatalf("expected TabStreamDoneMsg, got %T", msg)
		}
		if tabMsg.TabID != "agent-3" {
			t.Fatalf("expected TabID agent-3, got %s", tabMsg.TabID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestTabMsgChan_RewritesError(t *testing.T) {
	dest := make(chan tea.Msg, 10)
	ch := newTabMsgChan("agent-4", dest)

	ch <- display.ErrorMsg{Err: fmt.Errorf("test error")}
	close(ch)

	select {
	case msg := <-dest:
		tabMsg, ok := msg.(display.TabErrorMsg)
		if !ok {
			t.Fatalf("expected TabErrorMsg, got %T", msg)
		}
		if tabMsg.TabID != "agent-4" {
			t.Fatalf("expected TabID agent-4, got %s", tabMsg.TabID)
		}
		if tabMsg.Err.Error() != "test error" {
			t.Fatalf("expected error 'test error', got %s", tabMsg.Err.Error())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}
