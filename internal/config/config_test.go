package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Model == "" {
		t.Error("default model should not be empty")
	}
	if cfg.MaxTokens <= 0 {
		t.Errorf("default MaxTokens = %d, want > 0", cfg.MaxTokens)
	}
	if cfg.Temperature < 0 || cfg.Temperature > 2 {
		t.Errorf("default Temperature = %f, want 0..2", cfg.Temperature)
	}
	if cfg.Theme == "" {
		t.Error("default theme should not be empty")
	}
	if cfg.MaxToolConcurrency <= 0 {
		t.Errorf("default MaxToolConcurrency = %d, want > 0", cfg.MaxToolConcurrency)
	}
	if cfg.BashTimeout <= 0 {
		t.Errorf("default BashTimeout = %d, want > 0", cfg.BashTimeout)
	}
	if cfg.APIEndpoint == "" {
		t.Error("default APIEndpoint should not be empty")
	}
	if cfg.SystemPrompt == "" {
		t.Error("default SystemPrompt should not be empty")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-12345")
	t.Setenv("CLAUDE_MODEL", "claude-haiku-4-5-20251001")
	t.Setenv("OPENAI_API_KEY", "openai-test-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.APIKey != "test-key-12345" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "test-key-12345")
	}
	if cfg.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("Model = %q, want %q", cfg.Model, "claude-haiku-4-5-20251001")
	}
	if cfg.OpenAIAPIKey != "openai-test-key" {
		t.Errorf("OpenAIAPIKey = %q, want %q", cfg.OpenAIAPIKey, "openai-test-key")
	}
}

func TestLoad_YAMLFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	yamlContent := `model: claude-test-model
max_tokens: 4096
temperature: 0.5
theme: ocean
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Temporarily change working directory so config.yaml is found
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Model != "claude-test-model" {
		t.Errorf("Model = %q, want claude-test-model", cfg.Model)
	}
	if cfg.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", cfg.MaxTokens)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("Temperature = %f, want 0.5", cfg.Temperature)
	}
	if cfg.Theme != "ocean" {
		t.Errorf("Theme = %q, want ocean", cfg.Theme)
	}
}

func TestLoad_NoConfigFile_UsesDefaults(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Unset env overrides
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_MODEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	defaults := DefaultConfig()
	if cfg.MaxTokens != defaults.MaxTokens {
		t.Errorf("MaxTokens = %d, want default %d", cfg.MaxTokens, defaults.MaxTokens)
	}
	if cfg.Theme != defaults.Theme {
		t.Errorf("Theme = %q, want default %q", cfg.Theme, defaults.Theme)
	}
}
