package commands

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jonpo/cclient3/internal/agent"
	"github.com/jonpo/cclient3/internal/display"
)

// RegisterBuiltins registers all built-in slash commands.
func RegisterBuiltins(reg *Registry, ag *agent.Agent, msgChan chan tea.Msg) {
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
}
