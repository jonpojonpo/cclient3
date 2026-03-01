package commands

import (
	"fmt"
	"strings"
)

type CommandHandler func(args string) error

type Command struct {
	Name        string
	Description string
	Handler     CommandHandler
}

type Registry struct {
	commands map[string]Command
	order    []string
}

func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
	}
}

func (r *Registry) Register(cmd Command) {
	r.commands[cmd.Name] = cmd
	r.order = append(r.order, cmd.Name)
}

func (r *Registry) Execute(input string) (bool, error) {
	if !strings.HasPrefix(input, "/") {
		return false, nil
	}

	parts := strings.SplitN(input, " ", 2)
	name := parts[0]
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	cmd, ok := r.commands[name]
	if !ok {
		return true, fmt.Errorf("unknown command: %s (type /help for available commands)", name)
	}

	return true, cmd.Handler(args)
}

func (r *Registry) Help() string {
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, name := range r.order {
		cmd := r.commands[name]
		b.WriteString(fmt.Sprintf("  %-12s %s\n", cmd.Name, cmd.Description))
	}
	return b.String()
}
