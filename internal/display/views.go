package display

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Panel renders for different content types.

// renderPanel is the shared skeleton for all bordered panels.
func (m *Model) renderPanel(color lipgloss.Color, label, content string) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 1).
		Width(m.width - 4)
	lbl := lipgloss.NewStyle().Foreground(color).Bold(true).Render(label)
	return lbl + "\n" + style.Render(content)
}

func (m *Model) renderUserPanel(text string) string {
	return m.renderPanel(m.theme.Accent, "  You", text)
}

func (m *Model) renderAssistantPanel(text string) string {
	content := text
	if content == "" {
		content = " "
	} else {
		content = m.renderMarkdown(content)
	}
	return m.renderPanel(m.theme.Primary, "  Assistant", content)
}

// renderMarkdown renders markdown text using glamour with theme-matched styling.
func (m *Model) renderMarkdown(text string) string {
	// Content width inside the panel border + padding
	contentWidth := m.width - 8
	if contentWidth < 40 {
		contentWidth = 40
	}

	r := m.mdRenderer.Get(m.theme.Name, contentWidth)

	rendered, err := r.Render(text)
	if err != nil {
		return text
	}

	// Glamour adds trailing newlines — trim them
	return strings.TrimRight(rendered, "\n")
}

func (m *Model) renderThinkingPanel(text string) string {
	if len(text) > 500 {
		text = text[:500] + "..."
	}
	th := m.theme
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Dim).
		Padding(0, 1).
		Width(m.width - 4)
	label := lipgloss.NewStyle().Foreground(th.Dim).Italic(true).Render("  Thinking...")
	return label + "\n" + style.Render(text)
}

func (m *Model) renderToolPanel(name, id, lang string, result *string, isError bool) string {
	th := m.theme
	borderColor := th.Secondary
	if isError {
		borderColor = th.Error
	}

	var content string
	if result == nil {
		content = lipgloss.NewStyle().Foreground(th.Dim).Render("executing...")
	} else {
		r := *result
		if len(r) > 600 {
			r = r[:600] + "\n... (truncated)"
		}
		// Use language tag for syntax highlighting when known.
		fence := "```" + lang + "\n" + r + "\n```"
		content = m.renderMarkdown(fence)
	}

	label := lipgloss.NewStyle().Foreground(th.Secondary).Bold(true).Render(fmt.Sprintf("  Tool: %s", name))
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(m.width - 4)
	return label + "\n" + style.Render(content)
}

// renderSubAgentPanel renders a header card when a sub-agent is spawned.
// provider is shown alongside the model (e.g. "ollama" or "anthropic").
func (m *Model) renderSubAgentPanel(id, task, model, provider string) string {
	th := m.theme

	taskDisplay := task
	if len(taskDisplay) > 120 {
		taskDisplay = taskDisplay[:117] + "..."
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(th.Secondary).
		Padding(0, 1).
		Width(m.width - 4)

	providerTag := ""
	if provider != "" && provider != "anthropic" {
		providerTag = fmt.Sprintf(" via %s", provider)
	}
	label := lipgloss.NewStyle().Foreground(th.Secondary).Bold(true).
		Render(fmt.Sprintf("  Sub-agent [%s] — %s%s", id, model, providerTag))

	body := lipgloss.NewStyle().Foreground(th.Dim).Render(taskDisplay)

	return label + "\n" + style.Render(body)
}

func (m *Model) renderErrorPanel(text string) string {
	return m.renderPanel(m.theme.Error, "  Error", text)
}

// renderEnsembleHeader renders the agent roster at the top of an ensemble tab.
func (m *Model) renderEnsembleHeader(agents []EnsembleAgentInfo) string {
	th := m.theme
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(th.Accent).Render("  Ensemble Group Chat"))
	lines = append(lines, "")
	for _, a := range agents {
		color := lipgloss.Color(a.Color)
		name := lipgloss.NewStyle().Bold(true).Foreground(color).Render(a.Name)
		meta := lipgloss.NewStyle().Foreground(th.Dim).Render(
			fmt.Sprintf(" (%s via %s)", a.Model, a.Provider))
		desc := ""
		if a.Personality != "" {
			short := a.Personality
			if len(short) > 80 {
				short = short[:77] + "..."
			}
			desc = "\n    " + lipgloss.NewStyle().Foreground(th.Dim).Italic(true).Render(short)
		}
		lines = append(lines, "  "+name+meta+desc)
	}
	lines = append(lines, "")
	sep := lipgloss.NewStyle().Foreground(th.Dim).Render(strings.Repeat("─", m.width-4))
	lines = append(lines, sep)
	return strings.Join(lines, "\n")
}

// renderEnsembleSpeakerPanel renders a single agent's message in the ensemble.
func (m *Model) renderEnsembleSpeakerPanel(speaker, text, hexColor string) string {
	color := lipgloss.Color(hexColor)
	content := text
	if content != "" {
		content = m.renderMarkdown(content)
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 1).
		Width(m.width - 4)
	label := lipgloss.NewStyle().Foreground(color).Bold(true).Render("  " + speaker)
	return label + "\n" + style.Render(content)
}

// renderEnsembleUserPanel renders a user message in the ensemble chat.
func (m *Model) renderEnsembleUserPanel(text string) string {
	return m.renderPanel(m.theme.Accent, "  You", text)
}

func (m *Model) renderStatusBar() string {
	th := m.theme

	modelStyle := lipgloss.NewStyle().
		Foreground(th.Primary).
		Bold(true)

	themeStyle := lipgloss.NewStyle().
		Foreground(th.Secondary)

	tokenStyle := lipgloss.NewStyle().
		Foreground(th.Dim)

	left := modelStyle.Render(m.model)
	middle := themeStyle.Render(fmt.Sprintf("[%s]", m.theme.Name))

	scrollInfo := ""
	if offset := m.activeTab().scrollOffset; offset > 0 {
		scrollInfo = lipgloss.NewStyle().Foreground(th.Accent).Render(fmt.Sprintf(" [+%d] ", offset))
	}

	// Context window gauge (200k token limit)
	const ctxMax = 200000
	ctxPct := 0
	if m.ctxTokensUsed > 0 {
		ctxPct = m.ctxTokensUsed * 100 / ctxMax
		if ctxPct > 100 {
			ctxPct = 100
		}
	}
	ctxGauge := renderGauge(ctxPct, 8, m.theme)

	tokenInfo := fmt.Sprintf("in:%d out:%d", m.inputTokens, m.outputTokens)
	if m.cacheRead > 0 || m.cacheCreated > 0 {
		tokenInfo += fmt.Sprintf(" cache:%d", m.cacheRead)
	}
	// Cumulative session cost
	if m.sessionCostUSD >= 0.001 {
		tokenInfo += fmt.Sprintf(" cost:$%.3f", m.sessionCostUSD)
	} else if m.sessionCostUSD > 0 {
		tokenInfo += " cost:<$0.001"
	}
	right := tokenStyle.Render(ctxGauge + " ctx:" + fmt.Sprintf("%d%%", ctxPct) + " " + tokenInfo)

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(middle) - lipgloss.Width(scrollInfo) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}

	bar := lipgloss.NewStyle().
		Width(m.width).
		Background(lipgloss.Color("#1a1a1a")).
		Render(left + strings.Repeat(" ", gap/2) + middle + strings.Repeat(" ", gap-gap/2) + scrollInfo + right)

	return bar
}

func (m *Model) renderSpinner() string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frame := frames[m.spinnerIdx%len(frames)]
	return lipgloss.NewStyle().Foreground(m.theme.Primary).Render(frame)
}

// renderGauge renders a mini ASCII bar showing pct% filled out of width chars.
func renderGauge(pct, width int, th Theme) string {
	filled := pct * width / 100
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	color := th.Primary
	if pct >= 80 {
		color = th.Accent
	}
	if pct >= 95 {
		color = th.Error
	}
	return lipgloss.NewStyle().Foreground(color).Render("[" + bar + "]")
}
