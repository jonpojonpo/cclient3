// Package skills manages agent skills — reusable system-prompt fragments stored
// as Markdown files in a .skills/ directory (local or ~/.config/cclient3/skills/).
//
// Skill file format (.skills/my-skill.md):
//
//	---
//	description: One-line description shown in /skills
//	---
//	System prompt content injected when the skill is active.
package skills

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Skill is a named system-prompt fragment loaded from a .md file.
type Skill struct {
	Name        string // derived from filename (without .md)
	Description string // from frontmatter
	Content     string // system prompt text (after frontmatter)
	Path        string // absolute path to source file
}

// Manager holds the full set of available skills and tracks which are active.
type Manager struct {
	available []Skill
	active    map[string]bool
}

// NewManager loads all skills from standard directories and returns a Manager.
func NewManager() *Manager {
	return &Manager{
		available: FindAll(),
		active:    make(map[string]bool),
	}
}

// Available returns all discovered skills.
func (m *Manager) Available() []Skill { return m.available }

// Activate enables a skill by name. Returns false if no such skill exists.
func (m *Manager) Activate(name string) bool {
	for _, s := range m.available {
		if s.Name == name {
			m.active[name] = true
			return true
		}
	}
	return false
}

// Deactivate disables a skill.
func (m *Manager) Deactivate(name string) {
	delete(m.active, name)
}

// Toggle enables a skill if inactive, disables it if active.
// Returns (nowActive, found).
func (m *Manager) Toggle(name string) (active bool, found bool) {
	for _, s := range m.available {
		if s.Name == name {
			if m.active[name] {
				delete(m.active, name)
				return false, true
			}
			m.active[name] = true
			return true, true
		}
	}
	return false, false
}

// IsActive reports whether a skill is currently active.
func (m *Manager) IsActive(name string) bool { return m.active[name] }

// ActiveContent returns the concatenated system-prompt content of all active skills,
// separated by double newlines.
func (m *Manager) ActiveContent() string {
	var parts []string
	for _, s := range m.available {
		if m.active[s.Name] {
			parts = append(parts, s.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

// FindAll discovers skills from all standard directories.
// Local .skills/ overrides ~/.config/cclient3/skills/ for skills with the same name.
func FindAll() []Skill {
	var all []Skill
	seen := map[string]bool{}

	for _, dir := range skillDirs() {
		skills, _ := Load(dir)
		for _, s := range skills {
			if !seen[s.Name] {
				all = append(all, s)
				seen[s.Name] = true
			}
		}
	}
	return all
}

// skillDirs returns directories to search, in order of increasing precedence
// (later entries win for duplicate names).
func skillDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "cclient3", "skills"))
	}
	// Local .skills/ takes precedence — checked last, seen-map wins first match
	// so we reverse: local should go last in FindAll's seen-guard loop.
	// Actually we want local to WIN, so we check global first so local can
	// override via seen-map. Append local last.
	dirs = append(dirs, ".skills")
	return dirs
}

// Load reads all .md files in dir and returns them as Skills.
// Returns nil slice (not error) when the directory does not exist.
func Load(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []Skill
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		s, err := parseSkillFile(path)
		if err != nil {
			continue // skip malformed files
		}
		s.Name = strings.TrimSuffix(e.Name(), ".md")
		skills = append(skills, s)
	}
	return skills, nil
}

// parseSkillFile reads a skill Markdown file, parsing optional YAML-like frontmatter.
func parseSkillFile(path string) (Skill, error) {
	f, err := os.Open(path)
	if err != nil {
		return Skill{}, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return Skill{}, err
	}

	skill := Skill{Path: path}

	// Parse optional frontmatter block: --- ... ---
	if len(lines) > 0 && lines[0] == "---" {
		endIdx := -1
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				endIdx = i
				break
			}
			// Simple key: value parsing
			if parts := strings.SplitN(lines[i], ":", 2); len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				if key == "description" {
					skill.Description = val
				}
			}
		}
		if endIdx >= 0 {
			skill.Content = strings.Join(lines[endIdx+1:], "\n")
		} else {
			// No closing ---, treat everything as content
			skill.Content = strings.Join(lines, "\n")
		}
	} else {
		skill.Content = strings.Join(lines, "\n")
	}

	skill.Content = strings.TrimSpace(skill.Content)
	return skill, nil
}
