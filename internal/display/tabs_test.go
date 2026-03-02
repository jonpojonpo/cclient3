package display

import "testing"

func TestNewTabManager(t *testing.T) {
	tm := NewTabManager()
	if tm.Count() != 1 {
		t.Fatalf("expected 1 tab, got %d", tm.Count())
	}
	if tm.Active().Kind != TabChat {
		t.Fatal("expected chat tab to be active")
	}
	if tm.ActiveIdx() != 0 {
		t.Fatal("expected active index 0")
	}
}

func TestAddAndFindTab(t *testing.T) {
	tm := NewTabManager()
	idx := tm.Add(&Tab{ID: "agent-1", Label: "agent-1", Kind: TabAgent, Status: TabRunning})
	if idx != 1 {
		t.Fatalf("expected index 1, got %d", idx)
	}
	if tm.Count() != 2 {
		t.Fatalf("expected 2 tabs, got %d", tm.Count())
	}

	tab, i := tm.FindByID("agent-1")
	if tab == nil || i != 1 {
		t.Fatal("FindByID failed")
	}

	tab, i = tm.FindByID("nonexistent")
	if tab != nil || i != -1 {
		t.Fatal("FindByID should return nil,-1 for missing")
	}
}

func TestRemoveTab(t *testing.T) {
	tm := NewTabManager()
	tm.Add(&Tab{ID: "a1", Kind: TabAgent})
	tm.Add(&Tab{ID: "a2", Kind: TabAgent})

	// Can't remove chat
	if tm.Remove("chat") {
		t.Fatal("should not remove chat tab")
	}

	// Remove middle tab while viewing it
	tm.SetActive(1)
	if !tm.Remove("a1") {
		t.Fatal("should remove a1")
	}
	if tm.Count() != 2 {
		t.Fatalf("expected 2 tabs, got %d", tm.Count())
	}
	if tm.ActiveIdx() != 0 {
		t.Fatalf("expected active to fall back to 0, got %d", tm.ActiveIdx())
	}
}

func TestRemoveLastTab(t *testing.T) {
	tm := NewTabManager()
	tm.Add(&Tab{ID: "a1", Kind: TabAgent})
	tm.SetActive(1)
	if !tm.Remove("a1") {
		t.Fatal("should remove a1")
	}
	if tm.ActiveIdx() != 0 {
		t.Fatalf("expected active 0 after removing last tab, got %d", tm.ActiveIdx())
	}
}

func TestRemoveTabAfterActive(t *testing.T) {
	tm := NewTabManager()
	tm.Add(&Tab{ID: "a1", Kind: TabAgent})
	tm.Add(&Tab{ID: "a2", Kind: TabAgent})
	tm.SetActive(1) // viewing a1
	if !tm.Remove("a2") {
		t.Fatal("should remove a2")
	}
	if tm.ActiveIdx() != 1 {
		t.Fatalf("expected active still 1, got %d", tm.ActiveIdx())
	}
}

func TestCycleTabsWrap(t *testing.T) {
	tm := NewTabManager()
	tm.Add(&Tab{ID: "a1", Kind: TabAgent})
	tm.Add(&Tab{ID: "a2", Kind: TabAgent})

	// Forward wrap
	tm.SetActive(2)
	tm.NextTab()
	if tm.ActiveIdx() != 0 {
		t.Fatalf("expected wrap to 0, got %d", tm.ActiveIdx())
	}

	// Backward wrap
	tm.SetActive(0)
	tm.PrevTab()
	if tm.ActiveIdx() != 2 {
		t.Fatalf("expected wrap to 2, got %d", tm.ActiveIdx())
	}
}

func TestSetActiveOutOfRange(t *testing.T) {
	tm := NewTabManager()
	if tm.SetActive(5) {
		t.Fatal("should return false for out of range")
	}
	if tm.SetActive(-1) {
		t.Fatal("should return false for negative")
	}
}

func TestUnreadCleared(t *testing.T) {
	tm := NewTabManager()
	tm.Add(&Tab{ID: "a1", Kind: TabAgent, Unread: true})
	tm.SetActive(1)
	if tm.tabs[1].Unread {
		t.Fatal("SetActive should clear unread")
	}
}

func TestChatTabAlwaysPresent(t *testing.T) {
	tm := NewTabManager()
	chat := tm.ChatTab()
	if chat.ID != "chat" || chat.Kind != TabChat {
		t.Fatal("ChatTab should return the chat tab")
	}
}
