# cclient3 — project notes for agents

A terminal AI agent client (Go + Bubbletea) with multi-provider support
(Anthropic / OpenAI / Ollama), parallel tool execution, and dynamic
sub-agent workflows.

## Build & test

```bash
make build     # build ./cclient3
go test ./...  # all tests (make test runs with -race)
make lint      # go vet
gofmt -w <files you touch>
```

## Layout

- `cmd/cclient3` — entry point, flags, mode selection
- `internal/api` — provider clients (anthropic/openai/ollama), SSE parsing, wire types
- `internal/agent` — agent loop, conversation history, sub_agent tool
- `internal/tools` — tool implementations + parallel executor
- `internal/display` — Bubbletea TUI (tabs, themes, markdown, cost meter)
- `internal/commands` — slash commands
- `internal/config` — YAML + env config

## Conventions

- The Anthropic client (`internal/api/client.go`) sanitizes requests per
  model: adaptive thinking is injected where supported, unsupported params
  (effort on Haiku, etc.) are stripped. Update `applyModelDefaults` when new
  model families ship.
- Thinking blocks must be echoed back with their `signature` — don't strip
  them from conversation history.
- Model pricing lives in `internal/display/cost.go`; keep it in sync with
  the models you add.
- Default models: `claude-opus-4-8` (main), `claude-haiku-4-5` (fast_model
  for cheap sub-tasks).
