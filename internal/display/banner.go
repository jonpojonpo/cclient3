package display

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Banner generates a styled ASCII art banner for the given theme.
// It tries toilet (with colour), then figlet, then falls back to a plain box.
func Banner(theme Theme, modelName string) string {
	art := renderArt(theme)
	subtitle := renderSubtitle(theme, modelName)
	return art + "\n" + subtitle
}

// renderArt tries toilet --gay, then toilet plain, then figlet, then hardcoded fallback.
func renderArt(theme Theme) string {
	// 1. Try toilet with rainbow colours
	if out, err := runCmd("toilet", "-f", "mono12", "--gay", "cclient3"); err == nil {
		return out
	}
	// 2. Try toilet plain (no colour — we'll colour it ourselves)
	if out, err := runCmd("toilet", "-f", "mono12", "cclient3"); err == nil {
		return colourLines(out, theme)
	}
	// 3. Try figlet slant
	if out, err := runCmd("figlet", "-f", "slant", "cclient3"); err == nil {
		return colourLines(out, theme)
	}
	// 4. Try figlet standard
	if out, err := runCmd("figlet", "cclient3"); err == nil {
		return colourLines(out, theme)
	}
	// 5. Hardcoded fallback block
	return colourLines(fallbackArt, theme)
}

func renderSubtitle(theme Theme, modelName string) string {
	tag := fmt.Sprintf("[ claude client  •  model: %s ]", modelName)
	return lipgloss.NewStyle().
		Foreground(theme.Secondary).
		Bold(true).
		PaddingLeft(2).
		Render(tag)
}

// colourLines applies alternating theme colours to each line of ASCII art.
func colourLines(art string, theme Theme) string {
	colours := []lipgloss.Color{theme.Primary, theme.Secondary, theme.Accent}
	lines := strings.Split(art, "\n")
	var b strings.Builder
	ci := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(lipgloss.NewStyle().Foreground(colours[ci%len(colours)]).Render(line))
		b.WriteString("\n")
		ci++
	}
	return b.String()
}

func runCmd(name string, args ...string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(path, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	out := stdout.String()
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("empty output")
	}
	return out, nil
}

const fallbackArt = `
  ██████╗ ██████╗██╗     ██╗███████╗███╗   ██╗████████╗██████╗ 
 ██╔════╝██╔════╝██║     ██║██╔════╝████╗  ██║╚══██╔══╝╚════██╗
 ██║     ██║     ██║     ██║█████╗  ██╔██╗ ██║   ██║    █████╔╝
 ██║     ██║     ██║     ██║██╔══╝  ██║╚██╗██║   ██║    ╚═══██╗
 ╚██████╗╚██████╗███████╗██║███████╗██║ ╚████║   ██║   ██████╔╝
  ╚═════╝ ╚═════╝╚══════╝╚═╝╚══════╝╚═╝  ╚═══╝   ╚═╝   ╚═════╝ 
`
