# cclient3

A fast, clean terminal AI agent powered by Claude — with real-time streaming, adaptive thinking, parallel tool execution, dynamic sub-agent workflows, and multi-provider support (Anthropic / OpenAI / Ollama).

![cclient3 demo](assets/demo.gif)

---

## What is this?

`cclient3` is a terminal-native AI agent client for [Claude](https://www.anthropic.com/claude). It gives Claude real tools — bash, file I/O, grep, glob, web search — and executes them in parallel while streaming responses live to a rich, themed TUI.

Think of it as a local Claude Code you can hack on yourself.

---

## Features

### Modern model support
- **Latest Claude models** — defaults to `claude-opus-4-8`; works with `claude-sonnet-5`, `claude-sonnet-4-6`, `claude-haiku-4-5`, and `claude-fable-5`
- **Adaptive thinking** — automatically enabled on models that support it; the model decides when and how much to reason, with summarized reasoning streamed to the TUI
- **Effort control** — set `effort: low|medium|high|xhigh|max` in config to trade cost against reasoning depth (stripped automatically on models that don't support it)
- **Model-aware requests** — parameters the target model would reject (e.g. sampling params on Opus 4.7+) are never sent
- **Model validation** — the configured model is checked against the live `/v1/models` API at startup, with auto-correction to the nearest match

### Dynamic workflows
- **Parallel sub-agents** — the agent decomposes big tasks into concurrent workstreams via the `sub_agent` tool; each sub-agent streams into its own tab
- **Sub-sub-agents** — sub-agents can delegate one further level down (max depth 2), so very large tasks decompose into a shallow tree of parallel workers; the leaf level is tool-only
- **Effort-tiered routing** — sub-tasks are routed to the cheapest capable model: `fast` shorthand dispatches to the configured `fast_model` (default `claude-haiku-4-5`) for parsing/summarising/light research, while hard reasoning stays on the default model
- **Cross-provider delegation** — sub-agents can run on a different backend (`provider: ollama` for local inference, `provider: openai`)
- **Parallel tool execution** — independent tool calls in one response run concurrently via a bounded semaphore pool
- **Safe context trimming** — long conversations are trimmed to the window without ever orphaning a tool result from its tool call

### Terminal UI
- **Real-time streaming** — responses appear token by token as they're generated
- **Tab management** — Chat, Agent, and Bash tabs with `Alt+1-9` switching
- **9 built-in themes** — `cyber` (default), `ocean`, `ember`, `mono`, `forest`, `nord`, `gruvbox`, `rose`, `dracula`
- **Markdown rendering** — code blocks, headers, lists, all beautifully rendered per-theme
- **Token meter** — live display of input / output / cache tokens + cumulative session cost
- **Confirmation prompts** — risky bash commands require an explicit y/n

### Multi-Provider Support
- **Anthropic** — Claude models via the Anthropic API (default)
- **OpenAI** — GPT models via the OpenAI API (`/v1/chat/completions` + `/v1/models`)
- **Ollama** — local models, no API key required
- **Provider switching** — `/provider openai` or `/provider ollama` to switch defaults at runtime; the request model switches with it automatically
- **Anthropic key optional** — run fully local with `default_provider: ollama` and no `ANTHROPIC_API_KEY`

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
| `web_search` | Search the web via DuckDuckGo (no API key required) |
| `web_fetch` | Fetch and extract readable content from any URL |
| `sub_agent` | Spawn autonomous parallel sub-agents (any provider/model) |

### Slash Commands
| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
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

### Two Modes
- **Interactive TUI** — rich full-screen terminal UI (default)
- **Single-turn CLI** — `cclient3 -p "your prompt"` for scripting and automation

---

## Installation

### Prerequisites
- Go 1.21+
- `ANTHROPIC_API_KEY` environment variable set (not required if `default_provider` is `ollama`/`openai`)
- (Optional) `OPENAI_API_KEY` for the OpenAI provider
- (Optional) Ollama running locally for local inference

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
./cclient3 --prompt "what files are in the current directory?" --model claude-sonnet-5
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
model: claude-opus-4-8         # Default model (most capable)
fast_model: claude-haiku-4-5   # Cheap/fast model for simple sub-tasks
max_tokens: 16384              # Max response tokens
# effort: high                 # low | medium | high | xhigh | max
theme: cyber                   # Default theme: cyber | ocean | ember | mono | ...
max_tool_concurrency: 16       # Max parallel tool calls
bash_timeout: 120              # Bash command timeout (seconds)
api_endpoint: https://api.anthropic.com/v1/messages
# context_size: 1000000        # Context window used for trimming (set ~8192 for Ollama)

# Multi-provider support
ollama_endpoint: http://localhost:11434
ollama_model: qwen3.6:27b      # best local coding model for a 24GB card
openai_endpoint: https://api.openai.com
openai_model: gpt-5.5
default_provider: anthropic    # anthropic | openai | ollama

system_prompt: |
  You are a powerful AI agent with access to tools...
```

**Environment variable overrides:**

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Your Anthropic API key (required for the anthropic provider) |
| `OPENAI_API_KEY` | Optional — enables OpenAI provider (`openai`) |
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
| `forest` | Earthy greens and natural tones |
| `nord` | Arctic, bluish color palette |
| `gruvbox` | Retro warm browns and muted colors |
| `rose` | Soft pinks and rose tones |
| `dracula` | Classic dark purple, pink, and green |

Themes control terminal colors, panel borders, and glamour markdown styles. Add your own by dropping a JSON file in `internal/display/styles/`.

---

## Architecture

```
cclient3/
├── cmd/cclient3/         Entry point, CLI flags, mode selection
├── internal/
│   ├── agent/            Agent loop, conversation history, sub-agents, tool orchestration
│   ├── api/              Anthropic + OpenAI + Ollama providers, SSE streaming, types
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
- Sub-agents run as parallel tool executions, each streaming into its own tab
- All agent → display communication is via typed message channels

**Dynamic workflow flow:**

```
User task
    ↓
Main agent (claude-opus-4-8, adaptive thinking)
    ↓ decomposes
  ┌─────────────┬─────────────┐
  ↓             ↓             ↓
sub_agent     sub_agent     sub_agent      (parallel tool calls)
"fast" model  default       provider:ollama
  ↓             ↓             ↓
own tab      ┌─┴─┐          own tab        (concurrent streaming)
             ↓   ↓
        sub-sub  sub-sub                   (optional depth-2 delegation;
        agent    agent                      leaf level is tool-only)
  └─────────────┴─────────────┘
    ↓ results return as tool_results
Main agent synthesizes → final answer
```

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
- [ ] **Image / vision support** — pass screenshots and images into the conversation
- [ ] **Session persistence** — save and resume conversations across restarts
- [ ] **Server-side web search/fetch** — use Anthropic's hosted `web_search`/`web_fetch` tools

### Medium-term
- [ ] **MCP server support** — connect to any Model Context Protocol tool server
- [ ] **Plugin system** — drop-in tools without recompiling
- [ ] **Diff viewer** — pretty inline diffs for file edits
- [ ] **Compaction** — server-side context summarization for very long sessions

### Longer-term
- [ ] **TUI multiplexer** — split-pane layout for agent + file explorer + shell
- [ ] **Workspace snapshots** — checkpoint and rollback file system state
- [ ] **Structured output mode** — JSON schema-constrained responses

---

## License

MIT

---

*Built with [Bubbletea](https://github.com/charmbracelet/bubbletea), [Glamour](https://github.com/charmbracelet/glamour), and [Lipgloss](https://github.com/charmbracelet/lipgloss) by the Charm team.*
