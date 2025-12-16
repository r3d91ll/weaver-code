# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Weaver Code** is an AI orchestration CLI that coordinates Claude Code (Senior Engineer) with a local model (Junior Engineer) for efficient development workflows. Users talk to Claude; Claude delegates simple tasks to the local model; local model responses return to Claude for review.

```
YOU --> CLAUDE CODE --> (delegates) --> LOCAL MODEL
             ^                              |
             +------- (reviews) ------------+
```

## Active Development: Go Port

The primary implementation is now in `go-port/`. This is a single-binary Go CLI with:
- Auto-detection of local model services (LM Studio, Ollama, vLLM, LocalAI)
- Junior Assessment (15 coding challenges to evaluate local models)
- Phoenix/OpenTelemetry tracing for LLM observability
- No runtime dependencies

## Development Commands (Go Port)

```bash
cd go-port

# Build
make build                           # Build for current platform
make cross-compile                   # Build for all platforms
make test                            # Run tests

# Run
./weaver                             # Interactive mode (auto-detects models)
./weaver -m "Write hello"            # Single query
echo "Query" | ./weaver              # Pipe mode

# With Phoenix tracing
./weaver --trace my-project --trace-endpoint localhost:6006

# Custom local model
./weaver --local-url http://localhost:11434/v1 --local-model llama3.2
```

## Legacy Python Version

The original Python implementation is in the root directory. Commands:

```bash
poetry install
poetry run weaver                    # Interactive mode
poetry run pytest                    # Run tests
poetry run mypy src --pretty         # Type checking
```

## Architecture

### Core Components

| Module | Purpose |
|--------|---------|
| `cli.py` | Rich terminal interface with slash commands |
| `orchestrator.py` | Main `Weaver` class coordinating agents and routing |
| `agents.py` | `ClaudeCodeAgent` (subprocess) and `LocalModelAgent` (HTTP) |
| `router.py` | Message routing - all user messages go to Claude, `/local` goes to Junior |
| `conversation.py` | Conversation history with agent attribution |
| `memory.py` | Shared notepad (JSON file at `~/.weaver/`) |
| `prompts.py` | System prompts defining Senior/Junior Engineer roles |
| `mcp_memory.py` | MCP server for Claude to access shared memory |

### Data Flow

1. User message enters through `cli.py`
2. `orchestrator.py` routes through `router.py` (always to Claude)
3. `ClaudeCodeAgent` spawns `claude` CLI subprocess
4. If Claude's response contains `/local <task>`, orchestrator sends to `LocalModelAgent`
5. Local model response auto-returns to Claude for review
6. Final response displayed to user

### Agent Types

```python
class AgentType(Enum):
    CLAUDE_CODE = "claude_code"    # Primary - handles all user messages
    LOCAL_FAST = "local_fast"      # Junior Engineer - delegated tasks
    LOCAL_SPECIALIZED = "local_specialized"  # Domain-specific (unused)
```

### Configuration

- Default local endpoint: `http://localhost:1234/v1` (LM Studio)
- Shared notes stored at: `~/.weaver/shared_notes.json`
- Conversation history in-memory, exportable via `/export`

## Key Patterns

### Claude Code Agent

Spawns `claude` CLI as subprocess with JSON streaming:
```python
cmd = ["claude", "-p", "--verbose", "--output-format", "stream-json", "--dangerously-skip-permissions"]
```

### Local Model Agent

Standard OpenAI-compatible HTTP client using `httpx`:
```python
await client.stream("POST", "/chat/completions", json={...})
```

### Memory Injection

Local models receive shared memory context via prompt injection (no MCP access):
```python
memory_context = self._get_memory_context_for_local()
effective_message = f"{memory_context}\n---\n\n{message}"
```

## In-Session Commands

| Command | Description |
|---------|-------------|
| `/agents` | List available agents and status |
| `/memory` | Show shared memory status |
| `/memory list` | List notes in shared notepad |
| `/memory write <text>` | Write to shared notepad |
| `/memory read <id>` | Read a note |
| `/history` | Show conversation history |
| `/clear` | Clear conversation |
| `/help` | Show help |
| `/quit` | Exit |

## Phoenix Tracing (LLM Observability)

Weaver integrates with [Arize Phoenix](https://github.com/Arize-ai/phoenix) for LLM observability.

```bash
# Start Phoenix
docker run -d -p 6006:6006 arizephoenix/phoenix:latest

# Run Weaver with tracing (Go port)
cd go-port
./weaver --trace my-project-name

# View traces at http://localhost:6006
```

**Technical note**: Phoenix routes traces via the `openinference.project.name` resource attribute (not HTTP headers). Create the project in Phoenix dashboard first.

## Dependencies (Python - Legacy)

- **httpx**: HTTP client for local model API calls
- **rich**: Terminal UI (panels, tables, markdown rendering)
- **pydantic**: Data validation
- **opentelemetry**: Tracing support (Arize Phoenix)

## Testing

```bash
poetry run pytest                              # All tests
poetry run pytest tests/test_file.py           # Single file
poetry run pytest tests/test_file.py::test_fn  # Single test
poetry run pytest -v                           # Verbose output
```

Test configuration in `pyproject.toml` uses `asyncio_mode = "auto"` for async tests.
