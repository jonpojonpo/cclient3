package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonpo/cclient3/internal/api"
	"github.com/jonpo/cclient3/internal/config"
	"github.com/jonpo/cclient3/internal/display"
	"github.com/jonpo/cclient3/internal/tools"
)

// SubAgentTool is a tool that spawns a fully autonomous child agent.
// The child has its own conversation context and full tool access (except
// sub_agent itself, to prevent unbounded recursion).
// Multiple sub_agent calls in one response execute in parallel via the
// normal parallel tool executor.
type SubAgentTool struct {
	client   *api.Client
	cfg      *config.Config
	registry *tools.Registry // child registry — no sub_agent registered
	executor *tools.Executor
	msgChan  chan tea.Msg
	counter  atomic.Int64
}

// NewSubAgentTool builds a SubAgentTool using a pre-built child registry.
func NewSubAgentTool(
	client *api.Client,
	cfg *config.Config,
	childRegistry *tools.Registry,
	msgChan chan tea.Msg,
) *SubAgentTool {
	return &SubAgentTool{
		client:   client,
		cfg:      cfg,
		registry: childRegistry,
		executor: tools.NewExecutor(childRegistry, 6),
		msgChan:  msgChan,
	}
}

func (t *SubAgentTool) Name() string { return "sub_agent" }

func (t *SubAgentTool) Description() string {
	return `Spawn a fully autonomous sub-agent to complete a task independently.

The sub-agent has its own conversation context and access to all tools
(bash, file_read, file_write, file_edit, glob, grep, web_fetch).
It runs its own tool-use loop until it produces a final answer.

Multiple sub_agent calls in a single response run IN PARALLEL — use this
to decompose complex work into concurrent workstreams (e.g. one agent
writes tests while another writes implementation).

The sub-agent's final answer is returned as a tool result string.`
}

func (t *SubAgentTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {
				"type": "string",
				"description": "Full self-contained task description. The sub-agent has no memory of the parent conversation, so include all necessary context."
			},
			"model": {
				"type": "string",
				"description": "Optional model override (e.g. use a faster/cheaper model for simple sub-tasks). Defaults to the parent model."
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
		SystemPrompt string `json:"system_prompt"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return tools.ToolResult{Error: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}
	if params.Task == "" {
		return tools.ToolResult{Error: "task is required", IsError: true}
	}

	model := params.Model
	if model == "" {
		model = t.cfg.Model
	}

	id := fmt.Sprintf("agent-%d", t.counter.Add(1))
	t.send(display.SubAgentStartMsg{ID: id, Task: params.Task, Model: model})

	result, err := t.runTask(ctx, id, params.Task, model, params.SystemPrompt)
	if err != nil {
		t.send(display.SubAgentDoneMsg{ID: id, IsError: true})
		return tools.ToolResult{
			Error:   fmt.Sprintf("[%s] failed: %v", id, err),
			IsError: true,
		}
	}

	t.send(display.SubAgentDoneMsg{ID: id, IsError: false})
	return tools.ToolResult{
		Output: fmt.Sprintf("[%s]\n%s", id, result),
	}
}

// runTask runs the full agent loop for a sub-agent and returns its final answer.
func (t *SubAgentTool) runTask(ctx context.Context, id, task, model, extraSystem string) (string, error) {
	systemText := t.cfg.SystemPrompt
	if extraSystem != "" {
		systemText += "\n\n" + extraSystem
	}

	conv := NewConversation()
	conv.AddUser(task)

	toolDefs := t.registry.APIDefs()
	// Mark last tool def with cache_control for efficiency on repeated calls.
	ephemeral := &api.CacheControl{Type: "ephemeral"}
	if len(toolDefs) > 0 {
		toolDefs[len(toolDefs)-1].CacheControl = ephemeral
	}

	const maxTurns = 30 // safety limit — prevent runaway sub-agents
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

		// Collect streaming response — no forwarding to display (silent collector).
		collector := newBlockCollector(nil)
		if err := t.client.StreamMessage(ctx, req, collector); err != nil {
			return "", fmt.Errorf("turn %d stream error: %w", turn, err)
		}

		blocks := collector.buildAssistantBlocks()
		conv.AddAssistant(blocks)

		toolCalls := collector.toolCalls()
		if len(toolCalls) == 0 {
			// No more tool calls — extract final text answer.
			var sb string
			for _, b := range blocks {
				if b.Type == "text" {
					sb += b.Text
				}
			}
			return sb, nil
		}

		// Notify display of each tool the sub-agent is calling.
		for _, tc := range toolCalls {
			t.send(display.SubAgentStepMsg{ID: id, ToolName: tc.Name})
		}

		// Execute tools in parallel.
		results := t.executor.ExecuteAll(ctx, toolCalls)

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
	if t.msgChan != nil {
		t.msgChan <- msg
	}
}
