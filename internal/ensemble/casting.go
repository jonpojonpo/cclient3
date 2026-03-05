package ensemble

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jonpo/cclient3/internal/api"
)

// CastAgents uses the AI to dynamically generate an agent roster based on the prompt.
// It asks the default provider to analyze the prompt and create 2-4 diverse agents
// with appropriate personalities, assigning them to different providers/models when available.
func CastAgents(
	ctx context.Context,
	provider api.Provider,
	model string,
	prompt string,
	availableProviders []string,
	availableModels []string,
) ([]AgentSpec, error) {

	providerList := strings.Join(availableProviders, ", ")
	modelList := "the default model"
	if len(availableModels) > 0 {
		modelList = strings.Join(availableModels, ", ")
	}

	castingPrompt := fmt.Sprintf(`You are a casting director for an AI ensemble discussion.

Given a user's prompt, create 2-4 AI agents with diverse perspectives to discuss it.
Each agent should have a unique name, personality, and if possible, use different AI providers/models for diversity.

Available providers: %s
Available models: %s

The user's prompt is:
%s

Respond with ONLY a JSON array of agent objects. No other text. Example format:
[
  {"name": "Nova", "personality": "A creative visionary who thinks big and explores unconventional ideas", "model": "", "provider": "", "color": "#FF6B6B"},
  {"name": "Atlas", "personality": "A methodical analyst who focuses on data, facts, and practical feasibility", "model": "", "provider": "", "color": "#4ECDC4"}
]

Rules:
- 2-4 agents total
- Give them memorable, short names (not "Agent 1")
- Personalities should create productive tension and diverse viewpoints
- Leave model/provider empty strings to use defaults, or assign specific ones for diversity
- Use hex colors that are visually distinct`, providerList, modelList, prompt)

	req := &api.Request{
		Model:     model,
		MaxTokens: 1024,
		System:    "You are a casting director. Respond only with valid JSON.",
		Messages: []api.Message{
			{Role: "user", Content: castingPrompt},
		},
	}

	resp, err := provider.SendMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("casting agent failed: %w", err)
	}

	// Extract text from response
	var responseText string
	for _, block := range resp.Content {
		if block.Type == "text" {
			responseText += block.Text
		}
	}

	// Parse JSON from response (handle markdown code fences)
	responseText = strings.TrimSpace(responseText)
	if strings.HasPrefix(responseText, "```") {
		lines := strings.Split(responseText, "\n")
		// Remove first and last lines (code fences)
		if len(lines) > 2 {
			responseText = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var agents []AgentSpec
	if err := json.Unmarshal([]byte(responseText), &agents); err != nil {
		return nil, fmt.Errorf("failed to parse agent roster: %w (response: %s)", err, responseText)
	}

	if len(agents) < 2 {
		return nil, fmt.Errorf("casting agent produced %d agents (need at least 2)", len(agents))
	}
	if len(agents) > 6 {
		agents = agents[:6]
	}

	return agents, nil
}

// DefaultPresets returns built-in ensemble presets.
func DefaultPresets() []AgentSpec {
	return []AgentSpec{
		{
			Name:        "Sage",
			Personality: "A wise, experienced architect who values clean design, maintainability, and proven patterns. Thinks about long-term consequences.",
			Color:       "#4ECDC4",
		},
		{
			Name:        "Spark",
			Personality: "A creative innovator who challenges assumptions, suggests novel approaches, and isn't afraid of unconventional solutions.",
			Model:       "qwen3.5:9b",
			Provider:    "ollama",
			Color:       "#FF6B6B",
		},
		{
			Name:        "Sentinel",
			Personality: "A security-minded skeptic who stress-tests ideas, finds edge cases, and ensures nothing is overlooked. Asks 'what could go wrong?'",
			Color:       "#FFE66D",
		},
	}
}
