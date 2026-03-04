package display

import "strings"

// TabKind identifies the type of a tab.
type TabKind int

const (
	TabChat     TabKind = iota // always-present main chat
	TabAgent                   // sub-agent streaming output
	TabBash                    // persistent bash session viewer
	TabEnsemble                // ensemble group chat
)

// TabStatus tracks the lifecycle of an agent/bash tab.
type TabStatus int

const (
	TabRunning TabStatus = iota
	TabDone
	TabError
)

// Tab holds the per-tab state for history, streaming, and scrolling.
type Tab struct {
	ID    string
	Label string
	Kind  TabKind

	// Agent/bash lifecycle status
	Status TabStatus

	// Scrollable history (rendered panels)
	history      []historyEntry
	scrollOffset int

	// Per-tab streaming state (mirrors what Model used to hold globally)
	state           displayState
	currentText     strings.Builder
	currentThinking strings.Builder

	// Bash tabs: rolling line buffer
	outputLines []string
	sessionName string

	// User pinned this tab (prevents auto-switch away)
	UserPinned bool

	// Has unread content since last viewed
	Unread bool

	// Ensemble tab fields
	EnsembleInputChan chan string // user messages into ensemble
	currentSpeaker    string     // who is currently streaming
	currentColor      string     // hex color of current speaker
	agentCount        int        // number of agents in the ensemble
}

// TabManager manages the ordered set of tabs.
type TabManager struct {
	tabs       []*Tab
	activeIdx  int
	autoSwitch bool
}

// NewTabManager creates a TabManager with the initial Chat tab.
func NewTabManager() *TabManager {
	chatTab := &Tab{
		ID:    "chat",
		Label: "Chat",
		Kind:  TabChat,
	}
	return &TabManager{
		tabs:       []*Tab{chatTab},
		activeIdx:  0,
		autoSwitch: true,
	}
}

// Active returns the currently active tab.
func (tm *TabManager) Active() *Tab {
	if tm.activeIdx < 0 || tm.activeIdx >= len(tm.tabs) {
		return tm.tabs[0]
	}
	return tm.tabs[tm.activeIdx]
}

// ActiveIdx returns the current active tab index.
func (tm *TabManager) ActiveIdx() int {
	return tm.activeIdx
}

// Count returns the number of open tabs.
func (tm *TabManager) Count() int {
	return len(tm.tabs)
}

// Tabs returns the slice of all tabs (read-only intent).
func (tm *TabManager) Tabs() []*Tab {
	return tm.tabs
}

// ChatTab returns the always-present chat tab at index 0.
func (tm *TabManager) ChatTab() *Tab {
	return tm.tabs[0]
}

// SetActive switches to tab at the given index.
// Returns false if the index is out of range.
func (tm *TabManager) SetActive(idx int) bool {
	if idx < 0 || idx >= len(tm.tabs) {
		return false
	}
	tm.activeIdx = idx
	tm.tabs[idx].Unread = false
	return true
}

// FindByID returns the tab and its index, or nil,-1 if not found.
func (tm *TabManager) FindByID(id string) (*Tab, int) {
	for i, t := range tm.tabs {
		if t.ID == id {
			return t, i
		}
	}
	return nil, -1
}

// Add appends a new tab and returns its index.
func (tm *TabManager) Add(tab *Tab) int {
	tm.tabs = append(tm.tabs, tab)
	return len(tm.tabs) - 1
}

// Remove removes a tab by ID. The Chat tab (index 0) cannot be removed.
// If the active tab is removed, switches to the previous tab.
// Returns true if a tab was removed.
func (tm *TabManager) Remove(id string) bool {
	idx := -1
	for i, t := range tm.tabs {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx <= 0 { // can't remove chat (0) or not found (-1)
		return false
	}

	tm.tabs = append(tm.tabs[:idx], tm.tabs[idx+1:]...)

	// Fix active index
	if tm.activeIdx >= len(tm.tabs) {
		tm.activeIdx = len(tm.tabs) - 1
	} else if tm.activeIdx > idx {
		tm.activeIdx--
	} else if tm.activeIdx == idx {
		// Was viewing the removed tab — go to previous
		tm.activeIdx--
		if tm.activeIdx < 0 {
			tm.activeIdx = 0
		}
	}
	return true
}

// NextTab cycles to the next tab, wrapping around.
func (tm *TabManager) NextTab() {
	tm.activeIdx = (tm.activeIdx + 1) % len(tm.tabs)
	tm.tabs[tm.activeIdx].Unread = false
}

// PrevTab cycles to the previous tab, wrapping around.
func (tm *TabManager) PrevTab() {
	tm.activeIdx--
	if tm.activeIdx < 0 {
		tm.activeIdx = len(tm.tabs) - 1
	}
	tm.tabs[tm.activeIdx].Unread = false
}

// SetSessionName sets the bash session name for a bash tab.
func (t *Tab) SetSessionName(name string) {
	t.sessionName = name
}

// SessionName returns the bash session name.
func (t *Tab) SessionName() string {
	return t.sessionName
}

// AutoSwitch returns whether auto-switching is enabled.
func (tm *TabManager) AutoSwitch() bool {
	return tm.autoSwitch
}

// SetAutoSwitch enables or disables auto-switching.
func (tm *TabManager) SetAutoSwitch(on bool) {
	tm.autoSwitch = on
}
