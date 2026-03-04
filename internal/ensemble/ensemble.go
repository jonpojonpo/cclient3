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
		agents:    agents,
		tabID:     tabID,
		msgChan:   msgChan,
		userChan:  userChan,
		providers: providers,
		cfg:       cfg,
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
// It sends the initial prompt to all agents in parallel, collects responses,
// then waits for user input to continue the discussion.
func (e *Ensemble) Run(ctx context.Context, initialPrompt string) {
	fullTabID := e.TabID()

	// Add user prompt to transcript
	e.addTranscript("You", initialPrompt)

	// First round: all agents respond simultaneously
	e.runRound(ctx, fullTabID)

	// Loop: wait for user input, then run another round
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

			// Check for done command
			if strings.TrimSpace(strings.ToLower(userMsg)) == "/done" {
				e.msgChan <- display.EnsembleDoneMsg{
					TabID:   fullTabID,
					Summary: e.buildSummary(),
				}
				return
			}

			// Add user message to transcript
			e.addTranscript("You", userMsg)

			// Run another round
			e.runRound(ctx, fullTabID)
		}
	}
}

// runRound fans out to all agents in parallel and waits for all to complete.
func (e *Ensemble) runRound(ctx context.Context, fullTabID string) {
	var wg sync.WaitGroup

	for _, agent := range e.agents {
		wg.Add(1)
		go func(a AgentSpec) {
			defer wg.Done()
			e.runAgent(ctx, a, fullTabID)
		}(agent)
	}

	wg.Wait()

	// Drain any user messages that arrived during the round
	// and queue them for the next round
	select {
	case userMsg := <-e.userChan:
		e.addTranscript("You", userMsg)
		e.msgChan <- display.EnsembleUserMsg{TabID: fullTabID, Text: userMsg}
	default:
	}

	e.msgChan <- display.EnsembleRoundDoneMsg{TabID: fullTabID}
}

// runAgent sends the transcript to a single agent and streams the response.
func (e *Ensemble) runAgent(ctx context.Context, agent AgentSpec, fullTabID string) {
	provider := e.providers.Get(agent.Provider)

	// Build system prompt with personality
	systemPrompt := fmt.Sprintf(
		`You are "%s" in a group discussion with other AI agents and a human user.

%s

Rules:
- Be concise (2-4 paragraphs max)
- Build on what others said, offer your unique perspective
- Disagree constructively when you see things differently
- Don't repeat what others have already said
- Address other agents by name when responding to their points
- Stay in character`,
		agent.Name, agent.Personality)

	// Build the transcript as the user message
	transcriptText := e.buildTranscriptText()

	req := &api.Request{
		Model:     agent.Model,
		MaxTokens: e.cfg.MaxTokens,
		System:    systemPrompt,
		Messages: []api.Message{
			{Role: "user", Content: transcriptText + "\n\n---\nContinue the discussion as " + agent.Name + ":"},
		},
	}

	// Signal that this agent is starting
	e.msgChan <- display.EnsembleSpeakerMsg{
		TabID:   fullTabID,
		Speaker: agent.Name,
		Color:   agent.Color,
		Model:   agent.Model,
	}

	// Stream the response
	collector := &ensembleCollector{
		tabID:   fullTabID,
		speaker: agent.Name,
		color:   agent.Color,
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

	// Add to transcript
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

// ensembleCollector implements api.StreamCallback to collect and stream agent output.
type ensembleCollector struct {
	tabID   string
	speaker string
	color   string
	msgChan chan tea.Msg

	mu      sync.Mutex
	content strings.Builder
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

func (c *ensembleCollector) OnThinkingDelta(index int, thinking string) {
	// Thinking is hidden in ensemble mode for cleaner group chat
}

func (c *ensembleCollector) OnInputJSONDelta(index int, partialJSON string) {}
func (c *ensembleCollector) OnContentBlockStop(index int)                   {}
func (c *ensembleCollector) OnMessageDelta(delta api.MessageDelta, usage *api.Usage) {}
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
