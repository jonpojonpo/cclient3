package display

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	Name      string
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Accent    lipgloss.Color
	Dim       lipgloss.Color
	Error     lipgloss.Color
	BG        lipgloss.Color
	Text      lipgloss.Color
}

var Themes = map[string]Theme{
	"cyber": {
		Name:      "cyber",
		Primary:   lipgloss.Color("#00FFFF"),
		Secondary: lipgloss.Color("#FF00FF"),
		Accent:    lipgloss.Color("#00FF00"),
		Dim:       lipgloss.Color("#666666"),
		Error:     lipgloss.Color("#FF0000"),
		BG:        lipgloss.Color("#000000"),
		Text:      lipgloss.Color("#FFFFFF"),
	},
	"ocean": {
		Name:      "ocean",
		Primary:   lipgloss.Color("#5B9BD5"),
		Secondary: lipgloss.Color("#70AD47"),
		Accent:    lipgloss.Color("#FFC000"),
		Dim:       lipgloss.Color("#808080"),
		Error:     lipgloss.Color("#FF6B6B"),
		BG:        lipgloss.Color("#0D1117"),
		Text:      lipgloss.Color("#E6EDF3"),
	},
	"ember": {
		Name:      "ember",
		Primary:   lipgloss.Color("#FF6B35"),
		Secondary: lipgloss.Color("#F7C948"),
		Accent:    lipgloss.Color("#E84855"),
		Dim:       lipgloss.Color("#888888"),
		Error:     lipgloss.Color("#FF0000"),
		BG:        lipgloss.Color("#1A1A2E"),
		Text:      lipgloss.Color("#EAEAEA"),
	},
	"mono": {
		Name:      "mono",
		Primary:   lipgloss.Color("#CCCCCC"),
		Secondary: lipgloss.Color("#999999"),
		Accent:    lipgloss.Color("#FFFFFF"),
		Dim:       lipgloss.Color("#555555"),
		Error:     lipgloss.Color("#FF4444"),
		BG:        lipgloss.Color("#000000"),
		Text:      lipgloss.Color("#AAAAAA"),
	},
}

func ThemeNames() []string {
	return []string{"cyber", "ocean", "ember", "mono"}
}
