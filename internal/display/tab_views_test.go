package display

import (
	"fmt"
	"strings"
	"testing"
)

func newTestModel() *Model {
	m := NewModel("cyber", "test-model")
	m.width = 120
	m.height = 40
	return m
}

func TestRenderTabBarSingleTab(t *testing.T) {
	m := newTestModel()
	// Only 1 tab (Chat) — tab bar is not shown (handled by View), but renderTabBar should still work
	bar := m.renderTabBar()
	if !strings.Contains(bar, "1: Chat") {
		t.Fatalf("expected Chat tab in bar, got: %s", bar)
	}
}

func TestRenderTabBarMultipleTabs(t *testing.T) {
	m := newTestModel()
	m.tabs.Add(&Tab{ID: "agent-1", Label: "agent-1", Kind: TabAgent, Status: TabRunning})
	m.tabs.Add(&Tab{ID: "agent-2", Label: "agent-2", Kind: TabAgent, Status: TabDone})

	bar := m.renderTabBar()
	if !strings.Contains(bar, "1: Chat") {
		t.Fatalf("expected Chat tab, got: %s", bar)
	}
	if !strings.Contains(bar, "2: agent-1") {
		t.Fatalf("expected agent-1 tab, got: %s", bar)
	}
	if !strings.Contains(bar, "3: agent-2") {
		t.Fatalf("expected agent-2 tab, got: %s", bar)
	}
}

func TestRenderStatusIcons(t *testing.T) {
	m := newTestModel()

	// Chat tab — no icon
	chatIcon := m.renderStatusIcon(m.chatTab())
	if chatIcon != "" {
		t.Fatalf("Chat tab should have no icon, got: %q", chatIcon)
	}

	// Running agent
	running := &Tab{Kind: TabAgent, Status: TabRunning}
	icon := m.renderStatusIcon(running)
	if !strings.Contains(icon, "●") {
		t.Fatalf("expected ● for idle running tab, got: %q", icon)
	}

	// Running + actively streaming
	running.state = stateStreaming
	icon = m.renderStatusIcon(running)
	// Should be a spinner frame character
	if icon == "" {
		t.Fatal("expected spinner icon for streaming tab")
	}

	// Done
	done := &Tab{Kind: TabAgent, Status: TabDone}
	icon = m.renderStatusIcon(done)
	if !strings.Contains(icon, "✓") {
		t.Fatalf("expected ✓ for done tab, got: %q", icon)
	}

	// Error
	errTab := &Tab{Kind: TabAgent, Status: TabError}
	icon = m.renderStatusIcon(errTab)
	if !strings.Contains(icon, "✗") {
		t.Fatalf("expected ✗ for error tab, got: %q", icon)
	}
}

func TestRenderTabBarOverflow(t *testing.T) {
	m := newTestModel()
	m.width = 60 // narrow terminal

	// Add many tabs to force overflow
	for i := 0; i < 10; i++ {
		m.tabs.Add(&Tab{
			ID:    fmt.Sprintf("agent-%d", i),
			Label: fmt.Sprintf("agent-%d", i),
			Kind:  TabAgent,
			Status: TabRunning,
		})
	}

	// Switch to a middle tab
	m.tabs.SetActive(5)

	bar := m.renderTabBar()
	// Should contain the active tab
	if !strings.Contains(bar, "6: agent-4") {
		t.Fatalf("expected active tab (6: agent-4) in overflow bar, got: %s", bar)
	}
}

func TestViewContainsTabBar(t *testing.T) {
	m := newTestModel()
	output := m.View()
	if !strings.Contains(output, "1: Chat") {
		t.Fatalf("View() should contain tab bar with '1: Chat', got:\n%s", output)
	}
	// Banner should also be present
	if !strings.Contains(output, "cclient3") && !strings.Contains(output, "CCLIENT3") && !strings.Contains(output, "claude client") {
		t.Fatalf("View() should contain banner, got:\n%s", output)
	}
}

func TestRenderTabUnreadBadge(t *testing.T) {
	m := newTestModel()
	m.tabs.Add(&Tab{ID: "a1", Label: "a1", Kind: TabAgent, Status: TabRunning, Unread: true})

	// Active tab is Chat (0), so a1 (index 1) is inactive with unread
	bar := m.renderTabBar()
	if !strings.Contains(bar, "•") {
		t.Fatalf("expected unread badge in bar, got: %s", bar)
	}
}
