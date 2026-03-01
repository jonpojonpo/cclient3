package display

import (
	"fmt"
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
	history    []historyEntry
	scrollPos  int
	width      int
	height     int
	spinnerIdx int

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

	// Channel for sending user input to the agent
	InputChan chan string
	// Channel for receiving bubbletea messages from the agent
	AgentMsgChan chan tea.Msg

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
		width:        80,
		height:       24,
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
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			if m.state == stateIdle {
				text := strings.TrimSpace(m.textInput.Value())
				if text != "" {
					m.textInput.SetValue("")
					m.history = append(m.history, historyEntry{content: m.renderUserPanel(text)})
					m.scrollToBottom()
					m.InputChan <- text
				}
			}
		case tea.KeyPgUp:
			m.scrollPos -= 5
			if m.scrollPos < 0 {
				m.scrollPos = 0
			}
		case tea.KeyPgDown:
			m.scrollPos += 5
			if m.scrollPos > len(m.history) {
				m.scrollPos = len(m.history)
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
			content: m.renderToolPanel(msg.Name, msg.ID, nil, false),
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
			content: m.renderToolPanel(r.Call.Name, r.Call.ID, &output, r.Result.IsError),
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
		cmds = append(cmds, m.waitForAgentMsg())

	case SetThemeMsg:
		if t, ok := Themes[msg.Name]; ok {
			m.theme = t
		}
		cmds = append(cmds, m.waitForAgentMsg())

	case SetModelMsg:
		m.model = msg.Name
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
		m.scrollPos = 0
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

	// Banner
	banner := lipgloss.NewStyle().
		Foreground(m.theme.Primary).
		Bold(true).
		Render(fmt.Sprintf("  cclient3 [%s]", m.model))
	sections = append(sections, banner)

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
	if len(bodyLines) > available && available > 0 {
		bodyLines = bodyLines[len(bodyLines)-available:]
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

	return body + "\n" + prompt + "\n" + statusBar
}

func (m *Model) scrollToBottom() {
	m.scrollPos = len(m.history)
}

func (m *Model) visibleHistory() []historyEntry {
	// scrollPos represents how many entries from the end are visible as the "bottom".
	// We show entries from max(0, scrollPos-maxVisible) to scrollPos.
	end := m.scrollPos
	if end > len(m.history) {
		end = len(m.history)
	}
	if end <= 0 {
		return nil
	}
	return m.history[:end]
}
