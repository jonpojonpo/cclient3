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
	"nord": {
		Name:      "nord",
		Primary:   lipgloss.Color("#88C0D0"),
		Secondary: lipgloss.Color("#81A1C1"),
		Accent:    lipgloss.Color("#EBCB8B"),
		Dim:       lipgloss.Color("#616E88"),
		Error:     lipgloss.Color("#BF616A"),
		BG:        lipgloss.Color("#2E3440"),
		Text:      lipgloss.Color("#D8DEE9"),
	},
	"gruvbox": {
		Name:      "gruvbox",
		Primary:   lipgloss.Color("#FABD2F"),
		Secondary: lipgloss.Color("#83A598"),
		Accent:    lipgloss.Color("#FE8019"),
		Dim:       lipgloss.Color("#928374"),
		Error:     lipgloss.Color("#FB4934"),
		BG:        lipgloss.Color("#282828"),
		Text:      lipgloss.Color("#EBDBB2"),
	},
	"forest": {
		Name:      "forest",
		Primary:   lipgloss.Color("#52B788"),
		Secondary: lipgloss.Color("#74C69D"),
		Accent:    lipgloss.Color("#D4AC0D"),
		Dim:       lipgloss.Color("#40916C"),
		Error:     lipgloss.Color("#BC4749"),
		BG:        lipgloss.Color("#1B2D23"),
		Text:      lipgloss.Color("#D4E6C3"),
	},
	"rose": {
		Name:      "rose",
		Primary:   lipgloss.Color("#FF85A1"),
		Secondary: lipgloss.Color("#E8A0BF"),
		Accent:    lipgloss.Color("#FFCBA4"),
		Dim:       lipgloss.Color("#C9A0DC"),
		Error:     lipgloss.Color("#FF4757"),
		BG:        lipgloss.Color("#2D1B2E"),
		Text:      lipgloss.Color("#F5E6E8"),
	},
}

func ThemeNames() []string {
	return []string{"cyber", "ocean", "ember", "mono", "nord", "gruvbox", "forest", "rose"}
}
