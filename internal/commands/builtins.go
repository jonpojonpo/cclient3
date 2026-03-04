package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonpo/cclient3/internal/agent"
	"github.com/jonpo/cclient3/internal/api"
	"github.com/jonpo/cclient3/internal/config"
	"github.com/jonpo/cclient3/internal/display"
	"github.com/jonpo/cclient3/internal/ensemble"
)

// RegisterBuiltins registers all built-in slash commands.
func RegisterBuiltins(reg *Registry, ag *agent.Agent, msgChan chan tea.Msg, dm *display.Model) {
	reg.Register(Command{
		Name:        "/help",
		Description: "Show available commands",
		Handler: func(args string) error {
			msgChan <- display.StatusMsg{Text: reg.Help()}
			return nil
		},
	})

	// Track cached models for cycling
	var cachedModels []string

	ensureModels := func() bool {
		if len(cachedModels) > 0 {
			return true
		}
		models, err := ag.ListModels(context.Background())
		if err != nil {
			msgChan <- display.ErrorMsg{Err: fmt.Errorf("failed to list models: %w", err)}
			return false
		}
		cachedModels = models
		return true
	}

	modelHandler := func(args string) error {
		args = strings.TrimSpace(args)
		if args == "" || args == "list" {
			if !ensureModels() {
				return nil
			}
			msgChan <- display.ModelsListMsg{Models: cachedModels}
			return nil
		}
		if args == "next" || args == "cycle" {
			// Cycle to next model
			if !ensureModels() {
				return nil
			}
			if len(cachedModels) == 0 {
				msgChan <- display.StatusMsg{Text: "No models available"}
				return nil
			}
			current := ag.Config().Model
			nextIdx := 0
			for i, m := range cachedModels {
				if m == current {
					nextIdx = (i + 1) % len(cachedModels)
					break
				}
			}
			ag.SetModel(cachedModels[nextIdx])
			msgChan <- display.StatusMsg{Text: fmt.Sprintf("Model switched to: %s", cachedModels[nextIdx])}
			return nil
		}
		ag.SetModel(args)
		msgChan <- display.StatusMsg{Text: fmt.Sprintf("Model switched to: %s", args)}
		return nil
	}

	reg.Register(Command{
		Name:        "/model",
		Description: "Switch model (/model <name>, /model list, /model next to cycle)",
		Handler:     modelHandler,
	})

	reg.Register(Command{
		Name:        "/models",
		Description: "List available models from API",
		Handler: func(args string) error {
			return modelHandler("list")
		},
	})

	reg.Register(Command{
		Name:        "/theme",
		Description: "Switch theme (cyber/ocean/ember/mono)",
		Handler: func(args string) error {
			name := strings.TrimSpace(args)
			if _, ok := display.Themes[name]; !ok {
				msgChan <- display.StatusMsg{Text: fmt.Sprintf("Unknown theme: %s. Available: %s", name, strings.Join(display.ThemeNames(), ", "))}
				return nil
			}
			msgChan <- display.SetThemeMsg{Name: name}
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/clear",
		Description: "Clear conversation history",
		Handler: func(args string) error {
			ag.Conversation().Clear()
			msgChan <- display.ClearMsg{}
			msgChan <- display.StatusMsg{Text: "Conversation cleared"}
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/save",
		Description: "Save conversation to file",
		Handler: func(args string) error {
			path := strings.TrimSpace(args)
			if path == "" {
				path = "conversation.json"
			}
			if err := ag.Conversation().Save(path); err != nil {
				msgChan <- display.ErrorMsg{Err: fmt.Errorf("save failed: %w", err)}
				return nil
			}
			msgChan <- display.StatusMsg{Text: fmt.Sprintf("Conversation saved to %s", path)}
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/load",
		Description: "Load conversation from file",
		Handler: func(args string) error {
			path := strings.TrimSpace(args)
			if path == "" {
				path = "conversation.json"
			}
			if err := ag.Conversation().Load(path); err != nil {
				msgChan <- display.ErrorMsg{Err: fmt.Errorf("load failed: %w", err)}
				return nil
			}
			msgChan <- display.StatusMsg{Text: fmt.Sprintf("Conversation loaded from %s", path)}
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/stats",
		Description: "Show token usage statistics",
		Handler: func(args string) error {
			msgChan <- display.StatusMsg{Text: fmt.Sprintf(
				"Model: %s\nConversation turns: %d",
				ag.Config().Model,
				len(ag.Conversation().Messages),
			)}
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/quit",
		Description: "Exit the program",
		Handler: func(args string) error {
			msgChan <- display.QuitMsg{}
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/skills",
		Description: "List available skills and their active state",
		Handler: func(args string) error {
			mgr := ag.Skills()
			available := mgr.Available()
			if len(available) == 0 {
				msgChan <- display.StatusMsg{Text: "No skills found. Add .md files to .skills/ in your project directory."}
				return nil
			}
			var b strings.Builder
			b.WriteString("Available skills (use /skill <name> to toggle):\n")
			for _, s := range available {
				marker := "  "
				if mgr.IsActive(s.Name) {
					marker = "✓ "
				}
				desc := s.Description
				if desc == "" {
					desc = "(no description)"
				}
				b.WriteString(fmt.Sprintf("%s%-20s %s\n", marker, s.Name, desc))
			}
			msgChan <- display.StatusMsg{Text: b.String()}
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/skill",
		Description: "Toggle a skill on/off (/skill <name>). No args lists skills.",
		Handler: func(args string) error {
			name := strings.TrimSpace(args)
			mgr := ag.Skills()
			if name == "" {
				// Redirect to /skills listing
				available := mgr.Available()
				if len(available) == 0 {
					msgChan <- display.StatusMsg{Text: "No skills found. Add .md files to .skills/ in your project directory."}
					return nil
				}
				var b strings.Builder
				b.WriteString("Available skills (use /skill <name> to toggle):\n")
				for _, s := range available {
					marker := "  "
					if mgr.IsActive(s.Name) {
						marker = "✓ "
					}
					desc := s.Description
					if desc == "" {
						desc = "(no description)"
					}
					b.WriteString(fmt.Sprintf("%s%-20s %s\n", marker, s.Name, desc))
				}
				msgChan <- display.StatusMsg{Text: b.String()}
				return nil
			}
			active, found := mgr.Toggle(name)
			if !found {
				msgChan <- display.ErrorMsg{Err: fmt.Errorf("skill %q not found (use /skills to list)", name)}
				return nil
			}
			state := "deactivated"
			if active {
				state = "activated"
			}
			msgChan <- display.StatusMsg{Text: fmt.Sprintf("Skill %q %s", name, state)}
			return nil
		},
	})

	// --- Tab commands ---

	reg.Register(Command{
		Name:        "/tab",
		Description: "Switch to a tab by number or name (/tab <n|name>)",
		Handler: func(args string) error {
			args = strings.TrimSpace(args)
			if args == "" {
				msgChan <- display.StatusMsg{Text: "Usage: /tab <number|name>"}
				return nil
			}
			tm := dm.TabManager()
			// Try as number first
			if n, err := strconv.Atoi(args); err == nil {
				idx := n - 1
				if tm.SetActive(idx) {
					msgChan <- display.StatusMsg{Text: fmt.Sprintf("Switched to tab %d", n)}
				} else {
					msgChan <- display.StatusMsg{Text: fmt.Sprintf("No tab %d (have %d tabs)", n, tm.Count())}
				}
				return nil
			}
			// Try as name/ID
			if _, i := tm.FindByID(args); i >= 0 {
				tm.SetActive(i)
				msgChan <- display.StatusMsg{Text: fmt.Sprintf("Switched to tab: %s", args)}
				return nil
			}
			// Try label match
			for i, tab := range tm.Tabs() {
				if strings.EqualFold(tab.Label, args) {
					tm.SetActive(i)
					msgChan <- display.StatusMsg{Text: fmt.Sprintf("Switched to tab: %s", tab.Label)}
					return nil
				}
			}
			msgChan <- display.StatusMsg{Text: fmt.Sprintf("Tab %q not found", args)}
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/tabs",
		Description: "List open tabs with status",
		Handler: func(args string) error {
			tm := dm.TabManager()
			var b strings.Builder
			b.WriteString("Open tabs:\n")
			for i, tab := range tm.Tabs() {
				marker := "  "
				if i == tm.ActiveIdx() {
					marker = "► "
				}
				status := ""
				switch tab.Kind {
				case display.TabAgent:
					switch tab.Status {
					case display.TabRunning:
						status = " [running]"
					case display.TabDone:
						status = " [done]"
					case display.TabError:
						status = " [error]"
					}
				case display.TabBash:
					status = fmt.Sprintf(" [bash: %s]", tab.ID)
				}
				pin := ""
				if tab.UserPinned {
					pin = " (pinned)"
				}
				unread := ""
				if tab.Unread {
					unread = " *"
				}
				b.WriteString(fmt.Sprintf("%s%d: %s%s%s%s\n", marker, i+1, tab.Label, status, pin, unread))
			}
			msgChan <- display.StatusMsg{Text: b.String()}
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/newtab",
		Description: "Open a new bash session tab (/newtab [session-name])",
		Handler: func(args string) error {
			name := strings.TrimSpace(args)
			if name == "" {
				name = fmt.Sprintf("bash-%d", dm.TabManager().Count())
			}
			// Create/acquire the bash session
			if err := ag.Sessions().Acquire(name); err != nil {
				msgChan <- display.ErrorMsg{Err: fmt.Errorf("session %q: %w", name, err)}
				return nil
			}

			tm := dm.TabManager()
			tabID := "bash-" + name
			tab := &display.Tab{
				ID:          tabID,
				Label:       name,
				Kind:        display.TabBash,
				Status:      display.TabRunning,
			}
			tab.SetSessionName(name)
			idx := tm.Add(tab)
			tm.SetActive(idx)

			// Start observer goroutine to pump bash output to the display
			obsCh := make(chan string, 200)
			ag.Sessions().AddObserver(name, obsCh)
			go func() {
				for line := range obsCh {
					msgChan <- display.BashOutputMsg{TabID: tabID, Lines: []string{line}}
				}
			}()

			msgChan <- display.StatusMsg{Text: fmt.Sprintf("Opened bash tab: %s", name)}
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/closetab",
		Description: "Close a tab by number or name (/closetab [n|name])",
		Handler: func(args string) error {
			tm := dm.TabManager()
			args = strings.TrimSpace(args)

			if args == "" {
				// Close current tab
				if tm.ActiveIdx() == 0 {
					msgChan <- display.StatusMsg{Text: "Cannot close the Chat tab"}
					return nil
				}
				id := tm.Active().ID
				tm.Remove(id)
				msgChan <- display.StatusMsg{Text: "Tab closed"}
				return nil
			}

			// Try as number
			if n, err := strconv.Atoi(args); err == nil {
				idx := n - 1
				if idx <= 0 || idx >= tm.Count() {
					msgChan <- display.StatusMsg{Text: fmt.Sprintf("Cannot close tab %d", n)}
					return nil
				}
				id := tm.Tabs()[idx].ID
				tm.Remove(id)
				msgChan <- display.StatusMsg{Text: fmt.Sprintf("Closed tab %d", n)}
				return nil
			}

			// Try as name/ID
			if tm.Remove(args) {
				msgChan <- display.StatusMsg{Text: fmt.Sprintf("Closed tab: %s", args)}
				return nil
			}
			msgChan <- display.StatusMsg{Text: fmt.Sprintf("Tab %q not found", args)}
			return nil
		},
	})

	// --- Provider commands ---

	reg.Register(Command{
		Name:        "/provider",
		Description: "Show or switch default AI provider (/provider [name])",
		Handler: func(args string) error {
			pr := ag.Providers()
			args = strings.TrimSpace(args)
			if args == "" {
				var b strings.Builder
				b.WriteString(fmt.Sprintf("Current provider: %s\n", pr.DefaultName()))
				b.WriteString("Registered providers: " + strings.Join(pr.Names(), ", ") + "\n")
				b.WriteString("Usage: /provider <name>  — switch default")
				msgChan <- display.StatusMsg{Text: b.String()}
				return nil
			}
			if pr.SetDefault(args) {
				msgChan <- display.StatusMsg{Text: fmt.Sprintf("Default provider switched to: %s", args)}
			} else {
				msgChan <- display.StatusMsg{Text: fmt.Sprintf(
					"Unknown provider %q. Registered: %s",
					args, strings.Join(pr.Names(), ", "),
				)}
			}
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/providers",
		Description: "List registered AI providers",
		Handler: func(args string) error {
			pr := ag.Providers()
			var b strings.Builder
			b.WriteString("Registered providers:\n")
			for _, name := range pr.Names() {
				marker := "  "
				if name == pr.DefaultName() {
					marker = "* "
				}
				b.WriteString(fmt.Sprintf("%s%s\n", marker, name))
			}
			b.WriteString("\nUse /provider <name> to switch default.")
			msgChan <- display.StatusMsg{Text: b.String()}
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/eval",
		Description: "Run a task on two providers in parallel and compare (/eval <task>)",
		Handler: func(args string) error {
			task := strings.TrimSpace(args)
			if task == "" {
				msgChan <- display.StatusMsg{Text: "Usage: /eval <task description>"}
				return nil
			}
			pr := ag.Providers()
			names := pr.Names()

			// Need at least 2 providers to compare
			if len(names) < 2 {
				msgChan <- display.StatusMsg{Text: fmt.Sprintf(
					"Need at least 2 providers for eval (have: %s). Add ollama_endpoint to config.yaml.",
					strings.Join(names, ", "),
				)}
				return nil
			}

			// Craft a prompt that makes the main agent call sub_agent twice in parallel
			prompt := fmt.Sprintf(
				`Please evaluate this task by running TWO sub-agents in parallel with DIFFERENT providers:

Task: %s

Call sub_agent TWICE simultaneously with:
1. provider: "%s" (default)
2. provider: "%s" (alternative)

After both complete, summarize each agent's response, then compare: which was more accurate, thorough, or concise? Note any interesting differences in approach.`,
				task, names[0], names[1],
			)
			dm.InputChan <- prompt
			return nil
		},
	})

	// --- Ensemble commands ---

	var ensembleCounter int

	reg.Register(Command{
		Name:        "/ensemble",
		Description: "Start ensemble group chat (/ensemble [preset|auto] <prompt>)",
		Handler: func(args string) error {
			args = strings.TrimSpace(args)
			if args == "" {
				msgChan <- display.StatusMsg{Text: "Usage: /ensemble [preset-name|auto] <prompt>\n  auto: AI casts agents dynamically\n  <preset>: use a named preset from config\n  If no mode specified, uses default agents"}
				return nil
			}

			cfg := ag.Config()
			providers := ag.Providers()

			// Parse mode and prompt
			parts := strings.SplitN(args, " ", 2)
			mode := parts[0]
			prompt := ""
			if len(parts) > 1 {
				prompt = parts[1]
			}

			var agents []ensemble.AgentSpec

			// Check if mode is a preset name
			for _, preset := range cfg.EnsemblePresets {
				if strings.EqualFold(preset.Name, mode) {
					for _, ad := range preset.Agents {
						agents = append(agents, ensemble.AgentSpec{
							Name:        ad.Name,
							Personality: ad.Personality,
							Model:       ad.Model,
							Provider:    ad.Provider,
							Color:       ad.Color,
						})
					}
					if prompt == "" {
						msgChan <- display.StatusMsg{Text: fmt.Sprintf("Preset %q loaded with %d agents. Add a prompt: /ensemble %s <your question>", mode, len(agents), mode)}
						return nil
					}
					break
				}
			}

			// Auto-cast mode
			if agents == nil && strings.EqualFold(mode, "auto") {
				if prompt == "" {
					msgChan <- display.StatusMsg{Text: "Usage: /ensemble auto <prompt>"}
					return nil
				}
				msgChan <- display.StatusMsg{Text: "Casting agents for your prompt..."}

				go func() {
					castCtx := context.Background()
					cast, err := ensemble.CastAgents(
						castCtx,
						providers.Default(),
						cfg.Model,
						prompt,
						providers.Names(),
						nil,
					)
					if err != nil {
						msgChan <- display.ErrorMsg{Err: fmt.Errorf("ensemble casting failed: %w", err)}
						return
					}
					startEnsemble(cast, prompt, cfg, providers, msgChan, &ensembleCounter)
				}()
				return nil
			}

			// Default agents (no preset matched, not auto)
			if agents == nil {
				// The entire args is the prompt, use default agents
				prompt = args
				agents = ensemble.DefaultPresets()
			}

			go startEnsemble(agents, prompt, cfg, providers, msgChan, &ensembleCounter)
			return nil
		},
	})

	reg.Register(Command{
		Name:        "/ensembles",
		Description: "List available ensemble presets",
		Handler: func(args string) error {
			cfg := ag.Config()
			if len(cfg.EnsemblePresets) == 0 {
				msgChan <- display.StatusMsg{Text: "No ensemble presets configured.\nUsage: /ensemble auto <prompt> — auto-cast agents\n       /ensemble <prompt> — use default agents (Sage, Spark, Sentinel)\n\nAdd presets to config.yaml under ensemble_presets:"}
				return nil
			}
			var b strings.Builder
			b.WriteString("Ensemble presets:\n")
			for _, p := range cfg.EnsemblePresets {
				b.WriteString(fmt.Sprintf("\n  %s (%d agents):\n", p.Name, len(p.Agents)))
				for _, a := range p.Agents {
					b.WriteString(fmt.Sprintf("    - %s", a.Name))
					if a.Model != "" {
						b.WriteString(fmt.Sprintf(" [%s]", a.Model))
					}
					if a.Provider != "" {
						b.WriteString(fmt.Sprintf(" via %s", a.Provider))
					}
					b.WriteString("\n")
				}
			}
			b.WriteString("\nUsage: /ensemble <preset-name> <prompt>")
			msgChan <- display.StatusMsg{Text: b.String()}
			return nil
		},
	})
}

// startEnsemble creates and runs an ensemble session.
func startEnsemble(
	agents []ensemble.AgentSpec,
	prompt string,
	cfg *config.Config,
	providers *api.ProviderRegistry,
	msgChan chan tea.Msg,
	counter *int,
) {
	*counter++
	tabID := fmt.Sprintf("ens-%d", *counter)
	userChan := make(chan string, 1)

	ens := ensemble.New(agents, providers, cfg, msgChan, userChan, tabID)

	// Send start message to create the tab — pass userChan so the TUI routes input to us
	msgChan <- display.EnsembleStartMsg{
		TabID:    tabID,
		Agents:   ens.AgentInfos(),
		Prompt:   prompt,
		UserChan: userChan,
	}

	// Run the ensemble — this blocks until done
	ctx := context.Background()
	ens.Run(ctx, prompt)
}
