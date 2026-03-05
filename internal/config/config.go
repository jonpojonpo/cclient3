package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// EnsembleAgentDef defines a single agent in an ensemble preset.
type EnsembleAgentDef struct {
	Name        string `yaml:"name"`
	Personality string `yaml:"personality"`
	Model       string `yaml:"model"`
	Provider    string `yaml:"provider"`
	Color       string `yaml:"color"`
}

// EnsemblePreset is a named group of agents for ensemble mode.
type EnsemblePreset struct {
	Name   string             `yaml:"name"`
	Agents []EnsembleAgentDef `yaml:"agents"`
}

type Config struct {
	Model              string  `yaml:"model"`
	MaxTokens          int     `yaml:"max_tokens"`
	Temperature        float64 `yaml:"temperature"`
	Theme              string  `yaml:"theme"`
	MaxToolConcurrency int     `yaml:"max_tool_concurrency"`
	BashTimeout        int     `yaml:"bash_timeout"`
	APIEndpoint        string  `yaml:"api_endpoint"`
	SystemPrompt       string  `yaml:"system_prompt"`
	APIKey             string  `yaml:"-"`
	// ContextSize is the model's context window in tokens. Used for conversation
	// trimming. Default 190000 suits Claude; set lower for Ollama (e.g. 8192).
	ContextSize int `yaml:"context_size"`
	// Local / alternative provider settings
	OllamaEndpoint  string `yaml:"ollama_endpoint"`
	OllamaModel     string `yaml:"ollama_model"`
	OpenAIEndpoint  string `yaml:"openai_endpoint"`
	OpenAIModel     string `yaml:"openai_model"`
	DefaultProvider string `yaml:"default_provider"`
	OpenAIAPIKey    string `yaml:"-"`
	// Ensemble mode presets
	EnsemblePresets []EnsemblePreset `yaml:"ensemble_presets"`
}

func DefaultConfig() *Config {
	return &Config{
		Model:              "claude-sonnet-4-6",
		MaxTokens:          8192,
		Temperature:        0.7,
		Theme:              "cyber",
		MaxToolConcurrency: 6,
		BashTimeout:        120,
		ContextSize:        190000,
		APIEndpoint:        "https://api.anthropic.com/v1/messages",
		OllamaEndpoint:     "http://localhost:11434",
		OllamaModel:        "qwen3.5:9b",
		OpenAIEndpoint:     "https://api.openai.com",
		OpenAIModel:        "gpt-5.4",
		DefaultProvider:    "anthropic",
		SystemPrompt:       "You are a helpful AI assistant with access to tools for reading, writing, and searching files, running bash commands, and more. Use tools when appropriate to help the user.",
	}
}

func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Try config file locations in order
	paths := []string{
		"config.yaml",
	}
	home, err := os.UserHomeDir()
	if err == nil {
		paths = append([]string{filepath.Join(home, ".config", "cclient3", "config.yaml")}, paths...)
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
		break
	}

	// Env overrides
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		cfg.APIKey = key
	}
	if model := os.Getenv("CLAUDE_MODEL"); model != "" {
		cfg.Model = model
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cfg.OpenAIAPIKey = key
	}

	return cfg, nil
}
