package ensemble

import (
	"context"
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonpo/cclient3/internal/api"
	"github.com/jonpo/cclient3/internal/config"
	"github.com/jonpo/cclient3/internal/display"
)

// AgentSpec defines a single agent in the ensemble.
type AgentSpec struct {
	Name        string `json:"name" yaml:"name"`
	Personality string `json:"personality" yaml:"personality"`
	Model       string `json:"model" yaml:"model"`
	Provider    string `json:"provider" yaml:"provider"`
	Color       string `json:"color" yaml:"color"`
}

// TranscriptEntry is a single message in the shared group conversation.
type TranscriptEntry struct {
	Speaker string
	Text    string
}

// Default agent colors for auto-assignment.
var agentColors = []string{
	"#FF6B6B", // red
	"#4ECDC4", // teal
	"#FFE66D", // yellow
	"#A8E6CF", // green
	"#DDA0DD", // plum
	"#87CEEB", // sky blue
	"#FFA07A", // salmon
	"#98FB98", // pale green
}

// Ensemble orchestrates a multi-agent group chat.
type Ensemble struct {
	agents     []AgentSpec
	transcript []TranscriptEntry
	mu         sync.Mutex

	tabID     string
	msgChan   chan tea.Msg
	userChan  chan string
	providers *api.ProviderRegistry
	cfg       *config.Config

	// Round configuration
	microRounds    int // sequential passes per user message (default 2)
	responseTokens int // max output tokens per agent response (default 300)
	thinkTokens    int // max output tokens for private scratchpad (default 150)
}

// New creates an Ensemble from agent specs and wires it up.
func New(
	agents []AgentSpec,
	providers *api.ProviderRegistry,
	cfg *config.Config,
	msgChan chan tea.Msg,
	userChan chan string,
	tabID string,
) *Ensemble {
	// Assign colors to agents that don't have one.
	for i := range agents {
		if agents[i].Color == "" {
			agents[i].Color = agentColors[i%len(agentColors)]
		}
		if agents[i].Provider == "" {
			agents[i].Provider = providers.DefaultName()
		}
		if agents[i].Model == "" {
			agents[i].Model = cfg.Model
		}
	}

	return &Ensemble{
		agents:         agents,
		tabID:          tabID,
		msgChan:        msgChan,
		userChan:       userChan,
		providers:      providers,
		cfg:            cfg,
		microRounds:    2,
		responseTokens: 300,
		thinkTokens:    150,
	}
}

// TabID returns the full tab ID used in messages.
func (e *Ensemble) TabID() string {
	return "ensemble-" + e.tabID
}

// AgentInfos returns display info for the TUI start message.
func (e *Ensemble) AgentInfos() []display.EnsembleAgentInfo {
	infos := make([]display.EnsembleAgentInfo, len(e.agents))
	for i, a := range e.agents {
		infos[i] = display.EnsembleAgentInfo{
			Name:        a.Name,
			Model:       a.Model,
			Provider:    a.Provider,
			Personality: a.Personality,
			Color:       a.Color,
		}
	}
	return infos
}

// Run starts the ensemble conversation loop.
func (e *Ensemble) Run(ctx context.Context, initialPrompt string) {
	fullTabID := e.TabID()

	e.addTranscript("You", initialPrompt)
	e.runRound(ctx, fullTabID)

	for {
		select {
		case <-ctx.Done():
			e.msgChan <- display.EnsembleDoneMsg{TabID: fullTabID}
			return

		case userMsg, ok := <-e.userChan:
			if !ok {
				e.msgChan <- display.EnsembleDoneMsg{TabID: fullTabID}
				return
			}

			if strings.TrimSpace(strings.ToLower(userMsg)) == "/done" {
				e.msgChan <- display.EnsembleDoneMsg{
					TabID:   fullTabID,
					Summary: e.buildSummary(),
				}
				return
			}

			e.addTranscript("You", userMsg)
			e.runRound(ctx, fullTabID)
		}
	}
}

// runRound executes N micro-rounds using a two-phase approach per micro-round:
//
//  1. Think phase (parallel): all agents silently read the transcript and write
//     private scratchpad notes — their "mental draft" while waiting their turn.
//
//  2. Speak phase (sequential): agents respond one at a time. Each agent sees
//     the transcript including whatever the previous agent just said, plus their
//     own private scratchpad. Responses are short (responseTokens budget).
func (e *Ensemble) runRound(ctx context.Context, fullTabID string) {
	for i := 0; i < e.microRounds; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		scratchpads := e.thinkPhase(ctx)
		e.speakPhase(ctx, fullTabID, scratchpads)
	}

	// Drain any user message that arrived during the round.
	select {
	case userMsg := <-e.userChan:
		e.addTranscript("You", userMsg)
		e.msgChan <- display.EnsembleUserMsg{TabID: fullTabID, Text: userMsg}
	default:
	}

	e.msgChan <- display.EnsembleRoundDoneMsg{TabID: fullTabID}
}

// thinkPhase runs all agents in parallel to generate private scratchpad notes
// from the current transcript state. The notes are never shown in the UI.
func (e *Ensemble) thinkPhase(ctx context.Context) map[string]string {
	var wg sync.WaitGroup
	var mu sync.Mutex
	scratchpads := make(map[string]string, len(e.agents))

	for _, agent := range e.agents {
		wg.Add(1)
		go func(a AgentSpec) {
			defer wg.Done()
			notes := e.generateScratchpad(ctx, a)
			mu.Lock()
			scratchpads[a.Name] = notes
			mu.Unlock()
		}(agent)
	}

	wg.Wait()
	return scratchpads
}

// speakPhase runs agents sequentially so each can build on what the previous said.
func (e *Ensemble) speakPhase(ctx context.Context, fullTabID string, scratchpads map[string]string) {
	for _, agent := range e.agents {
		select {
		case <-ctx.Done():
			return
		default:
		}
		e.runAgent(ctx, agent, fullTabID, scratchpads[agent.Name])
	}
}

// generateScratchpad silently calls an agent to produce private listening notes.
func (e *Ensemble) generateScratchpad(ctx context.Context, agent AgentSpec) string {
	provider := e.providers.Get(agent.Provider)

	systemPrompt := fmt.Sprintf(
		`You are "%s" in a group discussion.

%s

You are LISTENING. Write 3 brief private notes for yourself (one sentence each):
- What specific point do you most want to respond to?
- What unique angle or disagreement do you bring?
- What direction do you want to push the discussion?

These notes are private — be honest and specific.`,
		agent.Name, agent.Personality)

	req := &api.Request{
		Model:     agent.Model,
		MaxTokens: e.thinkTokens,
		System:    systemPrompt,
		Messages: []api.Message{
			{Role: "user", Content: e.buildTranscriptText() + "\n\nWrite your private notes:"},
		},
	}

	resp, err := provider.SendMessage(ctx, req)
	if err != nil {
		return ""
	}

	// Track token usage for cost accounting.
	e.msgChan <- display.TokenUpdateMsg{
		Model:        agent.Model,
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}

	var text string
	for _, block := range resp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return text
}

// runAgent sends the transcript (plus private scratchpad) to one agent and streams the response.
func (e *Ensemble) runAgent(ctx context.Context, agent AgentSpec, fullTabID, scratchpad string) {
	provider := e.providers.Get(agent.Provider)

	notesSection := ""
	if scratchpad != "" {
		notesSection = fmt.Sprintf("\n\nYour private notes (what you planned to say):\n%s", scratchpad)
	}

	systemPrompt := fmt.Sprintf(
		`You are "%s" in a group discussion with other AI agents and a human user.

%s%s

Rules:
- Be VERY concise: 2-4 sentences max (like a chat message, not an essay)
- Respond to what was most recently said — prioritise the last speaker
- Address agents by name when responding to their points
- Make ONE clear point per turn; don't try to summarise everything
- Disagree constructively when you see things differently
- Stay in character`,
		agent.Name, agent.Personality, notesSection)

	req := &api.Request{
		Model:     agent.Model,
		MaxTokens: e.responseTokens,
		System:    systemPrompt,
		Messages: []api.Message{
			{Role: "user", Content: e.buildTranscriptText() + "\n\n---\nContinue the discussion as " + agent.Name + ":"},
		},
	}

	e.msgChan <- display.EnsembleSpeakerMsg{
		TabID:   fullTabID,
		Speaker: agent.Name,
		Color:   agent.Color,
		Model:   agent.Model,
	}

	collector := &ensembleCollector{
		tabID:   fullTabID,
		speaker: agent.Name,
		color:   agent.Color,
		model:   agent.Model,
		msgChan: e.msgChan,
	}

	err := provider.StreamMessage(ctx, req, collector)
	if err != nil {
		e.msgChan <- display.EnsembleErrorMsg{
			TabID: fullTabID,
			Err:   fmt.Errorf("[%s] %w", agent.Name, err),
		}
		return
	}

	// Send token usage to cost tracker.
	if in, out := collector.tokenUsage(); in > 0 || out > 0 {
		e.msgChan <- display.TokenUpdateMsg{
			Model:        agent.Model,
			InputTokens:  in,
			OutputTokens: out,
		}
	}

	responseText := collector.text()
	if responseText != "" {
		e.addTranscript(agent.Name, responseText)
	}

	e.msgChan <- display.EnsembleTurnDoneMsg{
		TabID:   fullTabID,
		Speaker: agent.Name,
	}
}

func (e *Ensemble) addTranscript(speaker, text string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.transcript = append(e.transcript, TranscriptEntry{Speaker: speaker, Text: text})
}

func (e *Ensemble) buildTranscriptText() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var sb strings.Builder
	sb.WriteString("Group discussion transcript:\n\n")
	for _, entry := range e.transcript {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", entry.Speaker, entry.Text))
	}
	return sb.String()
}

func (e *Ensemble) buildSummary() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.transcript) == 0 {
		return "No discussion took place."
	}
	var sb strings.Builder
	sb.WriteString("Discussion participants: ")
	speakers := map[string]bool{}
	for _, entry := range e.transcript {
		speakers[entry.Speaker] = true
	}
	names := make([]string, 0, len(speakers))
	for name := range speakers {
		if name != "You" {
			names = append(names, name)
		}
	}
	sb.WriteString(strings.Join(names, ", "))
	sb.WriteString(fmt.Sprintf("\nTotal messages: %d", len(e.transcript)))
	return sb.String()
}

// ensembleCollector implements api.StreamCallback to collect and forward agent output.
type ensembleCollector struct {
	tabID   string
	speaker string
	color   string
	model   string
	msgChan chan tea.Msg

	mu      sync.Mutex
	content strings.Builder
	inTok   int
	outTok  int
}

func (c *ensembleCollector) OnMessageStart(msg api.Response) {}
func (c *ensembleCollector) OnContentBlockStart(index int, block api.ResponseBlock) {}

func (c *ensembleCollector) OnTextDelta(index int, text string) {
	c.mu.Lock()
	c.content.WriteString(text)
	c.mu.Unlock()

	c.msgChan <- display.EnsembleTextDeltaMsg{
		TabID:   c.tabID,
		Speaker: c.speaker,
		Text:    text,
		Color:   c.color,
	}
}

func (c *ensembleCollector) OnThinkingDelta(index int, thinking string) {}

func (c *ensembleCollector) OnInputJSONDelta(index int, partialJSON string) {}
func (c *ensembleCollector) OnContentBlockStop(index int)                   {}

func (c *ensembleCollector) OnMessageDelta(delta api.MessageDelta, usage *api.Usage) {
	if usage != nil {
		c.mu.Lock()
		c.inTok = usage.InputTokens
		c.outTok = usage.OutputTokens
		c.mu.Unlock()
	}
}

func (c *ensembleCollector) OnMessageStop() {}

func (c *ensembleCollector) OnError(err error) {
	c.msgChan <- display.EnsembleErrorMsg{
		TabID: c.tabID,
		Err:   fmt.Errorf("[%s] stream error: %w", c.speaker, err),
	}
}

func (c *ensembleCollector) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.content.String()
}

func (c *ensembleCollector) tokenUsage() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inTok, c.outTok
}
