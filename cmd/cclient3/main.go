package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonpo/cclient3/internal/agent"
	"github.com/jonpo/cclient3/internal/api"
	"github.com/jonpo/cclient3/internal/commands"
	"github.com/jonpo/cclient3/internal/config"
	"github.com/jonpo/cclient3/internal/display"
	"github.com/jonpo/cclient3/pkg/version"
)

func main() {
	var (
		prompt      string
		model       string
		theme       string
		ensembleArg string
		showVer     bool
	)

	flag.StringVar(&prompt, "prompt", "", "Single-turn prompt (non-interactive mode)")
	flag.StringVar(&prompt, "p", "", "Single-turn prompt (shorthand)")
	flag.StringVar(&model, "model", "", "Override model name")
	flag.StringVar(&model, "m", "", "Override model name (shorthand)")
	flag.StringVar(&theme, "theme", "", "Override theme (cyber/ocean/ember/mono)")
	flag.StringVar(&ensembleArg, "ensemble", "", "Start ensemble mode (auto, preset name, or empty for defaults)")
	flag.StringVar(&ensembleArg, "e", "", "Start ensemble mode (shorthand)")
	flag.BoolVar(&showVer, "version", false, "Show version")
	flag.BoolVar(&showVer, "v", false, "Show version (shorthand)")
	flag.Parse()

	if showVer {
		fmt.Printf("cclient3 %s (%s) built %s\n", version.Version, version.GitCommit, version.BuildDate)
		os.Exit(0)
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// CLI flag overrides
	if model != "" {
		cfg.Model = model
	}
	if theme != "" {
		cfg.Theme = theme
	}

	// Validate API key
	if cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "Error: ANTHROPIC_API_KEY not set")
		fmt.Fprintln(os.Stderr, "Set it with: export ANTHROPIC_API_KEY=your-key-here")
		os.Exit(1)
	}

	// Context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Single-turn mode — validate synchronously since we need the correct model
	if prompt != "" {
		validateModel(ctx, cfg)
		runSingleTurn(ctx, cfg, prompt)
		return
	}

	// Interactive TUI mode — validate in background to avoid startup latency
	go validateModel(ctx, cfg)
	runInteractive(ctx, cfg, ensembleArg)
}

// buildProviders constructs the provider registry from config.
func buildProviders(cfg *config.Config) *api.ProviderRegistry {
	anthropic := api.NewClient(cfg.APIKey, cfg.APIEndpoint)
	registry := api.NewProviderRegistry(anthropic)

	// Register Ollama if endpoint is configured (always register; it's free to try)
	if cfg.OllamaEndpoint != "" {
		registry.Register(api.NewOllamaProvider(cfg.OllamaEndpoint))
	}
	if cfg.OpenAIAPIKey != "" {
		registry.Register(api.NewOpenAIProvider(cfg.OpenAIAPIKey, cfg.OpenAIEndpoint))
	}

	// Switch default provider if configured
	if cfg.DefaultProvider != "" && cfg.DefaultProvider != "anthropic" {
		registry.SetDefault(cfg.DefaultProvider)
	}

	return registry
}

func runSingleTurn(ctx context.Context, cfg *config.Config, prompt string) {
	msgChan := make(chan tea.Msg, 100)
	providers := buildProviders(cfg)
	ag := agent.NewAgent(cfg, providers, msgChan)
	defer ag.Shutdown()

	text, err := ag.RunSingleTurn(ctx, prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(text)
}

func runInteractive(ctx context.Context, cfg *config.Config, ensembleArg string) {
	// Create display model
	m := display.NewModel(cfg.Theme, cfg.Model)

	// Build provider registry and create agent
	providers := buildProviders(cfg)
	ag := agent.NewAgent(cfg, providers, m.AgentMsgChan)

	// Wire the confirm channel: display writes to it, agent reads from it
	go func() {
		for v := range m.ConfirmChan {
			ag.ConfirmChan() <- v
		}
	}()

	// Create command registry
	cmdReg := commands.NewRegistry()
	commands.RegisterBuiltins(cmdReg, ag, m.AgentMsgChan, m)

	// If --ensemble flag was given, inject the command after startup
	if ensembleArg != "" {
		go func() {
			// Build the /ensemble command from the flag + any remaining args
			ensembleCmd := "/ensemble " + ensembleArg
			// Include any positional args as the prompt
			remaining := flag.Args()
			if len(remaining) > 0 {
				ensembleCmd += " " + strings.Join(remaining, " ")
			}
			m.InputChan <- ensembleCmd
		}()
	}

	// Agent goroutine: reads from InputChan, processes messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case input := <-m.InputChan:
				// Check for commands
				isCmd, err := cmdReg.Execute(input)
				if isCmd {
					if err != nil {
						m.AgentMsgChan <- display.ErrorMsg{Err: err}
					}
					continue
				}

				// Regular message — run agent loop
				if err := ag.Run(ctx, input); err != nil {
					if ctx.Err() != nil {
						return
					}
					m.AgentMsgChan <- display.ErrorMsg{Err: err}
				}
			}
		}
	}()

	// Run bubbletea
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	ag.Shutdown()
}

// validateModel checks the configured model against the API and auto-corrects if possible.
func validateModel(ctx context.Context, cfg *config.Config) {
	client := api.NewClient(cfg.APIKey, cfg.APIEndpoint)

	match, suggestions, err := client.ValidateModel(ctx, cfg.Model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not validate model: %v\n", err)
		return
	}

	if match != nil {
		// Model is valid
		return
	}

	// Model not found
	fmt.Fprintf(os.Stderr, "Warning: model %q not found in API\n", cfg.Model)

	if len(suggestions) > 0 {
		// Auto-select the first suggestion (most likely the right one)
		fmt.Fprintf(os.Stderr, "  Auto-selecting: %s (%s)\n", suggestions[0].ID, suggestions[0].DisplayName)
		cfg.Model = suggestions[0].ID
	} else {
		fmt.Fprintf(os.Stderr, "  No similar models found. Use /models to list available models.\n")
		fmt.Fprintf(os.Stderr, "  Proceeding with %q — API calls may fail.\n", cfg.Model)
	}
}
