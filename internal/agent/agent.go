package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonpo/cclient3/internal/api"
	"github.com/jonpo/cclient3/internal/config"
	"github.com/jonpo/cclient3/internal/display"
	"github.com/jonpo/cclient3/internal/tools"
)

type Agent struct {
	client       *api.Client
	config       *config.Config
	conversation *Conversation
	registry     *tools.Registry
	executor     *tools.Executor
	hooks        *HookRegistry
	msgChan      chan tea.Msg
}

func NewAgent(cfg *config.Config, msgChan chan tea.Msg) *Agent {
	client := api.NewClient(cfg.APIKey, cfg.APIEndpoint)
	registry := tools.NewRegistry()

	// Register all tools
	registry.Register(tools.NewBashTool(cfg.BashTimeout))
	registry.Register(tools.NewFileReadTool())
	registry.Register(tools.NewFileWriteTool())
	registry.Register(tools.NewFileEditTool())
	registry.Register(tools.NewGlobTool())
	registry.Register(tools.NewGrepTool())

	// Set up hooks
	hooks := NewHookRegistry()
	hooks.RegisterPreToolUse("bash", DefaultBashSafetyHook())

	return &Agent{
		client:       client,
		config:       cfg,
		conversation: NewConversation(),
		registry:     registry,
		executor:     tools.NewExecutor(registry, cfg.MaxToolConcurrency),
		hooks:        hooks,
		msgChan:      msgChan,
	}
}

func (a *Agent) Client() *api.Client {
	return a.client
}

func (a *Agent) Conversation() *Conversation {
	return a.conversation
}

func (a *Agent) Config() *config.Config {
	return a.config
}

// Run processes a user message through the agent loop.
// Streams response, collects tool calls, executes in parallel, loops.
func (a *Agent) Run(ctx context.Context, userInput string) error {
	a.conversation.AddUser(userInput)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Build request
		req := &api.Request{
			Model:       a.config.Model,
			MaxTokens:   a.config.MaxTokens,
			Temperature: a.config.Temperature,
			System:      a.config.SystemPrompt,
			Messages:    a.conversation.Messages,
			Tools:       a.registry.APIDefs(),
		}

		// Stream response
		collector := newBlockCollector(a.msgChan)
		err := a.client.StreamMessage(ctx, req, collector)
		if err != nil {
			a.send(display.ErrorMsg{Err: err})
			return err
		}

		// Send token update
		a.send(display.TokenUpdateMsg{
			InputTokens:  collector.usage.InputTokens,
			OutputTokens: collector.usage.OutputTokens,
		})

		// Add assistant message to conversation
		assistantBlocks := collector.buildAssistantBlocks()
		a.conversation.AddAssistant(assistantBlocks)

		// Collect tool calls
		toolCalls := collector.toolCalls()
		if len(toolCalls) == 0 {
			// No tool calls — turn complete
			a.send(display.StreamDoneMsg{})
			return nil
		}

		// Signal stream done before executing tools (flushes text)
		a.send(display.StreamDoneMsg{})

		// Run hooks + execute tools in parallel
		var approved []tools.ToolCall
		var denied []tools.ToolCallResult

		for _, tc := range toolCalls {
			if reason := a.hooks.CheckPreToolUse(tc); reason != "" {
				denied = append(denied, tools.ToolCallResult{
					Call: tc,
					Result: tools.ToolResult{
						Error:   reason,
						IsError: true,
					},
				})
				a.send(display.ToolResultMsg{Result: tools.ToolCallResult{
					Call:   tc,
					Result: tools.ToolResult{Error: reason, IsError: true},
				}})
			} else {
				approved = append(approved, tc)
				a.send(display.ToolStartMsg{ID: tc.ID, Name: tc.Name})
			}
		}

		// Execute approved tools in parallel
		results := a.executor.ExecuteAll(ctx, approved)

		// Send results to display
		for _, r := range results {
			a.send(display.ToolResultMsg{Result: r})
		}

		// Build tool_result blocks for the API
		allResults := append(denied, results...)
		var toolResultBlocks []api.ContentBlock
		for _, r := range allResults {
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

		a.conversation.AddToolResults(toolResultBlocks)
		// Loop back for next response
	}
}

func (a *Agent) send(msg tea.Msg) {
	a.msgChan <- msg
}

// blockCollector implements StreamCallback and collects content blocks.
type blockCollector struct {
	mu     sync.Mutex
	blocks []collectedBlock
	usage  api.Usage
	msgChan chan tea.Msg
}

type collectedBlock struct {
	typ     string // "text", "thinking", "tool_use"
	text    string
	id      string
	name    string
	jsonBuf string
}

func newBlockCollector(msgChan chan tea.Msg) *blockCollector {
	return &blockCollector{msgChan: msgChan}
}

func (c *blockCollector) OnMessageStart(msg api.Response) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.usage = msg.Usage
}

func (c *blockCollector) OnContentBlockStart(index int, block api.ResponseBlock) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.blocks) <= index {
		c.blocks = append(c.blocks, collectedBlock{})
	}
	c.blocks[index].typ = block.Type
	if block.Type == "tool_use" {
		c.blocks[index].id = block.ID
		c.blocks[index].name = block.Name
	}
}

func (c *blockCollector) OnTextDelta(index int, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < len(c.blocks) {
		c.blocks[index].text += text
	}
	c.msgChan <- display.TextDeltaMsg{Text: text}
}

func (c *blockCollector) OnThinkingDelta(index int, thinking string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < len(c.blocks) {
		c.blocks[index].text += thinking
	}
	c.msgChan <- display.ThinkingDeltaMsg{Thinking: thinking}
}

func (c *blockCollector) OnInputJSONDelta(index int, partialJSON string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < len(c.blocks) {
		c.blocks[index].jsonBuf += partialJSON
	}
}

func (c *blockCollector) OnContentBlockStop(index int) {}

func (c *blockCollector) OnMessageDelta(delta api.MessageDelta, usage *api.Usage) {
	if usage != nil {
		c.mu.Lock()
		c.usage.OutputTokens = usage.OutputTokens
		c.mu.Unlock()
	}
}

func (c *blockCollector) OnMessageStop() {}

func (c *blockCollector) OnError(err error) {
	c.msgChan <- display.ErrorMsg{Err: err}
}

func (c *blockCollector) buildAssistantBlocks() []api.ContentBlock {
	c.mu.Lock()
	defer c.mu.Unlock()

	var blocks []api.ContentBlock
	for _, b := range c.blocks {
		switch b.typ {
		case "text":
			blocks = append(blocks, api.ContentBlock{
				Type: "text",
				Text: b.text,
			})
		case "thinking":
			blocks = append(blocks, api.ContentBlock{
				Type:     "thinking",
				Thinking: b.text,
			})
		case "tool_use":
			blocks = append(blocks, api.ContentBlock{
				Type:  "tool_use",
				ID:    b.id,
				Name:  b.name,
				Input: json.RawMessage(b.jsonBuf),
			})
		}
	}
	return blocks
}

func (c *blockCollector) toolCalls() []tools.ToolCall {
	c.mu.Lock()
	defer c.mu.Unlock()

	var calls []tools.ToolCall
	for _, b := range c.blocks {
		if b.typ == "tool_use" {
			calls = append(calls, tools.ToolCall{
				ID:    b.id,
				Name:  b.name,
				Input: json.RawMessage(b.jsonBuf),
			})
		}
	}
	return calls
}

// RunSingleTurn runs a single non-interactive prompt and prints the response.
func (a *Agent) RunSingleTurn(ctx context.Context, prompt string) (string, error) {
	a.conversation.AddUser(prompt)

	req := &api.Request{
		Model:       a.config.Model,
		MaxTokens:   a.config.MaxTokens,
		Temperature: a.config.Temperature,
		System:      a.config.SystemPrompt,
		Messages:    a.conversation.Messages,
		Tools:       a.registry.APIDefs(),
	}

	resp, err := a.client.SendMessage(ctx, req)
	if err != nil {
		return "", err
	}

	var text string
	for _, block := range resp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return text, nil
}

// SetModel changes the active model.
func (a *Agent) SetModel(model string) {
	a.config.Model = model
	a.send(display.SetModelMsg{Name: model})
}

// ListModels fetches available models from the API.
func (a *Agent) ListModels(ctx context.Context) ([]string, error) {
	models, err := a.client.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	var names []string
	for _, m := range models {
		names = append(names, m.ID)
	}
	return names, nil
}
