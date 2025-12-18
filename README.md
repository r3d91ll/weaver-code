# Weaver Code

"Shannon's theory has so penetratingly cleared the air that one is now, perhaps for the first time, ready for a real theory of meaning."
— Warren Weaver, introduction to The Mathematical Theory of Communication by Claude E. Shannon and Warren Weaver (1949)

**Give Claude a Junior Engineer** - delegate tasks to local models.

Claude Code manages a local model assistant (Junior Engineer) for parallel development, code generation, and task delegation.

## Concept

- All your messages go to Claude (Senior Engineer)
- Claude decides when to delegate to the local model
- Local model responses automatically return to Claude for review
- Parallel branch work: Junior on one branch, Claude on another

## Installation

```bash
poetry install
```

## Usage

### Interactive Mode

```bash
poetry run weaver
```

### CLI Options

```bash
weaver                          # Interactive mode
weaver "Write hello world"      # Quick query
echo "Query" | weaver -p        # Pipe mode
weaver --local-only             # Only use local model
weaver --claude-only            # Only use Claude Code
```

### In-Session Commands

```
/agents         List available agents
/memory         Show shared memory status
/memory list    List shared notepad
/history        Show conversation history
/clear          Clear conversation history
/help           Show help
/quit           Exit
```

## Architecture

```
You -> Claude Code (Senior Engineer)
           |
           v
       [Routes/Delegates]
           |
           v
       Local Model (Junior Engineer)
           |
           v
       [Auto-returns to Claude]
           |
           v
       Claude reviews/continues
```

## Requirements

- Claude CLI (`claude`) installed and authenticated
- Local model server (LM Studio at localhost:1234)
- Python 3.11+

## Configuration

Default local model endpoint: `http://localhost:1234/v1` (LM Studio default)

Override with:
```bash
weaver --local-url http://localhost:8080/v1 --local-model my-model
```

## Shared Memory

Claude and the local model share a notepad for coordination:

- Claude has MCP tools: `write_shared`, `read_shared`, `list_shared`
- Local model gets memory context injected into prompts
- Falls back to local JSON storage when ArangoDB unavailable

## License

MIT
