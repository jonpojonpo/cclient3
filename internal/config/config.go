package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Model              string `yaml:"model"`
	MaxTokens          int    `yaml:"max_tokens"`
	Theme              string `yaml:"theme"`
	MaxToolConcurrency int    `yaml:"max_tool_concurrency"`
	BashTimeout        int    `yaml:"bash_timeout"`
	APIEndpoint        string `yaml:"api_endpoint"`
	SystemPrompt       string `yaml:"system_prompt"`
	APIKey             string `yaml:"-"`
	// Effort controls reasoning depth vs cost on models that support it
	// (low | medium | high | xhigh | max). Empty uses the API default (high).
	Effort string `yaml:"effort"`
	// FastModel is used for lightweight sub-tasks (delegation, summarising).
	FastModel string `yaml:"fast_model"`
	// ContextSize is the model's context window in tokens. Used for conversation
	// trimming. Default 1000000 suits current Claude models; set lower for
	// Ollama (e.g. 8192).
	ContextSize int `yaml:"context_size"`
	// Local / alternative provider settings
	OllamaEndpoint  string `yaml:"ollama_endpoint"`
	OllamaModel     string `yaml:"ollama_model"`
	OpenAIEndpoint  string `yaml:"openai_endpoint"`
	OpenAIModel     string `yaml:"openai_model"`
	DefaultProvider string `yaml:"default_provider"`
	OpenAIAPIKey    string `yaml:"-"`
}

func DefaultConfig() *Config {
	return &Config{
		Model:              "claude-opus-4-8",
		FastModel:          "claude-haiku-4-5",
		MaxTokens:          16384,
		Theme:              "cyber",
		MaxToolConcurrency: 6,
		BashTimeout:        120,
		ContextSize:        1000000,
		APIEndpoint:        "https://api.anthropic.com/v1/messages",
		OllamaEndpoint:     "http://localhost:11434",
		OllamaModel:        "qwen3.6:27b",
		OpenAIEndpoint:     "https://api.openai.com",
		OpenAIModel:        "gpt-5.5",
		DefaultProvider:    "anthropic",
		SystemPrompt:       defaultSystemPrompt,
	}
}

const defaultSystemPrompt = `You are a powerful AI agent with access to tools for reading, writing, and
searching files, running bash commands, fetching web pages, and spawning
autonomous sub-agents.

## Dynamic workflows
Decompose work to fit the task, not a fixed script:
- Independent tool calls (reads, greps, searches) belong in the SAME response —
  they run in parallel.
- Use the sub_agent tool to delegate self-contained or parallelisable
  workstreams. Multiple sub_agent calls in one response run concurrently.
  Give each sub-agent a complete, self-contained task description — it cannot
  see this conversation.
- Route sub-tasks to the cheapest model that can do them well: use a fast
  model for parsing, summarising, formatting, or light research, and the
  default model for design, coding, and complex reasoning.
- Sub-agents can themselves delegate one further level (sub-sub-agents) for
  very large tasks; depth is capped, so keep hierarchies shallow and prefer
  wide, parallel fan-out over deep nesting.

## Working style
When you have enough information to act, act — don't re-derive established
facts or narrate options you won't pursue. Verify your work with tools
(run the code, re-read the file) before declaring it done. Report outcomes
faithfully: if something failed, say so with the evidence. Be concise.`

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

// ModelFor resolves the model to use for a given provider. If the configured
// Model is a Claude model but the provider is ollama/openai, fall back to that
// provider's configured model — sending a Claude model ID to another backend
// is always an error.
func (c *Config) ModelFor(provider string) string {
	if !strings.HasPrefix(c.Model, "claude-") {
		return c.Model // user explicitly picked a non-Claude model
	}
	switch provider {
	case "ollama":
		if c.OllamaModel != "" {
			return c.OllamaModel
		}
	case "openai":
		if c.OpenAIModel != "" {
			return c.OpenAIModel
		}
	}
	return c.Model
}
