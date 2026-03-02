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

func (m *Model) renderToolPanel(name, id string, result *string, isError bool) string {
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
		if len(r) > 500 {
			r = r[:500] + "\n... (truncated)"
		}
		// Wrap tool output in a code fence so glamour preserves
		// newlines and applies syntax highlighting as preformatted text.
		content = m.renderMarkdown("```\n" + r + "\n```")
	}

	label := lipgloss.NewStyle().Foreground(th.Secondary).Bold(true).Render(fmt.Sprintf("  Tool: %s", name))
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(m.width - 4)
	return label + "\n" + style.Render(content)
}

func (m *Model) renderErrorPanel(text string) string {
	return m.renderPanel(m.theme.Error, "  Error", text)
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
	if m.scrollOffset > 0 {
		scrollInfo = lipgloss.NewStyle().Foreground(th.Accent).Render(fmt.Sprintf(" [+%d] ", m.scrollOffset))
	}

	tokenInfo := fmt.Sprintf("tokens: %d/%d", m.inputTokens, m.outputTokens)
	if m.cacheRead > 0 || m.cacheCreated > 0 {
		tokenInfo += fmt.Sprintf(" cache: %d/%d", m.cacheRead, m.cacheCreated)
	}
	right := tokenStyle.Render(tokenInfo)

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
