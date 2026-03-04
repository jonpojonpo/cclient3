# cclient3

A fast, beautiful terminal AI agent powered by Claude — with real-time streaming, parallel tool execution, multi-provider support, and **ensemble mode** for multi-agent group chat.

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

> **Generate the demo GIF**: `vhs demo.tape` (requires [VHS](https://github.com/charmbracelet/vhs))

---

## What is this?

`cclient3` is a terminal-native AI agent client for [Claude](https://www.anthropic.com/claude). It gives Claude real tools — bash, file I/O, grep, glob — and executes them in parallel while streaming responses live to a rich, themed TUI.

Think of it as a local Claude Code you can hack on yourself.

---

## Ensemble Mode

Launch a **multi-agent group chat** where AI agents with distinct personalities discuss your prompt collaboratively — like a Slack channel with AI participants.

```
┌──────────────────────────────────────────────────────────────┐
│ ╭──────────╮╭──────────╮╭──────────╮                         │
│ │ 1: Chat  ││ 2: Ensemble ⠹ ││ 3: bash  │                   │
│ ╰──────────╯╰──────────╯╰──────────╯                         │
│                                                              │
│  Ensemble Group Chat                                         │
│                                                              │
│  Sage (claude-sonnet-4-6 via anthropic)                      │
│    A wise architect who values clean design...               │
│  Spark (llama3 via ollama)                                   │
│    A creative innovator who challenges assumptions...        │
│  Sentinel (claude-haiku-4-5-20251001 via anthropic)          │
│    A security-minded skeptic who stress-tests ideas...       │
│  ──────────────────────────────────────────────────────       │
│                                                              │
│  ╭─ You ─────────────────────────────────────────────╮       │
│  │ What are the tradeoffs of microservices vs         │       │
│  │ monoliths for a startup?                           │       │
│  ╰────────────────────────────────────────────────────╯       │
│                                                              │
│  ╭─ Sage ────────────────────────────────────────────╮       │
│  │ For an early-stage startup, I'd strongly advocate  │       │
│  │ starting with a well-structured monolith...        │       │
│  ╰────────────────────────────────────────────────────╯       │
│                                                              │
│  ╭─ Spark ───────────────────────────────────────────╮       │
│  │ Hold on — let's challenge that assumption. What if │       │
│  │ the team already has microservices experience?...   │       │
│  ╰────────────────────────────────────────────────────╯       │
│                                                              │
│  ╭─ Sentinel ────────────────────────────────────────╮       │
│  │ Both of you are overlooking the security surface   │       │
│  │ area. Microservices multiply your attack vectors...│       │
│  ╰────────────────────────────────────────────────────╯       │
│                                                              │
│  ─── Round complete. Type a message to continue. ───         │
│                                                              │
│ ┌────────────────────────────────────────────────────┐       │
│ │ What about developer experience?                    │       │
│ └────────────────────────────────────────────────────┘       │
│ claude-sonnet-4-6     [cyber]     in:4521 out:892 cost:$0.02 │
└──────────────────────────────────────────────────────────────┘
```

### How it works

1. **You provide a prompt** — a question, task, or topic for discussion
2. **Agents respond simultaneously** — all agents receive the shared transcript and respond in parallel
3. **You direct the conversation** — type messages to steer the discussion, ask follow-ups, or challenge points
4. **Diverse perspectives** — agents use different models (Claude + Ollama) with unique personalities
5. **Type `/done` to end** — get a summary and return to normal chat

### Quick start

```bash
# Default agents (Sage, Spark, Sentinel)
/ensemble Design a REST API for a todo app

# AI auto-casts agents based on your topic
/ensemble auto Should we use Rust or Go for our new CLI tool?

# Use a preset from config
/ensemble code-review Review the authentication module

# CLI flag to start directly in ensemble mode
./cclient3 --ensemble auto "Compare React vs Svelte for a dashboard"
```

### Configuring presets

Add custom agent rosters to `~/.config/cclient3/config.yaml`:

```yaml
ensemble_presets:
  - name: code-review
    agents:
      - name: Architect
        personality: "Focuses on system design, API boundaries, and scalability"
        model: claude-sonnet-4-6
        provider: anthropic
        color: "#4ECDC4"
      - name: Pragmatist
        personality: "Values simplicity, shipping fast, and avoiding over-engineering"
        model: llama3
        provider: ollama
        color: "#FF6B6B"
      - name: Security
        personality: "Hunts for vulnerabilities, injection risks, and auth issues"
        model: claude-haiku-4-5-20251001
        provider: anthropic
        color: "#FFE66D"

  - name: brainstorm
    agents:
      - name: Visionary
        personality: "Thinks 10 years ahead, explores moonshot ideas"
        color: "#DDA0DD"
      - name: Critic
        personality: "Devil's advocate — finds flaws and asks hard questions"
        color: "#FFA07A"
      - name: Builder
        personality: "Turns ideas into concrete implementation plans"
        color: "#98FB98"
```

### Default agents

When no preset is specified, ensemble uses three built-in agents:

| Agent | Personality | Color |
|-------|-------------|-------|
| **Sage** | Wise architect — clean design, proven patterns, long-term thinking | Teal |
| **Spark** | Creative innovator — challenges assumptions, unconventional solutions | Red |
| **Sentinel** | Security skeptic — stress-tests ideas, finds edge cases | Yellow |

---

## Features

### Terminal UI
- **Real-time streaming** — responses appear token by token as they're generated
- **Tab management** — Chat, Ensemble, Agent, and Bash tabs with `Alt+1-9` switching
- **4 built-in themes** — `cyber` (default), `ocean`, `ember`, `mono`
- **Markdown rendering** — code blocks, headers, lists, all beautifully rendered per-theme
- **Panel layout** — distinct zones for user input, assistant response, tool activity, and errors
- **Token meter** — live display of input / output / cache tokens + cumulative session cost
- **Spinner feedback** — visual indicator when the agent is thinking or executing tools

### Multi-Provider Support
- **Anthropic** — Claude models via the Anthropic API
- **Ollama** — local models via the OpenAI-compatible endpoint
- **Provider switching** — `/provider ollama` to switch defaults at runtime
- **Per-agent providers** — ensemble agents can each use different providers/models

### Agent Loop
- **Agentic tool use** — Claude can call tools, inspect results, and loop until done
- **Parallel tool execution** — up to 16 tool calls run concurrently via a semaphore pool
- **Sub-agents** — autonomous child agents in their own tabs for parallel workstreams
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
| `/ensemble [preset\|auto] <prompt>` | Start a multi-agent ensemble discussion |
| `/ensembles` | List available ensemble presets |
| `/model <name>` | Switch to a different model |
| `/model list` | List available models from the API |
| `/model next` | Cycle through available models |
| `/provider <name>` | Switch default AI provider |
| `/providers` | List registered providers |
| `/eval <task>` | Run a task on two providers in parallel and compare |
| `/theme <name>` | Switch theme at runtime |
| `/tab <n\|name>` | Switch to a tab by number or name |
| `/tabs` | List open tabs with status |
| `/newtab [name]` | Open a new bash session tab |
| `/closetab [n\|name]` | Close a tab |
| `/skills` | List available skills |
| `/skill <name>` | Toggle a skill on/off |
| `/save [path]` | Save conversation to file |
| `/load [path]` | Load conversation from file |
| `/clear` | Clear conversation history |
| `/stats` | Show token usage statistics |
| `/quit` | Exit (also `Ctrl+C`) |

### Keyboard Shortcuts
| Key | Action |
|-----|--------|
| `Alt+1-9` | Switch to tab by number |
| `Alt+[` / `Alt+]` | Cycle tabs left/right |
| `Alt+←` / `Alt+→` | Cycle tabs left/right |
| `Alt+w` | Close current tab |
| `Alt+p` | Pin/unpin current tab |
| `PgUp` / `PgDown` | Scroll history |
| `Ctrl+J` | Insert newline in input |
| `Enter` | Submit message |
| `Ctrl+C` | Exit |

### Three Modes
- **Interactive TUI** — rich full-screen terminal UI (default)
- **Single-turn CLI** — `cclient3 -p "your prompt"` for scripting and automation
- **Ensemble mode** — `cclient3 --ensemble auto` for multi-agent group chat

---

## Installation

### Prerequisites
- Go 1.21+
- `ANTHROPIC_API_KEY` environment variable set
- (Optional) Ollama running locally for multi-provider ensemble

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

### Ensemble mode

```bash
# Start with auto-cast agents
./cclient3 --ensemble auto

# Start with a preset
./cclient3 --ensemble code-review

# Prompt is provided interactively or via positional args
./cclient3 -e auto "Design a caching strategy"
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--prompt` | `-p` | Single-turn prompt (non-interactive) |
| `--model` | `-m` | Override configured model |
| `--ensemble` | `-e` | Start ensemble mode (auto, preset name) |
| `--theme` | | Override default theme |
| `--version` | `-v` | Print version info |

---

## Configuration

Default config file: `~/.config/cclient3/config.yaml` (falls back to `./config.yaml`)

```yaml
model: claude-sonnet-4-6       # Model to use
max_tokens: 8192               # Max response tokens
temperature: 0.7               # Sampling temperature (0-1)
theme: cyber                   # Default theme: cyber | ocean | ember | mono
max_tool_concurrency: 16       # Max parallel tool calls
bash_timeout: 120              # Bash command timeout (seconds)
api_endpoint: https://api.anthropic.com/v1/messages

# Multi-provider support
ollama_endpoint: http://localhost:11434
default_provider: anthropic    # anthropic | ollama

# Ensemble presets (optional)
ensemble_presets:
  - name: code-review
    agents:
      - name: Architect
        personality: "System design and scalability focus"
        model: claude-sonnet-4-6
        provider: anthropic
        color: "#4ECDC4"
      - name: Pragmatist
        personality: "Ship fast, avoid over-engineering"
        model: llama3
        provider: ollama
        color: "#FF6B6B"

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
│   ├── api/              Anthropic + Ollama providers, SSE streaming, types
│   ├── ensemble/         Ensemble orchestration engine + auto-casting
│   ├── tools/            Tool implementations + parallel executor
│   ├── display/          Bubbletea TUI model, panels, themes, markdown
│   ├── commands/         Slash command registry and built-ins
│   ├── skills/           Loadable skill system (.md files)
│   └── config/           YAML + env config loading
└── pkg/version/          Version metadata (set via ldflags)
```

**Concurrency model:**
- Agent runs in a background goroutine, receiving input via channels
- Bubbletea handles the UI in the main thread
- Tools execute in a bounded semaphore pool (default 16 concurrent)
- Ensemble agents run in parallel goroutines with shared transcript
- All agent/ensemble → display communication is via typed message channels

**Ensemble flow:**

```
User prompt → shared transcript
            ↓
  ┌─────────┼─────────┐
  ↓         ↓         ↓
Agent A   Agent B   Agent C     (parallel goroutines)
  ↓         ↓         ↓
Stream    Stream    Stream      (concurrent to TUI)
  ↓         ↓         ↓
Done      Done      Done
            ↓
  sync.WaitGroup.Wait()
            ↓
  Transcript updated
            ↓
  Wait for user input → next round
```

---

## Demo

A [VHS](https://github.com/charmbracelet/vhs) tape file is included to generate a demo GIF:

```bash
# Install VHS: https://github.com/charmbracelet/vhs#installation
vhs demo.tape
```

This produces `demo.gif` showing the TUI startup, slash commands, and ensemble mode in action.

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
- [ ] **More themes** — dracula, gruvbox, solarized, nord, etc.

### Medium-term
- [ ] **MCP server support** — connect to any Model Context Protocol tool server
- [ ] **Plugin system** — drop-in tools without recompiling
- [x] **~~Multi-agent mode~~** — ~~spawn sub-agents for parallel workstreams~~ **Done!** Ensemble mode
- [x] **~~Local model support~~** — ~~Ollama / llama.cpp backend~~ **Done!** Ollama provider
- [ ] **Diff viewer** — pretty inline diffs for file edits
- [ ] **Token budget warnings** — alert when approaching context limits
- [ ] **Ensemble voting/synthesis** — have agents vote on best approach, auto-synthesize

### Longer-term
- [ ] **TUI multiplexer** — split-pane layout for agent + file explorer + shell
- [ ] **Workspace snapshots** — checkpoint and rollback file system state
- [ ] **Voice input** — whisper integration for spoken prompts
- [ ] **Structured output mode** — JSON schema-constrained responses

---

## License

MIT

---

*Built with [Bubbletea](https://github.com/charmbracelet/bubbletea), [Glamour](https://github.com/charmbracelet/glamour), and [Lipgloss](https://github.com/charmbracelet/lipgloss) by the Charm team.*
