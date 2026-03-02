package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkillFile_WithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := "---\ndescription: A test skill\n---\nThis is the skill content.\nSecond line."
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	skill, err := parseSkillFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.Description != "A test skill" {
		t.Errorf("description = %q, want %q", skill.Description, "A test skill")
	}
	if skill.Content != "This is the skill content.\nSecond line." {
		t.Errorf("content = %q, want content after frontmatter", skill.Content)
	}
}

func TestParseSkillFile_WithoutFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.md")
	content := "No frontmatter here.\nJust raw content."
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	skill, err := parseSkillFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.Description != "" {
		t.Errorf("expected empty description, got %q", skill.Description)
	}
	if skill.Content != "No frontmatter here.\nJust raw content." {
		t.Errorf("content = %q", skill.Content)
	}
}

func TestParseSkillFile_UnclosedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unclosed.md")
	// No closing ---; everything should be treated as content
	content := "---\ndescription: Broken\nActual content."
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	skill, err := parseSkillFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Without a closing ---, content should include all lines
	if skill.Content == "" {
		t.Errorf("expected non-empty content for unclosed frontmatter")
	}
}

func TestLoad_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	skills, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills in empty dir, got %d", len(skills))
	}
}

func TestLoad_NonexistentDir(t *testing.T) {
	skills, err := Load("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if skills != nil {
		t.Errorf("expected nil slice for missing dir")
	}
}

func TestLoad_MultipleSkills(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"alpha.md": "---\ndescription: Alpha\n---\nAlpha content.",
		"beta.md":  "---\ndescription: Beta\n---\nBeta content.",
		"notes.txt": "not a skill",
	}
	for name, body := range files {
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
	}

	skills, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 2 {
		t.Errorf("expected 2 skills (ignoring .txt), got %d", len(skills))
	}
	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("expected alpha and beta skills, got: %v", names)
	}
}

func TestManager_ToggleAndActiveContent(t *testing.T) {
	mgr := &Manager{
		available: []Skill{
			{Name: "a", Content: "Content A"},
			{Name: "b", Content: "Content B"},
		},
		active: make(map[string]bool),
	}

	// Toggle a on
	active, found := mgr.Toggle("a")
	if !found {
		t.Error("expected skill 'a' to be found")
	}
	if !active {
		t.Error("expected skill 'a' to be active after first toggle")
	}

	content := mgr.ActiveContent()
	if content != "Content A" {
		t.Errorf("ActiveContent() = %q, want %q", content, "Content A")
	}

	// Toggle a off
	active, _ = mgr.Toggle("a")
	if active {
		t.Error("expected skill 'a' to be inactive after second toggle")
	}
	if mgr.ActiveContent() != "" {
		t.Errorf("expected empty ActiveContent after deactivation")
	}

	// Unknown skill
	_, found = mgr.Toggle("unknown")
	if found {
		t.Error("expected not found for unknown skill")
	}
}

func TestManager_MultipleActiveSkills(t *testing.T) {
	mgr := &Manager{
		available: []Skill{
			{Name: "x", Content: "X content"},
			{Name: "y", Content: "Y content"},
		},
		active: make(map[string]bool),
	}
	mgr.Activate("x")
	mgr.Activate("y")

	combined := mgr.ActiveContent()
	if combined == "" {
		t.Error("expected combined content")
	}
	if !contains(combined, "X content") || !contains(combined, "Y content") {
		t.Errorf("expected both skill contents, got: %q", combined)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
