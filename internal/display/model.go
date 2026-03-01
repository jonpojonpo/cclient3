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
	// TUI state
	state      displayState
	textInput  textinput.Model
	history       []historyEntry
	scrollOffset  int // lines scrolled up from bottom (0 = pinned to bottom)
	width      int
	height     int
	spinnerIdx int

	// Input history (up/down arrow recall)
	inputHistory []string
	historyIdx   int    // -1 = not navigating history
	savedInput   string // saved draft when navigating history

	// Current streaming state
	currentText     strings.Builder
	currentThinking strings.Builder

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
		state:        stateIdle,
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

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

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

		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyUp:
			if m.state == stateIdle && len(m.inputHistory) > 0 {
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
			if m.state == stateIdle && m.historyIdx != -1 {
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
			if m.state == stateIdle {
				text := strings.TrimSpace(m.textInput.Value())
				if text != "" {
					// Check for file paths / file:// URIs to attach
					text, attachments := extractFileAttachments(text)
					for _, att := range attachments {
						notice := lipgloss.NewStyle().
							Foreground(m.theme.Accent).
							Render(fmt.Sprintf("  Attached: %s (%d bytes)", att.name, len(att.content)))
						m.history = append(m.history, historyEntry{content: notice})
					}

					// Append to history (skip duplicate of last entry)
					if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != text {
						m.inputHistory = append(m.inputHistory, text)
					}
					m.historyIdx = -1
					m.savedInput = ""
					m.textInput.SetValue("")
					m.history = append(m.history, historyEntry{content: m.renderUserPanel(displayText(text, attachments))})
					m.scrollToBottom()
					m.InputChan <- text
				}
			}
		case tea.KeyPgUp:
			m.scrollOffset += m.height / 2
		case tea.KeyPgDown:
			m.scrollOffset -= m.height / 2
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		}

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scrollOffset += 3
		case tea.MouseButtonWheelDown:
			m.scrollOffset -= 3
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = msg.Width - 4

	case SpinnerTickMsg:
		m.spinnerIdx++
		cmds = append(cmds, m.tickSpinner())

	case TextDeltaMsg:
		m.state = stateStreaming
		m.currentText.WriteString(msg.Text)
		cmds = append(cmds, m.waitForAgentMsg())

	case ThinkingDeltaMsg:
		m.state = stateThinking
		m.currentThinking.WriteString(msg.Thinking)
		cmds = append(cmds, m.waitForAgentMsg())

	case ToolStartMsg:
		m.state = stateToolExecuting
		m.history = append(m.history, historyEntry{
			content: m.renderToolPanel(msg.Name, msg.ID, "", nil, false),
		})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case ToolResultMsg:
		r := msg.Result
		output := r.Result.Output
		if r.Result.Error != "" {
			output = r.Result.Error
		}
		m.history = append(m.history, historyEntry{
			content: m.renderToolPanel(r.Call.Name, r.Call.ID, r.Result.Lang, &output, r.Result.IsError),
		})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case StreamDoneMsg:
		// Flush current text to history
		if m.currentThinking.Len() > 0 {
			m.history = append(m.history, historyEntry{
				content: m.renderThinkingPanel(m.currentThinking.String()),
			})
			m.currentThinking.Reset()
		}
		if m.currentText.Len() > 0 {
			m.history = append(m.history, historyEntry{
				content: m.renderAssistantPanel(m.currentText.String()),
			})
			m.currentText.Reset()
		}
		m.state = stateIdle
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case ErrorMsg:
		m.history = append(m.history, historyEntry{
			content: m.renderErrorPanel(msg.Err.Error()),
		})
		m.state = stateIdle
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case TokenUpdateMsg:
		m.inputTokens = msg.InputTokens
		m.outputTokens = msg.OutputTokens
		m.cacheCreated = msg.CacheCreationInputTokens
		m.cacheRead = msg.CacheReadInputTokens
		m.ctxTokensUsed = msg.InputTokens
		cmds = append(cmds, m.waitForAgentMsg())

	case SetThemeMsg:
		if t, ok := Themes[msg.Name]; ok {
			m.theme = t
			m.bannerText = "" // invalidate banner cache
		}
		cmds = append(cmds, m.waitForAgentMsg())

	case SetModelMsg:
		m.model = msg.Name
		m.bannerText = "" // invalidate banner cache
		cmds = append(cmds, m.waitForAgentMsg())

	case StatusMsg:
		m.history = append(m.history, historyEntry{
			content: lipgloss.NewStyle().Foreground(m.theme.Dim).Render(msg.Text),
		})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case ModelsListMsg:
		var b strings.Builder
		b.WriteString("Available models:\n")
		for _, name := range msg.Models {
			b.WriteString(fmt.Sprintf("  - %s\n", name))
		}
		m.history = append(m.history, historyEntry{
			content: lipgloss.NewStyle().Foreground(m.theme.Secondary).Render(b.String()),
		})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case ClearMsg:
		m.history = nil
		m.scrollOffset = 0
		cmds = append(cmds, m.waitForAgentMsg())

	case SubAgentStartMsg:
		m.history = append(m.history, historyEntry{
			content: m.renderSubAgentPanel(msg.ID, msg.Task, msg.Model),
		})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case SubAgentStepMsg:
		step := lipgloss.NewStyle().Foreground(m.theme.Dim).
			Render(fmt.Sprintf("    ↳ [%s] %s", msg.ID, msg.ToolName))
		m.history = append(m.history, historyEntry{content: step})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case SubAgentDoneMsg:
		label := "done"
		color := m.theme.Secondary
		if msg.IsError {
			label = "failed"
			color = m.theme.Error
		}
		done := lipgloss.NewStyle().Foreground(color).
			Render(fmt.Sprintf("    ✓ [%s] %s", msg.ID, label))
		m.history = append(m.history, historyEntry{content: done})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case ContextTrimMsg:
		notice := lipgloss.NewStyle().Foreground(m.theme.Dim).Italic(true).
			Render(fmt.Sprintf("  [Context trimmed: %d messages dropped to fit window]", msg.Dropped))
		m.history = append(m.history, historyEntry{content: notice})
		m.scrollToBottom()
		cmds = append(cmds, m.waitForAgentMsg())

	case ConfirmRequestMsg:
		m.pendingConfirm = &msg
		cmds = append(cmds, m.waitForAgentMsg())

	case QuitMsg:
		m.quitting = true
		return m, tea.Quit
	}

	// Update text input
	if m.state == stateIdle {
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

	var sections []string

	// Banner — only render once and cache it
	if m.bannerText == "" {
		m.bannerText = Banner(m.theme, m.model)
	}
	sections = append(sections, m.bannerText)

	// History
	for _, entry := range m.visibleHistory() {
		sections = append(sections, entry.content)
	}

	// Current streaming content
	if m.state == stateThinking && m.currentThinking.Len() > 0 {
		sections = append(sections, m.renderThinkingPanel(m.currentThinking.String()))
	}
	if m.state == stateStreaming && m.currentText.Len() > 0 {
		sections = append(sections, m.renderAssistantPanel(m.currentText.String()))
	}

	// Spinner for thinking/executing states
	if m.state == stateThinking || m.state == stateToolExecuting {
		label := "Thinking"
		if m.state == stateToolExecuting {
			label = "Executing tools"
		}
		sections = append(sections, fmt.Sprintf(" %s %s...", m.renderSpinner(), label))
	}

	body := strings.Join(sections, "\n")

	// Calculate available height for body
	statusBarHeight := 1
	inputHeight := 2
	available := m.height - statusBarHeight - inputHeight - 1
	bodyLines := strings.Split(body, "\n")

	// Clamp scrollOffset to valid range
	maxOffset := len(bodyLines) - available
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}

	if len(bodyLines) > available && available > 0 {
		// Show a window: lines from [end-available-scrollOffset, end-scrollOffset]
		end := len(bodyLines) - m.scrollOffset
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
	if m.state == stateIdle {
		prompt = inputStyle.Render(m.textInput.View())
	} else {
		prompt = inputStyle.Render(
			lipgloss.NewStyle().Foreground(m.theme.Dim).Render("  (waiting for response...)"),
		)
	}

	// Status bar
	statusBar := m.renderStatusBar()

	// Confirmation dialog overlay — renders on top of everything else
	if m.pendingConfirm != nil {
		return body + "\n" + m.renderConfirmDialog() + "\n" + statusBar
	}

	return body + "\n" + prompt + "\n" + statusBar
}

func (m *Model) scrollToBottom() {
	m.scrollOffset = 0
}

func (m *Model) visibleHistory() []historyEntry {
	return m.history
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
