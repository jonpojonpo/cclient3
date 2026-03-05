# Ensemble Mode Technical Analysis Report

**Project:** cclient3  
**Date:** 2024  
**Focus:** Ensemble multi-agent mode architecture and enhancement opportunities

---

## Table of Contents

1. [Current Ensemble Flow](#current-ensemble-flow)
2. [Auto-Casting Analysis](#auto-casting-analysis)
3. [Top 3 Architectural Improvements](#top-3-architectural-improvements)
4. [Ensemble Synthesis Feature: Design & Implementation](#ensemble-synthesis-feature-design--implementation)
5. [Smarter Auto-Casting Strategy](#smarter-auto-casting-strategy)
6. [Conclusions](#conclusions)

---

## Current Ensemble Flow

Based on analysis of `internal/ensemble/ensemble.go`, the current flow is:

### Phase 1: Initialization (`Ensemble` struct)

```go
type Ensemble struct {
    agents     []AgentSpec
    transcript []TranscriptEntry
    mu         sync.Mutex
    
    tabID             string
    msgChan           chan tea.Msg
    userChan          chan string
    providers         *api.ProviderRegistry
    cfg               *config.Config
    
    microRounds       int  // default: 2
    responseTokens    int  // default: 300
    thinkTokens       int  // default: 150
}
```

### Phase 2: Run Method Flow

```go
func (e *Ensemble) Run(ctx context.Context, initialPrompt string) {
    e.addTranscript("You", initialPrompt)
    e.runRound(ctx, fullTabID)  // First round with just the prompt
    
    for {
        case userMsg := <-e.userChan:
            e.addTranscript("You", userMsg)
            e.runRound(ctx, fullTabID)
    }
}
```

### Phase 3: runRound - The Core Loop

```go
func (e *Ensemble) runRound(ctx context.Context, fullTabID string) {
    for i := 0; i < e.microRounds; i++ {
        scratchpads := e.thinkPhase(ctx)       // PARALLEL
        e.speakPhase(ctx, fullTabID, scratchpads)  // SEQUENTIAL
    }
}
```

#### Think Phase (Parallel `thinkPhase`)

All agents concurrently generate private scratchpads:

```go
func (e *Ensemble) thinkPhase(ctx context.Context) map[string]string {
    var wg sync.WaitGroup
    for _, agent := range e.agents {
        go func(a AgentSpec) {
            notes := e.generateScratchpad(ctx, a)  // Private thinking
            scratchpads[a.Name] = notes
        }(agent)
    }
    wg.Wait()
    return scratchpads
}
```

The scratchpad prompt from `generateScratchpad()`:
```go
systemPrompt := fmt.Sprintf(
    `You are "%s" in a group discussion.

%s

You are LISTENING. Write 3 brief private notes for yourself (one sentence each):
- What specific point do you most want to respond to?
- What unique angle or disagreement do you bring?
- What direction do you want to push the discussion?
```

#### Speak Phase (Sequential `speakPhase`)

Agents respond one at a time, seeing prior responses and their own scratchpad:

```go
func (e *Ensemble) speakPhase(ctx context.Context, fullTabID string, 
    scratchpads map[string]string) {
    for _, agent := range e.agents {
        e.runAgent(ctx, agent, fullTabID, scratchpads[agent.Name])
    }
}
```

The response prompt from `runAgent()`:
```go
systemPrompt := fmt.Sprintf(
    `You are "%s" in a group discussion with other AI agents and a human user.

%s%s    // includes their private notes

Rules:
- Be VERY concise: 2-4 sentences max
- Respond to what was most recently said — prioritise the last speaker
- Address agents by name when responding to their points
- Make ONE clear point per turn; don't try to summarise everything
- Disagree constructively when you see things differently
```

### Issues Identified

| Issue | Impact | Severity |
|-------|--------|----------|
| **No synthesis agent** | Discussion may go nowhere, no conclusion | High |
| **Fixed 2 rounds** | Limits depth of exploration | Medium |
| **Static round order** | No topic-based scheduling | Medium |
| **All-or-nothing casting** | JSON parsing can fail silently on malformed output | Low |
| **No conflict detection** | Agents may spiral without guidance | High |
| **Token budget per agent** (300) | May not allow complete thoughts | Medium |

---

## Auto-Casting Analysis

### Current Implementation (`casting.go`)

```go
func CastAgents(ctx context.Context, provider api.Provider, model string, 
    prompt string, availableProviders []string, 
    availableModels []string) ([]AgentSpec, error) {
    
    // Casting prompt asks AI to create 2-4 agents with unique personalities
    
    castingPrompt := fmt.Sprintf(`You are a casting director for an AI ensemble discussion.
    
Given a user's prompt, create 2-4 AI agents with diverse perspectives to discuss it.
Each agent should have a unique name, personality, and if possible, 
use different AI providers/models for diversity.

Available providers: %s
Available models: %s

The user's prompt is:
%s
    
Respond with ONLY a JSON array...`)
    
    // Sends request, extracts text (strips markdown), parses JSON
```

### Current Limitations

1. **Black box casting**: The model decides diversity without explicit guidelines
2. **No fallback mechanism**: If JSON parsing fails → error returned
3. **Static roster**: Agents don't adapt during conversation
4. **No topic mapping**: Generic "diverse perspectives" is vague

---

## Top 3 Architectural Improvements

### #1: Add Ensemble Synthesis Agent

**Priority:** HIGH  
**Problem:** Discussions may lack direction and conclusion  
**Solution:** Add a "Synthesizer" agent that aggregates all responses, summarizes key points, and proposes next steps.

#### Why this matters:
- Provides natural discussion closure (like a meeting moderator)
- Creates accountability loop (agents work toward synthesis goal)
- Reduces token waste by preventing circular arguments
- Enables `/done` to work beyond the magic word

### #2: Intelligent Topic Tracking & Adaptive Rounding

**Priority:** HIGH  
**Problem:** Agents use round-robin regardless of topical relevance  
**Solution:** Implement topic awareness so agents only respond when relevant, and track conversation progression.

#### Benefits:
- More efficient token usage (no one-topic agents)
- Natural discussion flow rather than forced turns
- Enables dynamic agent activation based on sub-topics

### #3: Conflict Resolution & Voting Mechanisms

**Priority:** MEDIUM  
**Problem:** Agents may disagree endlessly without resolution path  
**Solution:** Add voting system and conflict escalation paths.

#### Features:
- Majority consensus building
- Minority viewpoint preservation with counter-arguments
- Escalation to synthesis agent for deadlocks

---

## Ensemble Synthesis Feature: Design & Implementation

### Proposed Architecture Additions

Add a `SynthesizerAgent` type to `ensemble.go`:

```go
type SynthesizerAgent struct {
    AgentSpec
    role      string  // "Moderator/Synthesis/Summary"
    isActive  bool    // true if agent should contribute this round
}

// Extended Ensemble struct
type Ensemble struct {
    agents       []AgentSpec        // Original cast agents
    synthesizer   *SynthesizerAgent  // Synthesis moderator (optional)
    transcript   []TranscriptEntry
    mu           sync.Mutex
    
    topicTracker TopicTracker    // New: tracks current discussion topics
    roundState   RoundState      // New: tracks who's spoken this round
    consensus    ConsensusData   // New: voting/conflict tracking
    ...
}
```

### Implementation: Synthesis Phase Addition

Add to `runRound()`:

```go
func (e *Ensemble) runRound(ctx context.Context, fullTabID string) {
    for i := 0; i < e.microRounds; i++ {
        select {
        case <-ctx.Done():
            return
        default:
        }
        
        // Phase 1: All agents think privately
        scratchpads := e.thinkPhase(ctx)
        
        // Phase 2: Sequential speak phase with topic filters
        e.speakPhase(ctx, fullTabID, scratchpads)
        
        // <-- NEW PHASE 3: Synthesis/Moderation phase -->
        if e.synthesizer != nil && i == e.microRounds-1 {
            // After final round, run synthesis to summarize discussion
            e.synthesizeRound(ctx, fullTabID, scratchpads)
        }
    }
}
```

### Implementation: synthesizeRound() Method

```go
// synthesizeRound generates a synthesis of all responses
func (e *Ensemble) synthesizeRound(ctx context.Context, 
    fullTabID string, scratchpads map[string]string) {
    
    // Collect all transcript and scratchpad info
    transcriptText := e.buildTranscriptText()
    
    // Prepare synthesis prompt that includes:
    // - All previous messages
    // - Each agent's private notes from scratchpads
    // - Current topic/context
    
    systemPrompt := fmt.Sprintf(
        `You are the ensemble synthesizer/moderator for this discussion.

## Your Role
- Summarize key points raised by all participants
- Note areas of agreement and disagreement  
- Identify unresolved questions or tensions
- Build a coherent summary that captures the discussion's essence
    
## Discussion Transcript
%s
    
## Participants' Private Thoughts
%v (private scratchpad notes from each agent)

## Current Context
- Topics discussed: %s
- Key decisions: %s

Generate a synthesis response that:
1. Acknowledges each participant's contribution
2. Summarizes the main themes and points
3. Notes where consensus was or wasn't reached
4. Proposes next steps or new directions if appropriate

Rules:
- Be neutral and balanced
- Give credit to specific agents for their points
- Keep under 6 sentences
- Don't invent new claims not mentioned
    
Response format (plain text, under 600 tokens):`,
        transcriptText, scratchpads, e.topicTracker.ActiveTopics())
    
    req := &api.Request{
        Model:     "claude-sonnet-4", // Synthesis agent uses high-quality model
        MaxTokens: 1500,              // Allow comprehensive synthesis
        System:    []api.SystemBlock{{Type: "text", Text: systemPrompt}},
        Messages: []api.Message{
            {Role: "user", Content: e.buildTranscriptText()},
        },
    }
    
    // Stream the synthesis to TUI
    collector := &synthesisCollector{tabID: fullTabID, msgChan: e.msgChan}
    provider := e.providers.Get("anthropic") // Prefer higher-quality model
    err := provider.StreamMessage(ctx, req, collector)
    if err != nil {
        e.msgChan <- display.EnsembleErrorMsg{
            TabID: fullTabID,
            Err:   fmt.Errorf("synthesis failed: %w", err),
        }
        return
    }
    
    // Add synthesis to transcript as a special entry
    e.mu.Lock()
    e.transcript = append(e.transcript, TranscriptEntry{
        Speaker: "Synthesizer",
        Text: collector.text(),
    })
    e.mu.Unlock()
    
    e.msgChan <- display.EnsembleRoundDoneMsg{TabID: fullTabID}
    e.msgChan <- display.EnsembleSummaryMsg{
        TabID:   fullTabID,
        Summary: collector.text(),  // For /done command
    }
}
```

### Configuration for Synthesis Agent

Add to `AgentSpec`:

```go
// Agent can specify it's a synthesis agent
type AgentSpec struct {
    Name        string `json:"name" yaml:"name"`
    Personality string `json:"personality" yaml:"personality"`
    Model       string `json:"model" yaml:"model"`
    Provider    string `json:"provider" yaml:"provider"`
    Color       string `json:"color" yaml:"color"`
    
    // NEW fields
    Role        string `json:"role,omitempty" yaml:"role,omitempty"`  // "synthesizer", "moderator"
    IsSynthesis bool   `json:"is_synthesis" yaml:"is_synthesis"`       // Special synthesizer agent
}
```

### Adding Configuration Option

Extend `config.Config`:

```go
type Config struct {
    Model      string
    MaxTokens  int
    
    EnsemblePresets []AgentSpec    // Preset agent rosters
    
    // NEW settings for synthesis
    EnableSynthesis bool    // Enable summary/moderation agent (default: false)
    SynthesisModel  string   // Model for synthesis agent (default: same as main model)
    SynthesisRole   string   // "Moderator" or "Summary" or empty
}
```

---

## Smarter Auto-Casting Strategy

### Enhancement: Keyword & Topic Classification

The current `castAgents()` in `casting.go` is too generic. Here's how to make it smarter:

#### Current Casting Prompt (simplified):

```go
castingPrompt := fmt.Sprintf(`You are a casting director...
Create 2-4 AI agents with diverse perspectives to discuss it...`)
```

#### New Enhanced Casting with Topic Mapping

Add `topicClassifier()` helper function and enhanced prompt:

```go
// topicKeywords maps prompt topics to agent personality archetypes
var topicKeywords = map[string]map[string]string{
    // Architecture/System Design topics
    "architecture": {
        "senior-dev":      "experienced architect who knows best practices",
        "junior-dev":     "eager learner asking about fundamentals",
        "security-audit": "cautious security-focused reviewer",
        "performance-pro": "optimization and scalability expert",
        "startup-lead":   "cost-conscious shipping-first mindset",
    },
    // More topic categories...
}

// classifyTopics extracts topics from prompt and suggests agent archetypes
func classifyTopics(prompt string) map[string][]string {
    // Simple keyword matching first pass
    topics := make(map[string]bool)
    
    lowerPrompt := strings.ToLower(prompt)
    for _, kw := range []string{
        "api", "microservices", "monolith", "database", "cache",
        "security", "auth", "encryption", "tls", "jwt",
        "performance", "latency", "throughput", "scaling",
        "cost", "budget", "mvp", "shipping",
    } {
        if strings.Contains(lowerPrompt, kw) {
            topics[kw] = true
        }
    }
    
    return topics
}

// enhancedCastAgents is the improved version
func EnhancedCastAgents(
    ctx context.Context,
    provider api.Provider,
    model string,
    prompt string,
    availableProviders []string,
    availableModels []string,
) ([]AgentSpec, error) {
    
    // Step 1: Classify topics from the prompt
    keywords := classifyTopics(prompt)
    
    // Step 2: Extract task intent
    intent := classifyIntent(prompt)
    // Returns: "debate", "design-doc", "code-review", "exploration"
    
    // Step 3: Generate casting JSON with topic-based diversity
    castingPrompt := fmt.Sprintf(buildCastingPrompt(prompt, keywords, intent, provider, model))
    
    // ... rest of implementation (same as before)
}

// buildCastingPrompt generates the enhanced casting system prompt
func buildCastingPrompt(
    prompt string, 
    keywords map[string]bool,
    intent string,
    providers string,
    models string,
) string {
    
    return fmt.Sprintf(`You are an expert casting director for multi-agent AI ensemble discussions.

## Your Task
Given the user's prompt about %v, design an agent ensemble that will have 
meaningful, productive discussion while staying on topic.

## Prompt Being Discussed
%s

## Topics Identified: %v

## Discussion Intent: %s (debate | design-doc | code-review | exploration)

## Agent Count Guidelines
- Debate topics: Aim for 3-4 agents with contrasting viewpoints
- Technical review: Focus on complementary expertise areas
- Creative exploration: Start with 2-3 agents, allow expansion if needed

## Diversity Requirements
1. **Perspective diversity**: Ensure different angles on the topic
2. **Expertise diversity**: If applicable (dev/security/performance/etc.)
3. **Model diversity**: Use available providers for variety
4. **Personality tension**: Create natural conversation dynamics

## Agent Specification Format
Create agents like:
- "Architect" for high-level design questions
- "Security-skeptic" when security topics arise
- "Pragmatist" for implementation/production concerns  
- "Innovator" for creative problem exploration

## Output Rules
Respond with ONLY a JSON array of agent objects. Format:
[
  {
    "name": "<short name like 'Sage' or 'Architect'>",
    "personality": "<brief description, 15-20 words>",
    "model": "",           // Leave empty for default, specify for diversity
    "provider": "",        // Or 'ollama' to use local model
    "color": "#HEXCODE"    // Distinct colors for visual differentiation
  }
]

## Available Options
Providers: %s
Models: %s

## Response Format Reminder
NO extra text, no markdown code fences unless inside the JSON. 
Just the raw JSON array starting with '[' and ending with ']'.`,
        keywords, prompt, keywords, intent, providers, models)
}

// classifyIntent determines discussion goal from prompt
func classifyIntent(prompt string) string {
    lower := strings.ToLower(prompt)
    
    // Check for debate-style prompts
    if strings.Contains(lower, "should we") || 
       strings.Contains(lower, "compare") ||
       strings.Contains(lower, "vs") ||
       strings.Contains(lower, "better than") {
        return "debate"
    }
    
    // Check for code/design review prompts
    if strings.Contains(lower, "review") ||
       strings.Contains(lower, "evaluate") ||
       strings.Contains(lower, "analyze this") {
        return "code-review"
    }
    
    // Check for open-ended exploration
    if strings.Contains(lower, "brainstorm") ||
       strings.Contains(lower, "ideas") ||
       strings.Contains(lower, "approaches to") {
        return "design-doc"
    }
    
    // Default assumption
    return "exploration"
}
```

### Additional Smart Features for Casting

#### Feature 1: Provider/Model Distribution Optimization

```go
// Distribute agents across providers when possible to reduce cost/lower-latency
func distributeProviders(agentCount int, 
    providerPool []string, models []string) map[string]bool {
    
    // If multiple providers available, assign different ones
    assignment := make(map[int]string)
    
    if len(providerPool) >= 2 && agentCount > len(models[0]) || similarCheck() {
        // Round-robin distribution across providers for first N agents
        for i := 0; i < agentCount && i < len(providerPool); i++ {
            assignment[i] = providerPool[i%len(providerPool)]
        }
    } else if len(models[0]) > 2 && agentCount > 2 {
        // Fallback to model diversity within same provider
        for i := 0; i < agentCount && i < len(models); i++ {
            assignment[i] = model(i % len(model))
        }
    }
    
    return assignment
}
```

#### Feature 2: Dynamic Agent Adaptation (Future Enhancement)

Allow agents to modify their role/persona during conversation based on detected dynamics:

```go
type DynamicAgent interface {
    ObserveConversation() ConversationState       // Read transcript history
    AdjustRole(topic string, intent string) error   // "I'll take moderation here"
}
```

---

## Additional Improvement Ideas

### #4: Dynamic Micro-Rounds Configuration

Current code fixes `microRounds = 2`. Make it adaptive:

```go
// In generateScratchpad() after getting response
func (e *Ensemble) adaptRoundCount(ctx context.Context, 
    roundNum int) {
    
    // If agents are not engaging with each other, increase rounds
    engagementScore := e.measureEngagement(ctx)
    
    if engagementScore < 0.5 && roundNum < 4 {
        e.msgChan <- display.EnsemblePromptMsg{
            TabID:   fullTabID,
            Hint: "Discussion not moving forward - continuing another round...",
        }
    }
}

func (e *Ensemble) measureEngagement(ctx context.Context) float64 {
    // Analyze transcript for cross-agent references
    // Count mentions and disagreements vs pure self-talk
    
    var refs int
    for _, entry := range e.transcript {
        if strings.Contains(entry.Text, "sage") || 
           strings.Contains(entry.Text, "spark") {  // agent names
            refs++
        }
    }
    
    totalWords := len(strings.Join(e.buildTranscriptText(), " "))
    return float64(refs) / max(1, float64(totalWords)/20) * 2.0
}
```

### #5: Voting & Consensus Mechanism

Add voting after each round:

```go
func (e *Ensemble) tallyRoundVotes(ctx context.Context, 
    fullTabID string) []consensusPoint {
    
    // Analyze responses for agreement signals
    var points []consensusPoint
    
    transcript := e.buildTranscriptText()
    
    // Extract claims and opinions from responses
    claims := extractClaims(transcript)
    
    for _, claim := range claims {
        support := countSupporters(claims, transcript)
        
        if support/len(e.agents) >= 0.6 {
            points = append(points, consensusPoint{
                Statement: claim,
                Support:   support,
                Type: "consensus",
            })
        } else if support < float64(len(e.agents))/3 {
            points = append(points, consensusPoint{
                Statement: claim,
                Support:   support,
                Type: "controversial",
            })
        }
    }
    
    return points
}
```

---

## Conclusions

### Summary of Recommendations

| Rank | Feature | Implementation Effort | Impact | Priority |
|------|---------|----------------------|--------|----------|
| 1 | Ensemble Synthesis Agent | MEDIUM | HIGH (discussion quality) | **Must Have** |
| 2 | Topic-Aware Agent Casting | LOW | HIGH (relevance) | High |
| 3 | Dynamic Round Configuration | MEDIUM | MEDIUM (efficiency) | Medium |
| 4 | Voting/Consensus System | MEDIUM-HIGH | MEDIUM (engagement) | Optional |

### Implementation Order

1. **Phase 1 (Quick Win - 1-2 hours)**: Topic classification + intent detection
   - Add `classifyTopics()` and `classifyIntent()` helpers
   - Inject into casting prompt
   
2. **Phase 2 (Core Feature - 4-6 hours)**: Synthesis agent
   - Implement `synthesizeRound()` method
   - Handle synthesizer output display in TUI
   
3. **Phase 3 (Polish - additional time)**: Adaptive features
   - Dynamic micro-round counts based on engagement
   - Optional voting/consensus tracking

### Sample Configuration Additions

Update `./config.yaml` to enable synthesis:

```yaml
# Add to config after ensemble_presets section
ensemble:
  enable_synthesis: true          # Enable summary moderator agent
  synthesis_model: "claude-sonnet-4-6"  # High-quality for synthesis
  synthesis_role: "Moderator"     # or "Summary"
  
  # Adaptive behavior
  adaptive_rounds: true           # Increase rounds when engagement is high
  
  # Topic sensitivity
  topic_keywords_file: "topics.yaml"  # Custom topic mappings
    
ensemble_presets:
  - name: full-discussion
    agents:
      - role: "Architect"        # Architect, Pragmatist, Security, etc.
      - role: "Pragmatist"
      - role: "Security-skeptic"
    synthesizer: true             # This preset includes synthesis agent
```

### Testing Checklist

When implementing enhancements:

- [ ] Auto-casting produces valid JSON (test with various prompts)
- [ ] Synthesis agent receives all prior transcript correctly
- [ ] TUI displays synthesis message appropriately styled
- [ ] Agent references are grammatically correct ("Sage notes that...")
- [ ] Topic detection works for edge cases
- [ ] Thread safety maintained (mu locks on transcript in synthesizeRound)

---

**End Report**

Generated from analysis of:
- `/home/jonpo/cclient3/internal/ensemble/ensemble.go`
- `/home/jonpo/cclient3/internal/ensemble/casting.go`  
- `/home/jonpo/cclient3/internal/agent/agent.go`
- `/home/jonpo/cclient3/config.yaml`
- `/home/jonpo/cclient3/README.md`
