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
	"github.com/jonpo/cclient3/internal/skills"
	"github.com/jonpo/cclient3/internal/tools"
)

type Agent struct {
	providers    *api.ProviderRegistry
	config       *config.Config
	conversation *Conversation
	registry     *tools.Registry
	executor     *tools.Executor
	hooks        *HookRegistry
	skillsMgr    *skills.Manager
	msgChan      chan tea.Msg
	cwd          string
	sessions     *tools.SessionManager
	confirmChan  chan bool
}

func NewAgent(cfg *config.Config, providers *api.ProviderRegistry, msgChan chan tea.Msg) *Agent {
	sessions := tools.NewSessionManager()
	sessions.Prewarm("default", "scratch")

	// Child registry: all base tools, but NO sub_agent.
	childRegistry := tools.NewRegistry()
	childRegistry.Register(tools.NewBashTool(cfg.BashTimeout, sessions))
	childRegistry.Register(tools.NewFileReadTool())
	childRegistry.Register(tools.NewFileWriteTool())
	childRegistry.Register(tools.NewFileEditTool())
	childRegistry.Register(tools.NewGlobTool())
	childRegistry.Register(tools.NewGrepTool())
	childRegistry.Register(tools.NewWebFetchTool())

	// Parent registry: all base tools + sub_agent.
	registry := tools.NewRegistry()
	registry.Register(tools.NewBashTool(cfg.BashTimeout, sessions))
	registry.Register(tools.NewFileReadTool())
	registry.Register(tools.NewFileWriteTool())
	registry.Register(tools.NewFileEditTool())
	registry.Register(tools.NewGlobTool())
	registry.Register(tools.NewGrepTool())
	registry.Register(tools.NewWebFetchTool())
	registry.Register(NewSubAgentTool(providers, cfg, childRegistry, msgChan))

	hooks := NewHookRegistry()
	hooks.RegisterPreToolUse("bash", DefaultBashSafetyHook())

	cwd, _ := os.Getwd()

	return &Agent{
		providers:    providers,
		config:       cfg,
		conversation: NewConversation(),
		registry:     registry,
		executor:     tools.NewExecutor(registry, cfg.MaxToolConcurrency),
		hooks:        hooks,
		skillsMgr:    skills.NewManager(),
		msgChan:      msgChan,
		cwd:          cwd,
		sessions:     sessions,
		confirmChan:  make(chan bool, 1),
	}
}

func (a *Agent) Shutdown() { a.sessions.KillAll() }

func (a *Agent) ConfirmChan() chan bool { return a.confirmChan }

func (a *Agent) askConfirm(prompt, command string) bool {
	a.send(display.ConfirmRequestMsg{Prompt: prompt, Command: command})
	return <-a.confirmChan
}

func (a *Agent) Client() *api.ProviderRegistry { return a.providers }
func (a *Agent) Conversation() *Conversation   { return a.conversation }
func (a *Agent) Skills() *skills.Manager       { return a.skillsMgr }
func (a *Agent) Config() *config.Config        { return a.config }
func (a *Agent) Sessions() *tools.SessionManager { return a.sessions }

// Providers returns the provider registry for external callers (e.g. commands).
func (a *Agent) Providers() *api.ProviderRegistry { return a.providers }

func (a *Agent) Run(ctx context.Context, userInput string) error {
	a.conversation.AddUser(userInput)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		req := a.buildRequest()
		collector := newBlockCollector(a.msgChan)
		err := a.providers.Default().StreamMessage(ctx, req, collector)
		if err != nil {
			a.send(display.ErrorMsg{Err: err})
			return err
		}

		a.send(display.TokenUpdateMsg{
			InputTokens:              collector.usage.InputTokens,
			OutputTokens:             collector.usage.OutputTokens,
			CacheCreationInputTokens: collector.usage.CacheCreationInputTokens,
			CacheReadInputTokens:     collector.usage.CacheReadInputTokens,
		})

		assistantBlocks := collector.buildAssistantBlocks()
		a.conversation.AddAssistant(assistantBlocks)

		toolCalls := collector.toolCalls()
		if len(toolCalls) == 0 {
			a.send(display.StreamDoneMsg{})
			return nil
		}

		a.send(display.StreamDoneMsg{})

		var approved []tools.ToolCall
		var denied []tools.ToolCallResult

		for _, tc := range toolCalls {
			reason := a.hooks.CheckPreToolUse(tc)
			if reason == "" {
				approved = append(approved, tc)
				a.send(display.ToolStartMsg{ID: tc.ID, Name: tc.Name})
				continue
			}
			if len(reason) > 8 && reason[:8] == "CONFIRM:" {
				prompt := reason[8:]
				if a.askConfirm(prompt, tc.Name) {
					approved = append(approved, tc)
					a.send(display.ToolStartMsg{ID: tc.ID, Name: tc.Name})
					continue
				}
				reason = "Denied by user"
			}
			denied = append(denied, tools.ToolCallResult{
				Call:   tc,
				Result: tools.ToolResult{Error: reason, IsError: true},
			})
			a.send(display.ToolResultMsg{Result: tools.ToolCallResult{
				Call:   tc,
				Result: tools.ToolResult{Error: reason, IsError: true},
			}})
		}

		results := a.executor.ExecuteAll(ctx, approved)
		for _, r := range results {
			a.send(display.ToolResultMsg{Result: r})
		}

		allResults := append(denied, results...)
		a.conversation.AddToolResults(makeToolResultBlocks(allResults))
	}
}

func (a *Agent) buildRequest() *api.Request {
	ephemeral := &api.CacheControl{Type: "ephemeral"}

	contextBudget := 190000 - a.config.MaxTokens
	if contextBudget < 10000 {
		contextBudget = 10000
	}
	if dropped := a.conversation.TrimForWindow(contextBudget); dropped > 0 {
		a.send(display.ContextTrimMsg{Dropped: dropped})
	}

	systemText := a.config.SystemPrompt
	if a.cwd != "" {
		systemText += fmt.Sprintf("\n\nCurrent working directory: %s", a.cwd)
	}
	if skillContent := a.skillsMgr.ActiveContent(); skillContent != "" {
		systemText += "\n\n" + skillContent
	}
	system := []api.SystemBlock{{
		Type:         "text",
		Text:         systemText,
		CacheControl: ephemeral,
	}}

	toolDefs := a.registry.APIDefs()
	if len(toolDefs) > 0 {
		toolDefs[len(toolDefs)-1].CacheControl = ephemeral
	}

	messages := copyMessagesWithCache(a.conversation.Messages)

	return &api.Request{
		Model:       a.config.Model,
		MaxTokens:   a.config.MaxTokens,
		Temperature: a.config.Temperature,
		System:      system,
		Messages:    messages,
		Tools:       toolDefs,
	}
}

func copyMessagesWithCache(orig []api.Message) []api.Message {
	messages := make([]api.Message, len(orig))
	for i, msg := range orig {
		messages[i] = api.Message{Role: msg.Role}
		switch content := msg.Content.(type) {
		case string:
			messages[i].Content = content
		case []api.ContentBlock:
			blocks := make([]api.ContentBlock, len(content))
			for j, b := range content {
				blocks[j] = b
				blocks[j].CacheControl = nil
			}
			messages[i].Content = blocks
		default:
			messages[i].Content = msg.Content
		}
	}

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

func (a *Agent) send(msg tea.Msg) { a.msgChan <- msg }

type blockCollector struct {
	mu      sync.Mutex
	blocks  []collectedBlock
	usage   api.Usage
	msgChan chan tea.Msg
}

type collectedBlock struct {
	typ     string
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
	if c.msgChan != nil {
		c.msgChan <- display.TextDeltaMsg{Text: text}
	}
}

func (c *blockCollector) OnThinkingDelta(index int, thinking string) {
	c.mu.Lock()
	if index < len(c.blocks) {
		c.blocks[index].text += thinking
	}
	c.mu.Unlock()
	if c.msgChan != nil {
		c.msgChan <- display.ThinkingDeltaMsg{Thinking: thinking}
	}
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
	if c.msgChan != nil {
		c.msgChan <- display.ErrorMsg{Err: err}
	}
}

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
			blocks = append(blocks, api.ContentBlock{Type: "text", Text: b.text})
		case "thinking":
			blocks = append(blocks, api.ContentBlock{Type: "thinking", Thinking: b.text})
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

func (a *Agent) RunSingleTurn(ctx context.Context, prompt string) (string, error) {
	a.conversation.AddUser(prompt)

	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		req := a.buildRequest()
		resp, err := a.providers.Default().SendMessage(ctx, req)
		if err != nil {
			return "", err
		}

		var contentBlocks []api.ContentBlock
		var text string
		var toolCalls []tools.ToolCall
		for _, block := range resp.Content {
			switch block.Type {
			case "text":
				contentBlocks = append(contentBlocks, api.ContentBlock{Type: "text", Text: block.Text})
				text += block.Text
			case "thinking":
				contentBlocks = append(contentBlocks, api.ContentBlock{Type: "thinking", Thinking: block.Thinking})
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
	}
}

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

func (a *Agent) SetModel(model string) {
	a.config.Model = model
	a.send(display.SetModelMsg{Name: model})
}

func (a *Agent) ListModels(ctx context.Context) ([]string, error) {
	models, err := a.providers.Default().ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	var names []string
	for _, m := range models {
		names = append(names, m.ID)
	}
	return names, nil
}
