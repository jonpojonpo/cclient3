package display

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
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
	// Tab management
	tabs *TabManager

	// TUI global state
	textarea     textarea.Model
	textareaRows int // current visible rows (1–3)
	width        int
	height       int
	spinnerIdx   int

	// Input history (up/down arrow recall)
	inputHistory []string
	historyIdx   int
	savedInput   string

	// Markdown rendering
	mdRenderer *rendererCache

	// Config
	theme        Theme
	model        string
	inputTokens  int
	outputTokens int
	cacheCreated int
	cacheRead    int

	// Cumulative session cost (USD)
	sessionCostUSD float64

	// Cached banner
	bannerText string

	// Channels
	InputChan    chan string
	AgentMsgChan chan tea.Msg
	ConfirmChan  chan bool

	// Confirmation dialog state
	pendingConfirm *ConfirmRequestMsg

	// Context window tracking
	ctxTokensUsed int

	quitting bool
}

func NewModel(themeName, modelName string) *Model {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (Ctrl+J for newline, /help)"
	ta.Focus()
	ta.CharLimit = 0
	ta.SetHeight(1)
	ta.SetWidth(78)
	ta.ShowLineNumbers = false
	// Remap newline insertion: Enter submits; Ctrl+J or Alt+Enter inserts newline
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("ctrl+j", "alt+enter"),
		key.WithHelp("ctrl+j", "new line"),
	)

	theme, ok := Themes[themeName]
	if !ok {
		theme = Themes["cyber"]
	}

	return &Model{
		tabs:         NewTabManager(),
		textarea:     ta,
		textareaRows: 1,
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

func (m *Model) TabManager() *TabManager { return m.tabs }

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.waitForAgentMsg(),
		m.tickSpinner(),
	)
}

func (m *Model) waitForAgentMsg() tea.Cmd {
	return func() tea.Msg { return <-m.AgentMsgChan }
}

func (m *Model) tickSpinner() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg { return SpinnerTickMsg{} })
}

func (m *Model) chatTab() *Tab   { return m.tabs.ChatTab() }
func (m *Model) activeTab() *Tab { return m.tabs.Active() }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	chat := m.chatTab()
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

		// Tab navigation (Alt+N, Alt+[/], Alt+←/→, Alt+w, Alt+p)
		if m.handleTabKeys(msg) {
			active := m.activeTab()
			if m.tabs.ActiveIdx() == 0 || active.Kind == TabEnsemble {
				m.textarea.Focus()
			} else {
				m.textarea.Blur()
			}
			return m, tea.Batch(cmds...)
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			active := m.activeTab()
			// Ensemble tab: send to ensemble input channel
			if !msg.Alt && active.Kind == TabEnsemble && active.EnsembleInputChan != nil {
				text := strings.TrimSpace(m.textarea.Value())
				if text != "" {
					m.textarea.Reset()
					m.textareaRows = 1
					m.textarea.SetHeight(1)
					active.history = append(active.history, historyEntry{content: m.renderEnsembleUserPanel(text)})
					m.scrollToBottom()
					active.EnsembleInputChan <- text
				}
			} else if !msg.Alt && chat.state == stateIdle && m.tabs.ActiveIdx() == 0 {
				// Plain Enter (not Alt) on the chat tab = submit
				text := strings.TrimSpace(m.textarea.Value())
				if text != "" {
					text, attachments := extractFileAttachments(text)
					for _, att := range attachments {
						notice := lipgloss.NewStyle().Foreground(m.theme.Accent).
							Render(fmt.Sprintf("  Attached: %s (%d bytes)", att.name, len(att.content)))
						chat.history = append(chat.history, historyEntry{content: notice})
					}

					// Collapsed display for pasted multi-line content
					lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
					dispStr := displayText(text, attachments)
					if len(attachments) == 0 && len(lines) > 4 {
						dispStr = fmt.Sprintf("[pasted %d lines]\n> %s\n> %s\n...",
							len(lines), lines[0], lines[1])
					}

					if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != text {
						m.inputHistory = append(m.inputHistory, text)
					}
					m.historyIdx = -1
					m.savedInput = ""
					m.textarea.Reset()
					m.textareaRows = 1
					m.textarea.SetHeight(1)
					chat.history = append(chat.history, historyEntry{content: m.renderUserPanel(dispStr)})
					m.scrollToBottom()
					m.InputChan <- text
				}
			}
			// Alt+Enter: textarea keymap handles this as newline insertion

		case tea.KeyUp:
			if chat.state == stateIdle && m.tabs.ActiveIdx() == 0 {
				// History navigation when textarea has no embedded newlines
				if !strings.Contains(m.textarea.Value(), "\n") && len(m.inputHistory) > 0 {
					if m.historyIdx == -1 {
						m.savedInput = m.textarea.Value()
						m.historyIdx = len(m.inputHistory) - 1
					} else if m.historyIdx > 0 {
						m.historyIdx--
					}
					m.textarea.SetValue(m.inputHistory[m.historyIdx])
					m.adjustTextareaHeight()
				} else {
					var cmd tea.Cmd
					m.textarea, cmd = m.textarea.Update(msg)
					cmds = append(cmds, cmd)
				}
			}

		case tea.KeyDown:
			if chat.state == stateIdle && m.tabs.ActiveIdx() == 0 {
				if m.historyIdx != -1 && !strings.Contains(m.textarea.Value(), "\n") {
					m.historyIdx++
					if m.historyIdx >= len(m.inputHistory) {
						m.historyIdx = -1
						m.textarea.SetValue(m.savedInput)
						m.savedInput = ""
					} else {
						m.textarea.SetValue(m.inputHistory[m.historyIdx])
					}
					m.adjustTextareaHeight()
				} else if strings.Contains(m.textarea.Value(), "\n") {
					var cmd tea.Cmd
					m.textarea, cmd = m.textarea.Update(msg)
					cmds = append(cmds, cmd)
				}
			}

		case tea.KeyPgUp:
			m.activeTab().scrollOffset += m.height / 2
		case tea.KeyPgDown:
			active := m.activeTab()
			active.scrollOffset -= m.height / 2
			if active.scrollOffset < 0 {
				active.scrollOffset = 0
			}

		default:
			// Pass all other keys to textarea when on chat tab (idle) or ensemble tab
			active := m.activeTab()
			canType := (chat.state == stateIdle && m.tabs.ActiveIdx() == 0) ||
				active.Kind == TabEnsemble
			if canType {
				prevLen := len(m.textarea.Value())
				var cmd tea.Cmd
				m.textarea, cmd = m.textarea.Update(msg)
				cmds = append(cmds, cmd)
				if len(m.textarea.Value()) != prevLen {
					m.adjustTextareaHeight()
				}
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
		m.textarea.SetWidth(msg.Width - 6)

	case SpinnerTickMsg:
		m.spinnerIdx++
		cmds = append(cmds, m.tickSpinner())

	// --- Chat tab streaming ---
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
		if chat.currentThinking.Len() > 0 {
			chat.history = append(chat.history, historyEntry{content: m.renderThinkingPanel(chat.currentThinking.String())})
			chat.currentThinking.Reset()
		}
		if chat.currentText.Len() > 0 {
			chat.history = append(chat.history, historyEntry{content: m.renderAssistantPanel(chat.currentText.String())})
			chat.currentText.Reset()
		}
		chat.state = stateIdle
		m.scrollToBottom()

	case ErrorMsg:
		scheduleWait = true
		chat.history = append(chat.history, historyEntry{content: m.renderErrorPanel(msg.Err.Error())})
		chat.state = stateIdle
		m.scrollToBottom()

	// --- Tab-routed streaming (sub-agents) ---
	case TabTextDeltaMsg:
		scheduleWait = true
		if tab, idx := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.state = stateStreaming
			tab.currentText.WriteString(msg.Text)
			if m.tabs.Active() != tab {
				tab.Unread = true
				// Auto-switch from a finished agent tab to this active one.
				cur := m.tabs.Active()
				if m.tabs.AutoSwitch() && !cur.UserPinned &&
					cur.Kind == TabAgent && cur.Status != TabRunning {
					m.tabs.SetActive(idx)
				}
			}
		}

	case TabThinkingDeltaMsg:
		scheduleWait = true
		if tab, idx := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.state = stateThinking
			tab.currentThinking.WriteString(msg.Thinking)
			if m.tabs.Active() != tab {
				tab.Unread = true
				// Auto-switch from a finished agent tab to this active one.
				cur := m.tabs.Active()
				if m.tabs.AutoSwitch() && !cur.UserPinned &&
					cur.Kind == TabAgent && cur.Status != TabRunning {
					m.tabs.SetActive(idx)
				}
			}
		}

	case TabToolStartMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.state = stateToolExecuting
			tab.history = append(tab.history, historyEntry{content: m.renderToolPanel(msg.Name, msg.ID, "", nil, false)})
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
			tab.history = append(tab.history, historyEntry{content: m.renderToolPanel(msg.Name, msg.ID, r.Lang, &output, r.IsError)})
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}

	case TabStreamDoneMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			if tab.currentThinking.Len() > 0 {
				tab.history = append(tab.history, historyEntry{content: m.renderThinkingPanel(tab.currentThinking.String())})
				tab.currentThinking.Reset()
			}
			if tab.currentText.Len() > 0 {
				tab.history = append(tab.history, historyEntry{content: m.renderAssistantPanel(tab.currentText.String())})
				tab.currentText.Reset()
			}
			tab.state = stateIdle
			if len(tab.history) > 200 {
				tab.history = tab.history[len(tab.history)-200:]
			}
		}

	case TabErrorMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.history = append(tab.history, historyEntry{content: m.renderErrorPanel(msg.Err.Error())})
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
		costModel := m.model
		if msg.Model != "" {
			costModel = msg.Model
		}
		m.sessionCostUSD += ComputeCost(costModel,
			msg.InputTokens, msg.OutputTokens,
			msg.CacheReadInputTokens, msg.CacheCreationInputTokens)

	case SetThemeMsg:
		scheduleWait = true
		if t, ok := Themes[msg.Name]; ok {
			m.theme = t
			m.bannerText = ""
		}

	case SetModelMsg:
		scheduleWait = true
		m.model = msg.Name
		m.bannerText = ""

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
		m.sessionCostUSD = 0

	case SubAgentStartMsg:
		agentTab := &Tab{ID: msg.ID, Label: msg.ID, Kind: TabAgent, Status: TabRunning}
		idx := m.tabs.Add(agentTab)
		chat.history = append(chat.history, historyEntry{
			content: m.renderSubAgentPanel(msg.ID, msg.Task, msg.Model, msg.Provider),
		})
		m.scrollToBottom()
		if m.tabs.AutoSwitch() && !m.activeTab().UserPinned {
			m.tabs.SetActive(idx)
		}
		cmds = append(cmds, m.waitForAgentMsg())

	case SubAgentStepMsg:
		if tab, _ := m.tabs.FindByID(msg.ID); tab != nil {
			tab.history = append(tab.history, historyEntry{
				content: lipgloss.NewStyle().Foreground(m.theme.Dim).Render(fmt.Sprintf("  ↳ %s", msg.ToolName)),
			})
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}
		chat.history = append(chat.history, historyEntry{
			content: lipgloss.NewStyle().Foreground(m.theme.Dim).Render(fmt.Sprintf("    ↳ [%s] %s", msg.ID, msg.ToolName)),
		})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case SubAgentDoneMsg:
		if tab, _ := m.tabs.FindByID(msg.ID); tab != nil {
			if msg.IsError {
				tab.Status = TabError
			} else {
				tab.Status = TabDone
			}
			if m.tabs.Active() == tab && !tab.UserPinned && m.tabs.AutoSwitch() {
				m.tabs.SetActive(0)
			}
		}
		label, color := "done", m.theme.Secondary
		if msg.IsError {
			label, color = "failed", m.theme.Error
		}
		chat.history = append(chat.history, historyEntry{
			content: lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("    ✓ [%s] %s", msg.ID, label)),
		})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case BashOutputMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.outputLines = append(tab.outputLines, msg.Lines...)
			if len(tab.outputLines) > 500 {
				tab.outputLines = tab.outputLines[len(tab.outputLines)-500:]
			}
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}

	case ContextTrimMsg:
		chat.history = append(chat.history, historyEntry{
			content: lipgloss.NewStyle().Foreground(m.theme.Dim).Italic(true).
				Render(fmt.Sprintf("  [Context trimmed: %d messages dropped]", msg.Dropped)),
		})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case ConfirmRequestMsg:
		m.pendingConfirm = &msg
		cmds = append(cmds, m.waitForAgentMsg())

	// --- Ensemble tab messages ---

	case EnsembleStartMsg:
		scheduleWait = true
		ensTab := &Tab{
			ID:                "ensemble-" + msg.TabID,
			Label:             "Ensemble",
			Kind:              TabEnsemble,
			Status:            TabRunning,
			EnsembleInputChan: msg.UserChan,
			agentCount:        len(msg.Agents),
		}
		ensTab.history = append(ensTab.history, historyEntry{
			content: m.renderEnsembleHeader(msg.Agents),
		})
		ensTab.history = append(ensTab.history, historyEntry{
			content: m.renderEnsembleUserPanel(msg.Prompt),
		})
		idx := m.tabs.Add(ensTab)
		m.tabs.SetActive(idx)
		m.textarea.Focus()
		m.scrollToBottom()

	case EnsembleUserMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.history = append(tab.history, historyEntry{
				content: m.renderEnsembleUserPanel(msg.Text),
			})
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}

	case EnsembleSpeakerMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.state = stateStreaming
			tab.currentSpeaker = msg.Speaker
			tab.currentColor = msg.Color
			tab.currentText.Reset()
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}

	case EnsembleTextDeltaMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.state = stateStreaming
			tab.currentSpeaker = msg.Speaker
			tab.currentColor = msg.Color
			tab.currentText.WriteString(msg.Text)
			if m.tabs.Active() != tab {
				tab.Unread = true
			}
		}

	case EnsembleTurnDoneMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			if tab.currentText.Len() > 0 {
				tab.history = append(tab.history, historyEntry{
					content: m.renderEnsembleSpeakerPanel(tab.currentSpeaker, tab.currentText.String(), tab.currentColor),
				})
				tab.currentText.Reset()
			}
			tab.currentSpeaker = ""
			tab.currentColor = ""
			tab.state = stateIdle
			if len(tab.history) > 300 {
				tab.history = tab.history[len(tab.history)-300:]
			}
		}

	case EnsembleRoundDoneMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.state = stateIdle
			sep := lipgloss.NewStyle().Foreground(m.theme.Dim).Render(
				"  ─── Round complete. Type a message to continue, or /done to end. ───")
			tab.history = append(tab.history, historyEntry{content: sep})
		}

	case EnsembleDoneMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.Status = TabDone
			tab.state = stateIdle
			if msg.Summary != "" {
				tab.history = append(tab.history, historyEntry{
					content: m.renderPanel(m.theme.Accent, "  Summary", msg.Summary),
				})
			}
			done := lipgloss.NewStyle().Foreground(m.theme.Secondary).Render("  Ensemble session ended.")
			tab.history = append(tab.history, historyEntry{content: done})
		}

	case EnsembleErrorMsg:
		scheduleWait = true
		if tab, _ := m.tabs.FindByID(msg.TabID); tab != nil {
			tab.history = append(tab.history, historyEntry{
				content: m.renderErrorPanel(msg.Err.Error()),
			})
			tab.Status = TabError
			tab.state = stateIdle
		}

	case QuitMsg:
		m.quitting = true
		return m, tea.Quit
	}

	if scheduleWait {
		cmds = append(cmds, m.waitForAgentMsg())
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	if m.quitting {
		return lipgloss.NewStyle().Foreground(m.theme.Primary).Render("Goodbye!\n")
	}

	tabBar := m.renderTabBar()
	tabBarHeight := strings.Count(tabBar, "\n") + 1

	var sections []string
	active := m.activeTab()

	switch active.Kind {
	case TabChat:
		if m.bannerText == "" {
			m.bannerText = Banner(m.theme, m.model)
		}
		sections = append(sections, m.bannerText)
		for _, e := range active.history {
			sections = append(sections, e.content)
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

	case TabAgent:
		for _, e := range active.history {
			sections = append(sections, e.content)
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
		if len(active.outputLines) == 0 {
			sections = append(sections, lipgloss.NewStyle().Foreground(m.theme.Dim).Render("  (no output yet)"))
		} else {
			sections = append(sections, strings.Join(active.outputLines, "\n"))
		}

	case TabEnsemble:
		for _, e := range active.history {
			sections = append(sections, e.content)
		}
		if active.state == stateStreaming && active.currentText.Len() > 0 {
			sections = append(sections, m.renderEnsembleSpeakerPanel(
				active.currentSpeaker, active.currentText.String(), active.currentColor))
		}
		if active.state == stateStreaming || active.state == stateThinking {
			sections = append(sections, fmt.Sprintf(" %s %s is responding...",
				m.renderSpinner(), active.currentSpeaker))
		}
	}

	body := strings.Join(sections, "\n")

	statusBarHeight := 1
	inputHeight := m.textareaRows + 2 // content rows + top + bottom border

	available := m.height - tabBarHeight - statusBarHeight - inputHeight - 1
	if available < 1 {
		available = 1
	}
	bodyLines := strings.Split(body, "\n")

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

	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Primary).
		Width(m.width - 4)

	var prompt string
	if m.tabs.ActiveIdx() == 0 {
		if m.chatTab().state == stateIdle {
			prompt = inputStyle.Render(m.textarea.View())
		} else {
			prompt = inputStyle.Render(
				lipgloss.NewStyle().Foreground(m.theme.Dim).Render("  (waiting for response...)"),
			)
		}
	} else if active.Kind == TabEnsemble {
		prompt = inputStyle.Render(m.textarea.View())
	} else {
		prompt = inputStyle.Render(
			lipgloss.NewStyle().Foreground(m.theme.Dim).Render("  Alt+1: Chat  Alt+←/→: switch tabs"),
		)
	}

	statusBar := m.renderStatusBar()

	if m.pendingConfirm != nil {
		return tabBar + "\n" + body + "\n" + m.renderConfirmDialog() + "\n" + statusBar
	}

	return tabBar + "\n" + body + "\n" + prompt + "\n" + statusBar
}

// adjustTextareaHeight grows the visible textarea up to 3 rows based on content.
func (m *Model) adjustTextareaHeight() {
	lines := strings.Count(m.textarea.Value(), "\n") + 1
	if lines < 1 {
		lines = 1
	}
	if lines > 3 {
		lines = 3
	}
	if lines != m.textareaRows {
		m.textareaRows = lines
		m.textarea.SetHeight(lines)
	}
}

// handleTabKeys handles Alt-based tab navigation shortcuts.
func (m *Model) handleTabKeys(msg tea.KeyMsg) bool {
	k := msg.String()

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
			m.tabs.PruneDone()
			return true
		}
	}

	// Alt+Left / Alt+Right for tab cycling
	if msg.Alt && msg.Type == tea.KeyLeft {
		m.tabs.PrevTab()
		return true
	}
	if msg.Alt && msg.Type == tea.KeyRight {
		m.tabs.NextTab()
		return true
	}

	switch k {
	case "alt+[", "alt+left":
		m.tabs.PrevTab()
		return true
	case "alt+]", "alt+right":
		m.tabs.NextTab()
		return true
	case "alt+w":
		if m.tabs.ActiveIdx() != 0 {
			m.tabs.Remove(m.activeTab().ID)
		}
		return true
	case "alt+p":
		m.tabs.PruneDone()
		return true
	}

	if len(k) == 5 && k[:4] == "alt+" && k[4] >= '1' && k[4] <= '9' {
		idx := int(k[4] - '1')
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

func (m *Model) scrollToBottom()             { m.activeTab().scrollOffset = 0 }
func (m *Model) visibleHistory() []historyEntry { return m.activeTab().history }

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

type fileAttachment struct {
	name    string
	path    string
	content string
}

func extractFileAttachments(text string) (string, []fileAttachment) {
	var attachments []fileAttachment
	var sb strings.Builder

	remaining := text
	for {
		idx := strings.Index(remaining, "file://")
		if idx < 0 {
			sb.WriteString(remaining)
			break
		}
		sb.WriteString(remaining[:idx])
		rest := remaining[idx+7:]
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

func displayText(fullText string, attachments []fileAttachment) string {
	if len(attachments) == 0 {
		return fullText
	}
	names := make([]string, len(attachments))
	for i, a := range attachments {
		names[i] = a.name
	}
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
