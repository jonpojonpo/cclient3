package display

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderTabBar renders the horizontal tab bar across the top of the screen.
// If tabs exceed the terminal width, shows < / > overflow indicators and
// ensures the active tab is always visible.
func (m *Model) renderTabBar() string {
	allTabs := m.tabs.Tabs()
	activeIdx := m.tabs.ActiveIdx()

	// Render all tabs
	rendered := make([]string, len(allTabs))
	widths := make([]int, len(allTabs))
	for i, tab := range allTabs {
		rendered[i] = m.renderSingleTab(tab, i, i == activeIdx)
		// Width of the rendered tab (first line, since borders add height)
		lines := strings.Split(rendered[i], "\n")
		maxW := 0
		for _, line := range lines {
			if w := lipgloss.Width(line); w > maxW {
				maxW = w
			}
		}
		widths[i] = maxW
	}

	// Check if all tabs fit
	totalWidth := 0
	for _, w := range widths {
		totalWidth += w
	}

	if totalWidth <= m.width {
		bar := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
		return bar
	}

	// Tabs overflow — show a window around the active tab
	// Reserve space for overflow indicators
	arrowWidth := 3 // " < " or " > "
	availWidth := m.width - arrowWidth*2

	// Start from active tab and expand outward
	start, end := activeIdx, activeIdx
	usedWidth := widths[activeIdx]

	// Expand left, then right, alternating
	for {
		expanded := false
		if start > 0 && usedWidth+widths[start-1] <= availWidth {
			start--
			usedWidth += widths[start]
			expanded = true
		}
		if end < len(allTabs)-1 && usedWidth+widths[end+1] <= availWidth {
			end++
			usedWidth += widths[end]
			expanded = true
		}
		if !expanded {
			break
		}
	}

	// Build the visible tab bar
	visible := rendered[start : end+1]
	bar := lipgloss.JoinHorizontal(lipgloss.Top, visible...)

	// Add overflow arrows
	th := m.theme
	leftArrow := ""
	rightArrow := ""
	if start > 0 {
		leftArrow = lipgloss.NewStyle().Foreground(th.Dim).Render(" < ")
	} else {
		leftArrow = "   "
	}
	if end < len(allTabs)-1 {
		rightArrow = lipgloss.NewStyle().Foreground(th.Dim).Render(" > ")
	} else {
		rightArrow = "   "
	}

	// Arrows need to be on the same line as the tabs — combine them
	barLines := strings.Split(bar, "\n")
	if len(barLines) > 0 {
		// Add arrows to first line only (where the tab content is)
		for i, line := range barLines {
			if i == 0 {
				barLines[i] = leftArrow + line + rightArrow
			}
		}
	}

	return strings.Join(barLines, "\n")
}

// renderSingleTab renders one tab in the tab bar.
func (m *Model) renderSingleTab(tab *Tab, index int, isActive bool) string {
	th := m.theme

	label := fmt.Sprintf(" %d: %s", index+1, tab.Label)

	// Status icon
	icon := m.renderStatusIcon(tab)
	if icon != "" {
		label += " " + icon
	}

	// Unread badge
	if tab.Unread && !isActive {
		label += " " + lipgloss.NewStyle().Foreground(th.Accent).Render("•")
	}

	// Pin indicator
	if tab.UserPinned {
		label += " " + lipgloss.NewStyle().Foreground(th.Dim).Render("📌")
	}

	label += " "

	if isActive {
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(th.Primary).
			BorderBottom(false).
			Bold(true).
			Foreground(th.Text).
			Render(label)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Dim).
		BorderBottom(false).
		Foreground(th.Dim).
		Render(label)
}

// renderStatusIcon returns a status icon string for the tab.
func (m *Model) renderStatusIcon(tab *Tab) string {
	th := m.theme

	if tab.Kind == TabChat {
		return "" // no status icon for chat
	}

	switch tab.Status {
	case TabRunning:
		// Use spinner frame for active running tabs
		if tab.state != stateIdle {
			frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			frame := frames[m.spinnerIdx%len(frames)]
			return lipgloss.NewStyle().Foreground(th.Primary).Render(frame)
		}
		return lipgloss.NewStyle().Foreground(th.Primary).Render("●")
	case TabDone:
		return lipgloss.NewStyle().Foreground(th.Accent).Render("✓")
	case TabError:
		return lipgloss.NewStyle().Foreground(th.Error).Render("✗")
	}
	return ""
}
