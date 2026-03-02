package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	cwd          string          // cached at construction; bash tools run in isolated subshells
	sessions     *tools.SessionManager
}

func NewAgent(cfg *config.Config, msgChan chan tea.Msg) *Agent {
	client := api.NewClient(cfg.APIKey, cfg.APIEndpoint)
	registry := tools.NewRegistry()
	sessions := tools.NewSessionManager()
	sessions.Prewarm("default", "scratch")

	// Register all tools
	registry.Register(tools.NewBashTool(cfg.BashTimeout, sessions))
	registry.Register(tools.NewFileReadTool())
	registry.Register(tools.NewFileWriteTool())
	registry.Register(tools.NewFileEditTool())
	registry.Register(tools.NewGlobTool())
	registry.Register(tools.NewGrepTool())

	// Set up hooks
	hooks := NewHookRegistry()
	hooks.RegisterPreToolUse("bash", DefaultBashSafetyHook())

	cwd, _ := os.Getwd()

	return &Agent{
		client:       client,
		config:       cfg,
		conversation: NewConversation(),
		registry:     registry,
		executor:     tools.NewExecutor(registry, cfg.MaxToolConcurrency),
		hooks:        hooks,
		msgChan:      msgChan,
		cwd:          cwd,
		sessions:     sessions,
	}
}

// Shutdown cleans up agent resources (e.g. terminates bash sessions).
func (a *Agent) Shutdown() {
	a.sessions.KillAll()
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

		// Build request with prompt caching
		req := a.buildRequest()

		// Stream response
		collector := newBlockCollector(a.msgChan)
		err := a.client.StreamMessage(ctx, req, collector)
		if err != nil {
			a.send(display.ErrorMsg{Err: err})
			return err
		}

		// Send token update (includes cache stats)
		a.send(display.TokenUpdateMsg{
			InputTokens:              collector.usage.InputTokens,
			OutputTokens:             collector.usage.OutputTokens,
			CacheCreationInputTokens: collector.usage.CacheCreationInputTokens,
			CacheReadInputTokens:     collector.usage.CacheReadInputTokens,
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
		toolResultBlocks := makeToolResultBlocks(allResults)
		a.conversation.AddToolResults(toolResultBlocks)
		// Loop back for next response
	}
}

// buildRequest constructs an API request with prompt caching markers.
// The Anthropic API allows at most 4 cache_control breakpoints per request.
// We use them as: (1) system prompt, (2) last tool definition,
// (3) the penultimate user message — giving 3 breakpoints total,
// which leaves headroom and maximizes cache reuse across turns.
func (a *Agent) buildRequest() *api.Request {
	ephemeral := &api.CacheControl{Type: "ephemeral"}

	// Breakpoint 1: system prompt (with cwd context)
	systemText := a.config.SystemPrompt
	if a.cwd != "" {
		systemText += fmt.Sprintf("\n\nCurrent working directory: %s", a.cwd)
	}
	system := []api.SystemBlock{{
		Type:         "text",
		Text:         systemText,
		CacheControl: ephemeral,
	}}

	// Breakpoint 2: last tool definition
	tools := a.registry.APIDefs()
	if len(tools) > 0 {
		tools[len(tools)-1].CacheControl = ephemeral
	}

	// Build a clean copy of messages with exactly one cache breakpoint.
	// We strip any stale cache_control from all messages, then mark
	// the penultimate user message (breakpoint 3).
	messages := copyMessagesWithCache(a.conversation.Messages)

	return &api.Request{
		Model:       a.config.Model,
		MaxTokens:   a.config.MaxTokens,
		Temperature: a.config.Temperature,
		System:      system,
		Messages:    messages,
		Tools:       tools,
	}
}

// copyMessagesWithCache returns a deep copy of messages with all existing
// cache_control markers stripped, then adds one breakpoint on the last
// content block of the second-to-last user message.
func copyMessagesWithCache(orig []api.Message) []api.Message {
	messages := make([]api.Message, len(orig))

	// Deep copy each message, stripping any cache_control
	for i, msg := range orig {
		messages[i] = api.Message{Role: msg.Role}
		switch content := msg.Content.(type) {
		case string:
			messages[i].Content = content
		case []api.ContentBlock:
			blocks := make([]api.ContentBlock, len(content))
			for j, b := range content {
				blocks[j] = b
				blocks[j].CacheControl = nil // strip stale markers
			}
			messages[i].Content = blocks
		default:
			messages[i].Content = msg.Content
		}
	}

	// Find the second-to-last user message and mark its last block
	userCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			userCount++
			if userCount == 2 {
				markLastBlock(&messages[i])
				return messages
			}
		}
	}

	return messages
}

// markLastBlock adds cache_control to the last content block of a message.
func markLastBlock(msg *api.Message) {
	ephemeral := &api.CacheControl{Type: "ephemeral"}
	switch content := msg.Content.(type) {
	case []api.ContentBlock:
		if len(content) > 0 {
			content[len(content)-1].CacheControl = ephemeral
		}
	case string:
		msg.Content = []api.ContentBlock{{
			Type:         "text",
			Text:         content,
			CacheControl: ephemeral,
		}}
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
	if index < len(c.blocks) {
		c.blocks[index].text += text
	}
	c.mu.Unlock()
	c.msgChan <- display.TextDeltaMsg{Text: text}
}

func (c *blockCollector) OnThinkingDelta(index int, thinking string) {
	c.mu.Lock()
	if index < len(c.blocks) {
		c.blocks[index].text += thinking
	}
	c.mu.Unlock()
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

// safeRawJSON returns a valid json.RawMessage from a string buffer.
// Falls back to "{}" if the buffer is empty or invalid JSON.
func safeRawJSON(buf string) json.RawMessage {
	if buf == "" {
		return json.RawMessage("{}")
	}
	raw := json.RawMessage(buf)
	if !json.Valid(raw) {
		return json.RawMessage("{}")
	}
	return raw
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
				Input: safeRawJSON(b.jsonBuf),
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
				Input: safeRawJSON(b.jsonBuf),
			})
		}
	}
	return calls
}

// RunSingleTurn runs a single non-interactive prompt, executing tool calls
// in a loop until the model produces a final text response.
func (a *Agent) RunSingleTurn(ctx context.Context, prompt string) (string, error) {
	a.conversation.AddUser(prompt)

	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		req := a.buildRequest()
		resp, err := a.client.SendMessage(ctx, req)
		if err != nil {
			return "", err
		}

		// Convert ResponseBlocks to ContentBlocks for conversation history
		var contentBlocks []api.ContentBlock
		var text string
		var toolCalls []tools.ToolCall
		for _, block := range resp.Content {
			switch block.Type {
			case "text":
				contentBlocks = append(contentBlocks, api.ContentBlock{
					Type: "text",
					Text: block.Text,
				})
				text += block.Text
			case "thinking":
				contentBlocks = append(contentBlocks, api.ContentBlock{
					Type:     "thinking",
					Thinking: block.Thinking,
				})
			case "tool_use":
				input := safeRawJSON(string(block.Input))
				contentBlocks = append(contentBlocks, api.ContentBlock{
					Type:  "tool_use",
					ID:    block.ID,
					Name:  block.Name,
					Input: input,
				})
				toolCalls = append(toolCalls, tools.ToolCall{
					ID:    block.ID,
					Name:  block.Name,
					Input: input,
				})
			}
		}
		a.conversation.AddAssistant(contentBlocks)

		if len(toolCalls) == 0 {
			return text, nil
		}

		// Run hooks before executing tools (same safety checks as Run)
		var approved []tools.ToolCall
		var denied []tools.ToolCallResult
		for _, tc := range toolCalls {
			if reason := a.hooks.CheckPreToolUse(tc); reason != "" {
				denied = append(denied, tools.ToolCallResult{
					Call:   tc,
					Result: tools.ToolResult{Error: reason, IsError: true},
				})
			} else {
				approved = append(approved, tc)
			}
		}

		results := a.executor.ExecuteAll(ctx, approved)
		a.conversation.AddToolResults(makeToolResultBlocks(append(denied, results...)))
		// Loop back for next response
	}
}

// makeToolResultBlocks converts tool call results into API content blocks.
func makeToolResultBlocks(results []tools.ToolCallResult) []api.ContentBlock {
	blocks := make([]api.ContentBlock, len(results))
	for i, r := range results {
		content := r.Result.Output
		if r.Result.Error != "" {
			content = r.Result.Error
		}
		blocks[i] = api.ContentBlock{
			Type:      "tool_result",
			ToolUseID: r.Call.ID,
			Content:   content,
			IsError:   r.Result.IsError,
		}
	}
	return blocks
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
