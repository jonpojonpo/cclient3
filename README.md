# cclient3

A fast, beautiful terminal AI agent powered by Claude — with real-time streaming, parallel tool execution, and a gorgeous TUI.

```
╔═══════════════════════════════════════════════════════════════╗
║   ██████╗ ██████╗██╗     ██╗███████╗███╗   ██╗████████╗      ║
║  ██╔════╝██╔════╝██║     ██║██╔════╝████╗  ██║╚══██╔══╝      ║
║  ██║     ██║     ██║     ██║█████╗  ██╔██╗ ██║   ██║         ║
║  ██║     ██║     ██║     ██║██╔══╝  ██║╚██╗██║   ██║         ║
║  ╚██████╗╚██████╗███████╗██║███████╗██║ ╚████║   ██║         ║
║   ╚═════╝ ╚═════╝╚══════╝╚═╝╚══════╝╚═╝  ╚═══╝   ╚═╝   3    ║
╚═══════════════════════════════════════════════════════════════╝
```

---

## What is this?

`cclient3` is a terminal-native AI agent client for [Claude](https://www.anthropic.com/claude). It gives Claude real tools — bash, file I/O, grep, glob — and executes them in parallel while streaming responses live to a rich, themed TUI.

Think of it as a local Claude Code you can hack on yourself.

---

## Features

### Terminal UI
- **Real-time streaming** — responses appear token by token as they're generated
- **4 built-in themes** — `cyber` (default), `ocean`, `ember`, `mono`
- **Markdown rendering** — code blocks, headers, lists, all beautifully rendered per-theme
- **Panel layout** — distinct zones for user input, assistant response, tool activity, and errors
- **Token meter** — live display of input / output / cache tokens in the status bar
- **Spinner feedback** — visual indicator when the agent is thinking or executing tools

### Agent Loop
- **Agentic tool use** — Claude can call tools, inspect results, and loop until done
- **Parallel tool execution** — up to 16 tool calls run concurrently via a semaphore pool
- **Conversation memory** — full multi-turn history maintained across the session

### Prompt Caching
- **3-breakpoint cache strategy** — system prompt, tool definitions, and prior messages are cached
- **Automatic cache management** — stays within Anthropic's 4-breakpoint API limit
- **Token savings** — cache hits shown live in the status bar

### Tools Available to Claude
| Tool | Description |
|------|-------------|
| `bash` | Execute shell commands (configurable timeout) |
| `file_read` | Read file contents with offset/limit support |
| `file_write` | Create or overwrite files |
| `file_edit` | Edit specific sections of a file |
| `glob` | Find files matching glob patterns |
| `grep` | Search file contents with regex |

### Slash Commands
| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/model <name>` | Switch to a different Claude model |
| `/model list` | List available models from the API |
| `/model next` | Cycle through available models |
| `/theme <name>` | Switch theme at runtime |
| `/clear` | Clear conversation history |
| `/quit` | Exit (also `Ctrl+C`) |

### Two Modes
- **Interactive TUI** — rich full-screen terminal UI (default)
- **Single-turn CLI** — `cclient3 -p "your prompt"` for scripting and automation

---

## Installation

### Prerequisites
- Go 1.21+
- `ANTHROPIC_API_KEY` environment variable set

### Build from source

```bash
git clone https://github.com/jonpojonpo/cclient3
cd cclient3
make build
```

The binary lands at `./cclient3`.

For a static binary (no CGO):

```bash
make static
```

Cross-compile for other platforms:

```bash
make build-all   # linux + macOS ARM64 + windows
```

---

## Usage

### Interactive mode

```bash
export ANTHROPIC_API_KEY=your_key_here
./cclient3
```

### Single-turn mode

```bash
./cclient3 -p "summarize this repo for me"
./cclient3 --prompt "what files are in the current directory?" --model claude-opus-4-6
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--prompt` | `-p` | Single-turn prompt (non-interactive) |
| `--model` | `-m` | Override configured model |
| `--theme` | | Override default theme |
| `--version` | `-v` | Print version info |

---

## Configuration

Default config file: `~/.config/cclient3/config.yaml` (falls back to `./config.yaml`)

```yaml
model: claude-sonnet-4-6       # Model to use
max_tokens: 8192               # Max response tokens
temperature: 0.7               # Sampling temperature (0–1)
theme: cyber                   # Default theme: cyber | ocean | ember | mono
max_tool_concurrency: 16       # Max parallel tool calls
bash_timeout: 120              # Bash command timeout (seconds)
api_endpoint: https://api.anthropic.com/v1/messages
system_prompt: |
  You are a helpful AI assistant with access to tools...
```

**Environment variable overrides:**

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Required — your Anthropic API key |
| `CLAUDE_MODEL` | Override the configured model |

---

## Themes

Switch themes at runtime with `/theme <name>` or set the default in config.

| Theme | Vibe |
|-------|------|
| `cyber` | Neon purple/pink on dark — default |
| `ocean` | Cool blues and teals |
| `ember` | Warm orange and amber tones |
| `mono` | Minimal grayscale |

Themes control terminal colors, panel borders, and glamour markdown styles. Add your own by dropping a JSON file in `internal/display/styles/`.

---

## Architecture

```
cclient3/
├── cmd/cclient3/         Entry point, CLI flags, mode selection
├── internal/
│   ├── agent/            Agent loop, conversation history, tool orchestration
│   ├── api/              Anthropic HTTP client, SSE streaming parser, types
│   ├── tools/            Tool implementations + parallel executor
│   ├── display/          Bubbletea TUI model, panels, themes, markdown
│   ├── commands/         Slash command registry and built-ins
│   └── config/           YAML + env config loading
└── pkg/version/          Version metadata (set via ldflags)
```

**Concurrency model:**
- Agent runs in a background goroutine, receiving input via channels
- Bubbletea handles the UI in the main thread
- Tools execute in a bounded semaphore pool (default 16 concurrent)
- All agent → display communication is via typed message channels

---

## Development

```bash
make test      # Run tests with race detector
make lint      # go vet
make fmt       # gofmt
make clean     # Remove build artifacts
```

---

## Roadmap

### Near-term
- [ ] **Web search tool** — give Claude live internet access
- [ ] **Image / vision support** — pass screenshots and images into the conversation
- [ ] **Session persistence** — save and resume conversations across restarts
- [ ] **Conversation export** — save sessions as Markdown or JSON
- [ ] **More themes** — dracula, gruvbox, solarized, nord, etc.
- [ ] **Mouse support** — clickable scrollback, selectable text

### Medium-term
- [ ] **MCP server support** — connect to any Model Context Protocol tool server
- [ ] **Plugin system** — drop-in tools without recompiling
- [ ] **Multi-agent mode** — spawn sub-agents for parallel workstreams
- [ ] **Configurable system prompts** — per-project CLAUDE.md awareness (like Claude Code)
- [ ] **Diff viewer** — pretty inline diffs for file edits
- [ ] **Token budget warnings** — alert when approaching context limits

### Longer-term
- [ ] **TUI multiplexer** — split-pane layout for agent + file explorer + shell
- [ ] **Local model support** — Ollama / llama.cpp backend
- [ ] **Workspace snapshots** — checkpoint and rollback file system state
- [ ] **Voice input** — whisper integration for spoken prompts
- [ ] **Structured output mode** — JSON schema-constrained responses

---

## License

MIT

---

*Built with [Bubbletea](https://github.com/charmbracelet/bubbletea), [Glamour](https://github.com/charmbracelet/glamour), and [Lipgloss](https://github.com/charmbracelet/lipgloss) by the Charm team.*
