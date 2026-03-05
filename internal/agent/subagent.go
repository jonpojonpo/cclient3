package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonpo/cclient3/internal/api"
	"github.com/jonpo/cclient3/internal/config"
	"github.com/jonpo/cclient3/internal/display"
	"github.com/jonpo/cclient3/internal/tools"
)

// SubAgentTool spawns a fully autonomous child agent.
// Multiple sub_agent calls in one response execute in parallel.
type SubAgentTool struct {
	providers *api.ProviderRegistry
	cfg       *config.Config
	registry  *tools.Registry // child registry — no sub_agent
	executor  *tools.Executor
	msgChan   chan tea.Msg
	counter   atomic.Int64
}

func NewSubAgentTool(
	providers *api.ProviderRegistry,
	cfg *config.Config,
	childRegistry *tools.Registry,
	msgChan chan tea.Msg,
) *SubAgentTool {
	return &SubAgentTool{
		providers: providers,
		cfg:       cfg,
		registry:  childRegistry,
		executor:  tools.NewExecutor(childRegistry, 6),
		msgChan:   msgChan,
	}
}

func (t *SubAgentTool) Name() string { return "sub_agent" }

func (t *SubAgentTool) Description() string {
	return `Spawn a fully autonomous sub-agent to complete a task independently.

The sub-agent has its own conversation context and access to all tools
(bash, file_read, file_write, file_edit, glob, grep, web_fetch, web_search).
It runs its own tool-use loop until it produces a final answer.

Multiple sub_agent calls in a single response run IN PARALLEL — use this
to decompose complex work into concurrent workstreams.

Optionally specify 'provider' to route this agent to a different backend:
  - "anthropic" (default): full Claude API access
  - "ollama": local inference (uses configured ollama_model, e.g. qwen3.5:9b)

Optionally specify 'model' to use a specific model:
  - claude-sonnet-4-6: best capability (default)
  - claude-haiku-4-5-20251001: fast and cheap — ideal for simple sub-tasks
    like parsing, summarising, formatting, or light research
  - qwen3.5:9b: local/offline via ollama

The sub-agent's final answer is returned as a tool result string.`
}

func (t *SubAgentTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {
				"type": "string",
				"description": "Full self-contained task description. Include all necessary context — the sub-agent has no memory of the parent conversation."
			},
			"model": {
				"type": "string",
				"description": "Optional model override (e.g. use a faster model for simple sub-tasks). Defaults to the parent model."
			},
			"provider": {
				"type": "string",
				"description": "Optional provider to use: 'anthropic' (default) or 'ollama' for local inference. Falls back to default if unknown."
			},
			"system_prompt": {
				"type": "string",
				"description": "Optional extra system-prompt context injected for this sub-agent only."
			}
		},
		"required": ["task"]
	}`)
}

func (t *SubAgentTool) Execute(ctx context.Context, input json.RawMessage) tools.ToolResult {
	var params struct {
		Task         string `json:"task"`
		Model        string `json:"model"`
		Provider     string `json:"provider"`
		SystemPrompt string `json:"system_prompt"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return tools.ToolResult{Error: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}
	if params.Task == "" {
		return tools.ToolResult{Error: "task is required", IsError: true}
	}

	// Resolve provider first so we can pick the right default model.
	provider := t.providers.Get(params.Provider)
	providerName := provider.Name()

	model := params.Model
	if model == "" {
		if providerName == "ollama" && t.cfg.OllamaModel != "" {
			model = t.cfg.OllamaModel
		} else {
			model = t.cfg.Model
		}
	}

	// For Ollama, validate that the model actually exists before spawning.
	if providerName == "ollama" {
		ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
		available, err := provider.ListModels(ctx2)
		cancel()
		if err == nil {
			var names []string
			found := false
			for _, m := range available {
				names = append(names, m.ID)
				if m.ID == model {
					found = true
				}
			}
			if !found {
				return tools.ToolResult{
					Error:   fmt.Sprintf("ollama model %q not found. Available models: %s", model, strings.Join(names, ", ")),
					IsError: true,
				}
			}
		}
	}

	id := fmt.Sprintf("agent-%d", t.counter.Add(1))
	t.send(display.SubAgentStartMsg{
		ID:       id,
		Task:     params.Task,
		Model:    model,
		Provider: providerName,
	})

	result, err := t.runTask(ctx, id, params.Task, model, params.SystemPrompt, provider)
	if err != nil {
		t.send(display.SubAgentDoneMsg{ID: id, IsError: true})
		return tools.ToolResult{
			Error:   fmt.Sprintf("[%s] failed: %v", id, err),
			IsError: true,
		}
	}

	t.send(display.SubAgentDoneMsg{ID: id, IsError: false})
	return tools.ToolResult{
		Output: fmt.Sprintf("[%s via %s]\n%s", id, providerName, result),
	}
}

// runTask runs the full agent loop for a sub-agent using the given provider.
func (t *SubAgentTool) runTask(ctx context.Context, id, task, model, extraSystem string, provider api.Provider) (string, error) {
	systemText := t.cfg.SystemPrompt
	if extraSystem != "" {
		systemText += "\n\n" + extraSystem
	}

	conv := NewConversation()
	conv.AddUser(task)

	toolDefs := t.registry.APIDefs()
	ephemeral := &api.CacheControl{Type: "ephemeral"}
	if len(toolDefs) > 0 {
		toolDefs[len(toolDefs)-1].CacheControl = ephemeral
	}

	tabChan := newTabMsgChan(id, t.msgChan)
	defer close(tabChan)

	const maxTurns = 30
	for turn := 0; turn < maxTurns; turn++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		req := &api.Request{
			Model:     model,
			MaxTokens: t.cfg.MaxTokens,
			System: []api.SystemBlock{{
				Type:         "text",
				Text:         systemText,
				CacheControl: ephemeral,
			}},
			Messages: conv.Messages,
			Tools:    toolDefs,
		}

		collector := newBlockCollector(tabChan)
		if err := provider.StreamMessage(ctx, req, collector); err != nil {
			return "", fmt.Errorf("turn %d stream error: %w", turn, err)
		}

		tabChan <- display.StreamDoneMsg{}

		blocks := collector.buildAssistantBlocks()
		conv.AddAssistant(blocks)

		toolCalls := collector.toolCalls()
		if len(toolCalls) == 0 {
			var sb string
			for _, b := range blocks {
				if b.Type == "text" {
					sb += b.Text
				}
			}
			return sb, nil
		}

		for _, tc := range toolCalls {
			t.send(display.SubAgentStepMsg{ID: id, ToolName: tc.Name})
			t.msgChan <- display.TabToolStartMsg{TabID: id, Name: tc.Name, ID: tc.ID}
		}

		results := t.executor.ExecuteAll(ctx, toolCalls)

		for _, r := range results {
			t.msgChan <- display.TabToolResultMsg{
				TabID:  id,
				Name:   r.Call.Name,
				ID:     r.Call.ID,
				Result: r.Result,
			}
		}

		var toolResultBlocks []api.ContentBlock
		for _, r := range results {
			content := r.Result.Output
			if r.Result.Error != "" {
				content = r.Result.Error
			}
			toolResultBlocks = append(toolResultBlocks, api.ContentBlock{
				Type:      "tool_result",
				ToolUseID: r.Call.ID,
				Content:   content,
				IsError:   r.Result.IsError,
			})
		}
		conv.AddToolResults(toolResultBlocks)
	}

	return "", fmt.Errorf("reached maximum turn limit (%d) without producing a final answer", maxTurns)
}

func (t *SubAgentTool) send(msg tea.Msg) {
	if t.msgChan == nil {
		return
	}
	select {
	case t.msgChan <- msg:
	default:
		// Channel full — drop the message rather than blocking the agent goroutine.
	}
}
