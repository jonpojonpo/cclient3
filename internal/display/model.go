package display

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type displayState int

const (
	stateIdle displayState = iota
	stateThinking
	stateStreaming
	stateToolExecuting
)

// historyEntry stores a rendered panel for the scrollable history.
type historyEntry struct {
	content string
}

type Model struct {
	// Tab management — per-tab state (history, scrollOffset, streaming) lives here
	tabs *TabManager

	// TUI global state
	textInput  textinput.Model
	width      int
	height     int
	spinnerIdx int

	// Input history (up/down arrow recall)
	inputHistory []string
	historyIdx   int    // -1 = not navigating history
	savedInput   string // saved draft when navigating history

	// Markdown rendering
	mdRenderer *rendererCache

	// Config
	theme        Theme
	model        string
	inputTokens  int
	outputTokens int
	cacheCreated int
	cacheRead    int

	// Cached banner (invalidated on theme/model change)
	bannerText string

	// Channel for sending user input to the agent
	InputChan chan string
	// Channel for receiving bubbletea messages from the agent
	AgentMsgChan chan tea.Msg
	// ConfirmChan is written to when the user answers a confirmation dialog.
	ConfirmChan chan bool

	// Confirmation dialog state
	pendingConfirm *ConfirmRequestMsg

	// Context window tracking
	ctxTokensUsed int

	quitting bool
}

func NewModel(themeName, modelName string) *Model {
	ti := textinput.New()
	ti.Placeholder = "Type a message... (or /help)"
	ti.Focus()
	ti.CharLimit = 0
	ti.Width = 80

	theme, ok := Themes[themeName]
	if !ok {
		theme = Themes["cyber"]
	}

	return &Model{
		tabs:         NewTabManager(),
		textInput:    ti,
		mdRenderer:   newRendererCache(),
		theme:        theme,
		model:        modelName,
		InputChan:    make(chan string, 1),
		AgentMsgChan: make(chan tea.Msg, 100),
		ConfirmChan:  make(chan bool, 1),
		width:        80,
		height:       24,
		historyIdx:   -1,
	}
}

// TabManager returns the tab manager for external callers (e.g. commands).
func (m *Model) TabManager() *TabManager {
	return m.tabs
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.waitForAgentMsg(),
		m.tickSpinner(),
	)
}

func (m *Model) waitForAgentMsg() tea.Cmd {
	return func() tea.Msg {
		msg := <-m.AgentMsgChan
		return msg
	}
}

func (m *Model) tickSpinner() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}

// chatTab is a convenience accessor for the always-present Chat tab.
func (m *Model) chatTab() *Tab {
	return m.tabs.ChatTab()
}

// activeTab returns the currently active tab.
func (m *Model) activeTab() *Tab {
	return m.tabs.Active()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	chat := m.chatTab()

	// Agent messages all need to re-schedule waitForAgentMsg after handling.
	// We set this flag in each agent message case instead of repeating the call.
	scheduleWait := false

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Confirmation dialog intercepts all keys
		if m.pendingConfirm != nil {
			switch {
			case msg.Type == tea.KeyEsc || msg.String() == "n" || msg.String() == "N":
				m.pendingConfirm = nil
				m.ConfirmChan <- false
			case msg.String() == "y" || msg.String() == "Y" || msg.Type == tea.KeyEnter:
				m.pendingConfirm = nil
				m.ConfirmChan <- true
			}
			return m, tea.Batch(cmds...)
		}

		// Check for tab navigation keys (Alt+N, Alt+[, Alt+], Alt+w, Alt+p)
		if m.handleTabKeys(msg) {
			// Re-focus text input when switching to Chat tab
			if m.tabs.ActiveIdx() == 0 {
				m.textInput.Focus()
			} else {
				m.textInput.Blur()
			}
			return m, tea.Batch(cmds...)
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyUp:
			if chat.state == stateIdle && len(m.inputHistory) > 0 && m.tabs.ActiveIdx() == 0 {
				if m.historyIdx == -1 {
					m.savedInput = m.textInput.Value()
					m.historyIdx = len(m.inputHistory) - 1
				} else if m.historyIdx > 0 {
					m.historyIdx--
				}
				m.textInput.SetValue(m.inputHistory[m.historyIdx])
				m.textInput.CursorEnd()
			}
		case tea.KeyDown:
			if chat.state == stateIdle && m.historyIdx != -1 && m.tabs.ActiveIdx() == 0 {
				m.historyIdx++
				if m.historyIdx >= len(m.inputHistory) {
					m.historyIdx = -1
					m.textInput.SetValue(m.savedInput)
					m.savedInput = ""
				} else {
					m.textInput.SetValue(m.inputHistory[m.historyIdx])
				}
				m.textInput.CursorEnd()
			}
		case tea.KeyEnter:
			if chat.state == stateIdle && m.tabs.ActiveIdx() == 0 {
				text := strings.TrimSpace(m.textInput.Value())
				if text != "" {
					// Check for file paths / file:// URIs to attach
					text, attachments := extractFileAttachments(text)
					for _, att := range attachments {
						notice := lipgloss.NewStyle().
							Foreground(m.theme.Accent).
							Render(fmt.Sprintf("  Attached: %s (%d bytes)", att.name, len(att.content)))
						chat.history = append(chat.history, historyEntry{content: notice})
					}

					// Append to history (skip duplicate of last entry)
					if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != text {
						m.inputHistory = append(m.inputHistory, text)
					}
					m.historyIdx = -1
					m.savedInput = ""
					m.textInput.SetValue("")
					chat.history = append(chat.history, historyEntry{content: m.renderUserPanel(displayText(text, attachments))})
					m.scrollToBottom()
					m.InputChan <- text
				}
			}
		case tea.KeyPgUp:
			active := m.activeTab()
			active.scrollOffset += m.height / 2
		case tea.KeyPgDown:
			active := m.activeTab()
			active.scrollOffset -= m.height / 2
			if active.scrollOffset < 0 {
				active.scrollOffset = 0
			}
		}

	case tea.MouseMsg:
		active := m.activeTab()
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			active.scrollOffset += 3
		case tea.MouseButtonWheelDown:
			active.scrollOffset -= 3
			if active.scrollOffset < 0 {
				active.scrollOffset = 0
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = msg.Width - 4

	case SpinnerTickMsg:
		m.spinnerIdx++
		cmds = append(cmds, m.tickSpinner())

	// --- Chat tab streaming messages (from main agent) ---
	case TextDeltaMsg:
		scheduleWait = true
		chat.state = stateStreaming
		chat.currentText.WriteString(msg.Text)

	case ThinkingDeltaMsg:
		scheduleWait = true
		chat.state = stateThinking
		chat.currentThinking.WriteString(msg.Thinking)

	case ToolStartMsg:
		scheduleWait = true
		chat.state = stateToolExecuting
		chat.history = append(chat.history, historyEntry{
			content: m.renderToolPanel(msg.Name, msg.ID, "", nil, false),
		})
		m.scrollToBottom()

	case ToolResultMsg:
		scheduleWait = true
		r := msg.Result
		output := r.Result.Output
		if r.Result.Error != "" {
			output = r.Result.Error
		}
		chat.history = append(chat.history, historyEntry{
			content: m.renderToolPanel(r.Call.Name, r.Call.ID, r.Result.Lang, &output, r.Result.IsError),
		})
		m.scrollToBottom()

	case StreamDoneMsg:
		scheduleWait = true
		// Flush current text to history
		if chat.currentThinking.Len() > 0 {
			chat.history = append(chat.history, historyEntry{
				content: m.renderThinkingPanel(chat.currentThinking.String()),
			})
			chat.currentThinking.Reset()
		}
		if chat.currentText.Len() > 0 {
			chat.history = append(chat.history, historyEntry{
				content: m.renderAssistantPanel(chat.currentText.String()),
			})
			chat.currentText.Reset()
		}
		chat.state = stateIdle
		m.scrollToBottom()

	case ErrorMsg:
		scheduleWait = true
		chat.history = append(chat.history, historyEntry{
			content: m.renderErrorPanel(msg.Err.Error()),
		})
		chat.state = stateIdle
		m.scrollToBottom()

	// --- Tab-routed streaming messages (from sub-agents) ---
	case TabTextDeltaMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.state = stateStreaming
			tab.currentText.WriteString(msg.Text)
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}

	case TabThinkingDeltaMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.state = stateThinking
			tab.currentThinking.WriteString(msg.Thinking)
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}

	case TabToolStartMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.state = stateToolExecuting
			tab.history = append(tab.history, historyEntry{
				content: m.renderToolPanel(msg.Name, msg.ID, "", nil, false),
			})
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}

	case TabToolResultMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			r := msg.Result
			output := r.Output
			if r.Error != "" {
				output = r.Error
			}
			tab.history = append(tab.history, historyEntry{
				content: m.renderToolPanel(msg.Name, msg.ID, r.Lang, &output, r.IsError),
			})
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}

	case TabStreamDoneMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			// Flush current text to tab history
			if tab.currentThinking.Len() > 0 {
				tab.history = append(tab.history, historyEntry{
					content: m.renderThinkingPanel(tab.currentThinking.String()),
				})
				tab.currentThinking.Reset()
			}
			if tab.currentText.Len() > 0 {
				tab.history = append(tab.history, historyEntry{
					content: m.renderAssistantPanel(tab.currentText.String()),
				})
				tab.currentText.Reset()
			}
			tab.state = stateIdle
			// Cap agent tab history
			if len(tab.history) > 200 {
				tab.history = tab.history[len(tab.history)-200:]
			}
		}

	case TabErrorMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.history = append(tab.history, historyEntry{
				content: m.renderErrorPanel(msg.Err.Error()),
			})
			tab.state = stateIdle
			tab.Status = TabError
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}

	// --- Global messages ---
	case TokenUpdateMsg:
		scheduleWait = true
		m.inputTokens = msg.InputTokens
		m.outputTokens = msg.OutputTokens
		m.cacheCreated = msg.CacheCreationInputTokens
		m.cacheRead = msg.CacheReadInputTokens
		m.ctxTokensUsed = msg.InputTokens

	case SetThemeMsg:
		scheduleWait = true
		if t, ok := Themes[msg.Name]; ok {
			m.theme = t
			m.bannerText = "" // invalidate banner cache
		}

	case SetModelMsg:
		scheduleWait = true
		m.model = msg.Name
		m.bannerText = "" // invalidate banner cache

	case StatusMsg:
		scheduleWait = true
		chat.history = append(chat.history, historyEntry{
			content: lipgloss.NewStyle().Foreground(m.theme.Dim).Render(msg.Text),
		})
		m.scrollToBottom()

	case ModelsListMsg:
		scheduleWait = true
		var b strings.Builder
		b.WriteString("Available models:\n")
		for _, name := range msg.Models {
			b.WriteString(fmt.Sprintf("  - %s\n", name))
		}
		chat.history = append(chat.history, historyEntry{
			content: lipgloss.NewStyle().Foreground(m.theme.Secondary).Render(b.String()),
		})
		m.scrollToBottom()

	case ClearMsg:
		scheduleWait = true
		chat.history = nil
		chat.scrollOffset = 0

	case SubAgentStartMsg:
		// Create a new tab for this sub-agent
		agentTab := &Tab{
			ID:     msg.ID,
			Label:  msg.ID,
			Kind:   TabAgent,
			Status: TabRunning,
		}
		idx := m.tabs.Add(agentTab)

		// Also add a compact notification to Chat tab
		chat.history = append(chat.history, historyEntry{
			content: m.renderSubAgentPanel(msg.ID, msg.Task, msg.Model),
		})
		m.scrollToBottom()

		// Auto-switch to the new agent tab
		if m.tabs.AutoSwitch() && !m.activeTab().UserPinned {
			m.tabs.SetActive(idx)
		}

		cmds = append(cmds, m.waitForAgentMsg())

	case SubAgentStepMsg:
		// Add step to agent's tab history (not Chat)
		if tab, _ := m.tabs.FindByID(msg.ID); tab != nil {
			step := lipgloss.NewStyle().Foreground(m.theme.Dim).
				Render(fmt.Sprintf("  ↳ %s", msg.ToolName))
			tab.history = append(tab.history, historyEntry{content: step})
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}
		// Also add compact step to Chat tab
		step := lipgloss.NewStyle().Foreground(m.theme.Dim).
			Render(fmt.Sprintf("    ↳ [%s] %s", msg.ID, msg.ToolName))
		chat.history = append(chat.history, historyEntry{content: step})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case SubAgentDoneMsg:
		// Update tab status
		if tab, _ := m.tabs.FindByID(msg.ID); tab != nil {
			if msg.IsError {
				tab.Status = TabError
			} else {
				tab.Status = TabDone
			}
			// Auto-switch back to Chat if viewing the finished agent tab
			if m.tabs.Active() == tab && !tab.UserPinned && m.tabs.AutoSwitch() {
				m.tabs.SetActive(0)
			}
		}

		// Add completion notice to Chat tab
		label := "done"
		color := m.theme.Secondary
		if msg.IsError {
			label = "failed"
			color = m.theme.Error
		}
		done := lipgloss.NewStyle().Foreground(color).
			Render(fmt.Sprintf("    ✓ [%s] %s", msg.ID, label))
		chat.history = append(chat.history, historyEntry{content: done})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case BashOutputMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.outputLines = append(tab.outputLines, msg.Lines...)
			// Cap bash tab output
			if len(tab.outputLines) > 500 {
				tab.outputLines = tab.outputLines[len(tab.outputLines)-500:]
			}
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}

	case ContextTrimMsg:
		notice := lipgloss.NewStyle().Foreground(m.theme.Dim).Italic(true).
			Render(fmt.Sprintf("  [Context trimmed: %d messages dropped to fit window]", msg.Dropped))
		chat.history = append(chat.history, historyEntry{content: notice})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case ConfirmRequestMsg:
		m.pendingConfirm = &msg
		cmds = append(cmds, m.waitForAgentMsg())

	case QuitMsg:
		m.quitting = true
		return m, tea.Quit
	}

	if scheduleWait {
		cmds = append(cmds, m.waitForAgentMsg())
	}

	// Update text input only when on Chat tab and idle
	if chat.state == stateIdle && m.tabs.ActiveIdx() == 0 {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	if m.quitting {
		return lipgloss.NewStyle().Foreground(m.theme.Primary).Render("Goodbye!\n")
	}

	// === Fixed header: tab bar (never scrolls) ===
	tabBar := m.renderTabBar()
	tabBarHeight := strings.Count(tabBar, "\n") + 1

	// === Scrollable content ===
	var sections []string
	active := m.activeTab()

	switch active.Kind {
	case TabChat:
		// Banner — only render once and cache it
		if m.bannerText == "" {
			m.bannerText = Banner(m.theme, m.model)
		}
		sections = append(sections, m.bannerText)

		// History
		for _, entry := range active.history {
			sections = append(sections, entry.content)
		}

		// Current streaming content
		if active.state == stateThinking && active.currentThinking.Len() > 0 {
			sections = append(sections, m.renderThinkingPanel(active.currentThinking.String()))
		}
		if active.state == stateStreaming && active.currentText.Len() > 0 {
			sections = append(sections, m.renderAssistantPanel(active.currentText.String()))
		}

		// Spinner for thinking/executing states
		if active.state == stateThinking || active.state == stateToolExecuting {
			label := "Thinking"
			if active.state == stateToolExecuting {
				label = "Executing tools"
			}
			sections = append(sections, fmt.Sprintf(" %s %s...", m.renderSpinner(), label))
		}

	case TabAgent:
		// Agent tab: show history + streaming
		for _, entry := range active.history {
			sections = append(sections, entry.content)
		}
		if active.state == stateThinking && active.currentThinking.Len() > 0 {
			sections = append(sections, m.renderThinkingPanel(active.currentThinking.String()))
		}
		if active.state == stateStreaming && active.currentText.Len() > 0 {
			sections = append(sections, m.renderAssistantPanel(active.currentText.String()))
		}
		if active.state == stateThinking || active.state == stateToolExecuting {
			label := "Thinking"
			if active.state == stateToolExecuting {
				label = "Executing tools"
			}
			sections = append(sections, fmt.Sprintf(" %s %s...", m.renderSpinner(), label))
		}

	case TabBash:
		// Bash tab: show rolling output lines
		if len(active.outputLines) == 0 {
			sections = append(sections, lipgloss.NewStyle().Foreground(m.theme.Dim).
				Render("  (no output yet)"))
		} else {
			content := strings.Join(active.outputLines, "\n")
			sections = append(sections, content)
		}
	}

	body := strings.Join(sections, "\n")

	// === Fixed footer: input + status bar ===
	statusBarHeight := 1
	inputHeight := 2

	// Calculate available height for scrollable body
	available := m.height - tabBarHeight - statusBarHeight - inputHeight - 1
	if available < 1 {
		available = 1
	}
	bodyLines := strings.Split(body, "\n")

	// Compute effective scroll offset (read-only, no mutation in View)
	scrollOffset := active.scrollOffset
	maxOffset := len(bodyLines) - available
	if maxOffset < 0 {
		maxOffset = 0
	}
	if scrollOffset > maxOffset {
		scrollOffset = maxOffset
	}

	if len(bodyLines) > available && available > 0 {
		end := len(bodyLines) - scrollOffset
		start := end - available
		if start < 0 {
			start = 0
		}
		bodyLines = bodyLines[start:end]
	}
	body = strings.Join(bodyLines, "\n")

	// Input area
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Primary).
		Width(m.width - 4)

	prompt := ""
	if m.tabs.ActiveIdx() == 0 {
		// Chat tab: show text input
		if m.chatTab().state == stateIdle {
			prompt = inputStyle.Render(m.textInput.View())
		} else {
			prompt = inputStyle.Render(
				lipgloss.NewStyle().Foreground(m.theme.Dim).Render("  (waiting for response...)"),
			)
		}
	} else {
		// Non-chat tabs: show hint
		prompt = inputStyle.Render(
			lipgloss.NewStyle().Foreground(m.theme.Dim).Render("  Press Alt+1 to return to Chat"),
		)
	}

	statusBar := m.renderStatusBar()

	// Confirmation dialog overlay — renders on top of everything else
	if m.pendingConfirm != nil {
		return tabBar + "\n" + body + "\n" + m.renderConfirmDialog() + "\n" + statusBar
	}

	return tabBar + "\n" + body + "\n" + prompt + "\n" + statusBar
}

// handleTabKeys processes tab-navigation key bindings.
// Returns true if the key was consumed by tab navigation.
func (m *Model) handleTabKeys(msg tea.KeyMsg) bool {
	key := msg.String()

	// Alt+number: bubbletea v1 sends Alt+Rune as msg.Alt=true with runes.
	// msg.String() returns "alt+1", "alt+2", etc.
	// Also handle via msg.Alt + rune check for terminal compatibility.
	if msg.Alt && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		r := msg.Runes[0]
		if r >= '1' && r <= '9' {
			idx := int(r - '1')
			if idx >= 0 && idx < m.tabs.Count() {
				m.tabs.SetActive(idx)
				if idx == 0 {
					m.tabs.ChatTab().UserPinned = false
				}
			}
			return true
		}
		switch r {
		case '[':
			m.tabs.PrevTab()
			return true
		case ']':
			m.tabs.NextTab()
			return true
		case 'w':
			if m.tabs.ActiveIdx() != 0 {
				m.tabs.Remove(m.activeTab().ID)
			}
			return true
		case 'p':
			active := m.activeTab()
			active.UserPinned = !active.UserPinned
			return true
		}
	}

	// Fallback: match string representation for terminals that encode differently
	switch key {
	case "alt+[":
		m.tabs.PrevTab()
		return true
	case "alt+]":
		m.tabs.NextTab()
		return true
	case "alt+w":
		if m.tabs.ActiveIdx() != 0 {
			m.tabs.Remove(m.activeTab().ID)
		}
		return true
	case "alt+p":
		active := m.activeTab()
		active.UserPinned = !active.UserPinned
		return true
	}

	// Also check string-based alt+N pattern
	if len(key) == 5 && key[:4] == "alt+" && key[4] >= '1' && key[4] <= '9' {
		idx := int(key[4] - '1')
		if idx >= 0 && idx < m.tabs.Count() {
			m.tabs.SetActive(idx)
			if idx == 0 {
				m.tabs.ChatTab().UserPinned = false
			}
		}
		return true
	}

	return false
}

func (m *Model) scrollToBottom() {
	m.activeTab().scrollOffset = 0
}

func (m *Model) visibleHistory() []historyEntry {
	return m.activeTab().history
}

// renderConfirmDialog renders a modal confirmation prompt.
func (m *Model) renderConfirmDialog() string {
	if m.pendingConfirm == nil {
		return ""
	}
	th := m.theme
	c := m.pendingConfirm

	header := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("  Confirm Action")
	promptLine := lipgloss.NewStyle().Foreground(th.Text).Render("  " + c.Prompt)
	cmdLine := lipgloss.NewStyle().Foreground(th.Secondary).Render("  Command: " + c.Command)
	hint := lipgloss.NewStyle().Foreground(th.Dim).Render("  [y] Allow  [n/Esc] Deny")

	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(th.Accent).
		Padding(0, 1).
		Width(m.width - 4).
		Render(promptLine + "\n" + cmdLine + "\n\n" + hint)

	return header + "\n" + box
}

// fileAttachment holds a file path and its read content.
type fileAttachment struct {
	name    string
	path    string
	content string
}

// extractFileAttachments scans text for file:// URIs and bare absolute/relative paths.
// It returns the original text with path tokens replaced by placeholders, plus
// a slice of successfully-read attachments. The actual text sent to the agent
// includes inlined file content prepended to the message.
func extractFileAttachments(text string) (string, []fileAttachment) {
	var attachments []fileAttachment
	var sb strings.Builder

	// Scan for file:// URIs first
	remaining := text
	for {
		idx := strings.Index(remaining, "file://")
		if idx < 0 {
			sb.WriteString(remaining)
			break
		}
		sb.WriteString(remaining[:idx])
		rest := remaining[idx+7:] // after "file://"
		// path ends at first whitespace
		end := strings.IndexAny(rest, " \t\n\r")
		var rawPath string
		if end < 0 {
			rawPath = rest
			remaining = ""
		} else {
			rawPath = rest[:end]
			remaining = rest[end:]
		}
		if content, err := os.ReadFile(rawPath); err == nil {
			name := filepath.Base(rawPath)
			attachments = append(attachments, fileAttachment{name: name, path: rawPath, content: string(content)})
			sb.WriteString("[attached: " + name + "]")
		} else {
			sb.WriteString("file://" + rawPath)
		}
	}
	text = sb.String()

	// Build final message: prepend file contents
	if len(attachments) == 0 {
		return text, nil
	}
	var out strings.Builder
	for _, att := range attachments {
		out.WriteString(fmt.Sprintf("Contents of %s:\n```\n%s\n```\n\n", att.name, att.content))
	}
	out.WriteString(text)
	return out.String(), attachments
}

// displayText returns a short display string for the user panel when files were attached.
func displayText(fullText string, attachments []fileAttachment) string {
	if len(attachments) == 0 {
		return fullText
	}
	names := make([]string, len(attachments))
	for i, a := range attachments {
		names[i] = a.name
	}
	// Show a shortened version: just the user's words + attachment names
	parts := strings.SplitN(fullText, "\n\n", 2)
	userWords := ""
	if len(parts) > 1 {
		userWords = strings.TrimSpace(parts[len(parts)-1])
	} else {
		userWords = strings.TrimSpace(fullText)
	}
	if userWords == "" {
		return "Attached: " + strings.Join(names, ", ")
	}
	return userWords + "\n[Attached: " + strings.Join(names, ", ") + "]"
}
