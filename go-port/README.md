# Weaver Code - Go Edition

Single-binary CLI for orchestrating Claude Code (Senior Engineer) with local models (Junior Engineer).

## Features

- **Single binary**: No runtime dependencies, cross-compiles to Linux/macOS/Windows
- **Auto-detection**: Automatically finds running local model services (LM Studio, Ollama, vLLM, LocalAI)
- **Senior/Junior delegation**: Claude handles complex tasks, delegates simple tasks to local LLM
- **Junior Assessment**: Built-in coding challenges to evaluate local model capabilities
- **Phoenix Tracing**: Native Arize Phoenix integration for LLM observability
- **Context management**: Automatic compaction via self-summarization at 80% capacity
- **Shared memory**: JSON-based notepad at `~/.weaver/shared.json`

## Quick Start

```bash
# Build
make build

# Run (auto-detects local models)
./weaver

# With Phoenix tracing
./weaver --trace my-project --trace-endpoint localhost:6006
```

## Build

```bash
# Build for current platform
make build

# Cross-compile for all platforms
make cross-compile

# Install to $GOPATH/bin
make install

# Run tests
make test
```

## Usage

```bash
# Interactive mode (auto-detects local models)
weaver

# Single query
weaver -m "Write a hello world function"

# Pipe mode
echo "Explain this error" | weaver

# Skip auto-detection, use specific model
weaver --local-url http://localhost:11434/v1 --local-model llama3.2

# With Phoenix tracing enabled
weaver --trace my-project-name
```

## Phoenix Tracing (LLM Observability)

Weaver has built-in support for [Arize Phoenix](https://github.com/Arize-ai/phoenix), an open-source LLM observability tool. This lets you visualize and debug all LLM calls (both Senior and Junior).

### Setup Phoenix

```bash
# Run Phoenix via Docker
docker run -d -p 6006:6006 arizephoenix/phoenix:latest

# Open dashboard
open http://localhost:6006
```

### Enable Tracing

```bash
# Traces go to the specified project
weaver --trace my-project-name

# Custom endpoint (default: localhost:6006)
weaver --trace my-project-name --trace-endpoint phoenix.example.com:6006
```

### What Gets Traced

Every LLM call captures:
- **Input**: Full prompt sent to model
- **Output**: Complete response
- **Tokens**: Estimated token counts (prompt + completion)
- **Latency**: Request duration in milliseconds
- **Model**: Which model handled the request
- **Role**: "senior" (Claude) or "junior" (local model)

### Technical Details

Weaver uses OpenTelemetry (OTLP/HTTP) to send traces. Phoenix routes traces to projects via the `openinference.project.name` resource attribute. Create the project in Phoenix dashboard first, then use `--trace <project-name>`.

## Commands

### Model Management

| Command | Description |
|---------|-------------|
| `/models` | List available models from all detected services |
| `/load <service> <model>` | Load a model (e.g., `/load ollama llama3:8b`) |
| `/junior` | Show current Junior model status |
| `/junior-assessment` | Run coding challenges to evaluate Junior |

### Chat Commands

| Command | Description |
|---------|-------------|
| `/local <msg>` | Send message directly to Junior (bypasses Senior) |
| `/agents` | List available agents and status |
| `/memory` | Show shared notes |
| `/memory clear` | Clear shared notes |
| `/clear` | Clear all conversation history |
| `/clear-senior` | Clear only Senior's context |
| `/clear-junior` | Clear only Junior's context |
| `/help` | Show help |
| `/quit` | Exit |

## Junior Assessment

Evaluate your local model's coding abilities with 15 built-in challenges across 5 categories:

```bash
# Start interactive mode
weaver

# Run assessment
/junior-assessment
```

**Categories tested:**
- Basic Algorithms (FizzBuzz, palindrome, factorial)
- Data Structures (linked list, stack, binary search)
- Code Quality (refactoring, error handling, documentation)
- Real-World Tasks (REST endpoint, SQL query, config parsing)
- Problem Solving (rate limiter, LRU cache, retry logic)

Results are saved to `CLAUDE.md` in your current directory, giving Senior context about Junior's strengths and weaknesses for better delegation decisions.

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `--local-url` | (auto-detected) | Local model API URL |
| `--local-model` | (auto-detected) | Model name for local API |
| `--local-context` | `32000` | Local model context window |
| `--no-detect` | `false` | Skip auto-detection of local models |
| `--trace` | (disabled) | Phoenix project name for tracing |
| `--trace-endpoint` | `localhost:6006` | Phoenix OTLP endpoint |
| `--provider` | `claude_code` | Senior provider (`claude_code` or `anthropic_api`) |

## Auto-Detection

On startup, Weaver scans for running local model services:

| Service | Default URL | Detection |
|---------|-------------|-----------|
| LM Studio | `localhost:1234` | `/v1/models` endpoint |
| Ollama | `localhost:11434` | `/api/tags` endpoint |
| vLLM | `localhost:8000` | `/v1/models` endpoint |
| LocalAI | `localhost:8080` | `/v1/models` endpoint |

The first available service with a loaded model becomes Junior.

## Architecture

```
go-port/
├── cmd/weaver/           # CLI entry point
├── internal/
│   ├── senior/           # Senior model adapters
│   │   ├── types.go      # Adapter interface
│   │   └── claude.go     # Claude Code CLI wrapper
│   ├── junior/           # Junior model client
│   │   └── model.go      # OpenAI-compatible HTTP client
│   ├── orchestrator/     # Main coordination
│   │   ├── weaver.go     # Routing + delegation logic
│   │   └── prompts.go    # System prompts
│   ├── loader/           # Model service detection
│   │   ├── services.go   # Service definitions
│   │   ├── ollama.go     # Ollama-specific operations
│   │   └── lmstudio.go   # LM Studio operations
│   ├── assessment/       # Junior evaluation
│   │   ├── challenges.go # 15 coding challenges
│   │   ├── assessment.go # Assessment runner
│   │   └── report.go     # CLAUDE.md generation
│   ├── telemetry/        # Phoenix/OTEL integration
│   │   └── telemetry.go  # Trace provider setup
│   ├── context/          # Context window management
│   └── memory/           # Shared notepad
├── go.mod
├── Makefile
└── README.md
```

## Message Flow

```
You> "Write a function to parse JSON"
    │
    ▼
┌─────────────────┐
│  Orchestrator   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌─────────────────┐
│  Senior (Claude)│────▶│  Junior (Local) │
│                 │◀────│                 │
└─────────────────┘     └─────────────────┘
         │
         ▼
Senior> "Here's the function..."
```

1. All user messages go to Senior (Claude)
2. Senior may delegate to Junior via internal `/local` command
3. Junior's response returns to Senior for review
4. Senior provides final response to user

## Context Compaction

When context reaches 80% of limit:

1. Agent asked to summarize conversation
2. Summary preserved: decisions, task state, file references
3. Context reset with summary as starting point

For small-context local models (<32k), messages are truncated instead.

## Requirements

- **Claude CLI**: `claude` installed and authenticated ([install guide](https://docs.anthropic.com/en/docs/claude-code))
- **Local model server**: One of:
  - [LM Studio](https://lmstudio.ai/) (recommended for beginners)
  - [Ollama](https://ollama.ai/)
  - [vLLM](https://github.com/vllm-project/vllm)
  - [LocalAI](https://localai.io/)
- **Phoenix** (optional): For tracing - `docker run -p 6006:6006 arizephoenix/phoenix:latest`

## License

MIT
