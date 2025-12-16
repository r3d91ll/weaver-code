# Weaver Code Architecture

## What Is Weaver Code?

Weaver Code is an AI orchestration tool that weaves together Claude Code (Senior Engineer) and local models (Junior Engineer) to create efficient development workflows.

**Core Principle:** You talk to Claude. Claude decides when to delegate simple tasks to a local model. Local model responses always return to Claude for review before reaching you.

```
YOU ──→ CLAUDE CODE ──→ (delegates) ──→ LOCAL MODEL
              ↑                              │
              └────── (reviews) ─────────────┘
```

---

## Target Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              CLI (cli.py)                               │
│  - Rich terminal interface                                              │
│  - Commands: /setup, /agents, /memory, /history, /help, /quit          │
│  - Displays agent handoffs visibly                                      │
└───────────────────────────────┬─────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                        ORCHESTRATOR (orchestrator.py)                   │
│                                                                         │
│  Responsibilities:                                                      │
│  1. Route ALL user messages to Claude                                   │
│  2. Inject shared memory context into BOTH agents' prompts              │
│  3. Detect "/local <task>" in Claude's response → route to Junior       │
│  4. Auto-route Junior's response back to Claude for review              │
│  5. Parse natural language notes and store them                         │
│                                                                         │
└────────┬─────────────────┬─────────────────┬─────────────────┬──────────┘
         │                 │                 │                 │
         ▼                 ▼                 ▼                 ▼
┌─────────────┐   ┌─────────────┐   ┌─────────────┐   ┌─────────────────┐
│   Router    │   │Conversation │   │   Memory    │   │     Agents      │
│             │   │  Manager    │   │             │   │                 │
│ All msgs →  │   │             │   │ JSON file   │   │ ClaudeCodeAgent │
│ Claude      │   │ History     │   │ at ~/.weaver│   │ LocalModelAgent │
│             │   │ Export      │   │             │   │                 │
└─────────────┘   └─────────────┘   └─────────────┘   └─────────────────┘
```

### Shared Memory via Prompt Injection

**NO MCP servers.** Both agents receive memory context injected into their prompts by the orchestrator.

```python
# Orchestrator reads shared memory
memory_context = self.memory.get_context_for_prompt()

# Injects into Claude's prompt
claude_prompt = f"""
{memory_context}

---
{user_message}
"""

# Injects into Junior's prompt  
junior_prompt = f"""
{memory_context}

---
Task from Senior Engineer: {task}
"""
```

Memory is stored in `~/.weaver/shared.json`. Both agents can leave notes using natural language:
- "Note for Junior: The API uses JWT tokens"
- "Remember: Config is at src/config.yaml"

The orchestrator parses these patterns and stores them.

### Delegation Flow

```
1. User types message
         │
         ▼
2. Orchestrator injects memory context
         │
         ▼
3. Send to Claude Code (always)
         │
         ▼
4. Claude responds
         │
         ├── No "/local" in response ──→ Return to user
         │
         └── "/local <task>" found ──→ Extract task
                                              │
                                              ▼
                                       5. Inject memory context
                                              │
                                              ▼
                                       6. Send to Local Model
                                              │
                                              ▼
                                       7. Junior responds
                                              │
                                              ▼
                                       8. Format review prompt:
                                          "Junior completed the task.
                                           Here's the response: ...
                                           Please review."
                                              │
                                              ▼
                                       9. Send to Claude for review
                                              │
                                              ▼
                                       10. Claude reviews, responds to user
```

### Agent Types

Only two agent types:

| Type | Role | Implementation |
|------|------|----------------|
| `CLAUDE_CODE` | Senior Engineer | Subprocess spawning `claude` CLI (has built-in tools) |
| `LOCAL` | Junior Engineer | HTTP client + Tool Executor (we provide the tools) |

No `LOCAL_FAST` or `LOCAL_SPECIALIZED` - keep it simple.

### Tool Execution for Junior

**Critical:** Claude Code has file ops, shell, etc. built in. The local model is just text-in/text-out. Without tools, Junior can only *suggest* things, not *do* them.

We provide a **Tool Executor** that:
1. Defines available tools for Junior
2. Parses tool calls from Junior's responses
3. Executes tools safely
4. Returns results to Junior (or Claude for review)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         TOOL EXECUTOR (tools.py)                        │
│                                                                         │
│  Junior's response containing tool calls                                │
│         │                                                               │
│         ▼                                                               │
│  ┌─────────────────┐                                                    │
│  │  Parse tool     │  Extract structured tool calls from response       │
│  │  calls          │  (function calling format or XML tags)             │
│  └────────┬────────┘                                                    │
│           │                                                             │
│           ▼                                                             │
│  ┌─────────────────┐                                                    │
│  │  Validate &     │  Check paths are within workspace                  │
│  │  Sandbox        │  Validate command safety                           │
│  └────────┬────────┘                                                    │
│           │                                                             │
│           ▼                                                             │
│  ┌─────────────────┐                                                    │
│  │  Execute        │  Run the tool, capture output                      │
│  └────────┬────────┘                                                    │
│           │                                                             │
│           ▼                                                             │
│  ┌─────────────────┐                                                    │
│  │  Return result  │  Feed back to Junior or send to Claude for review  │
│  └─────────────────┘                                                    │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Available Tools for Junior

| Tool | Purpose | Parameters |
|------|---------|------------|
| `read_file` | Read file contents | `path: str` |
| `write_file` | Create/overwrite file | `path: str, content: str` |
| `edit_file` | Apply edits to file | `path: str, edits: list[{old: str, new: str}]` |
| `list_directory` | List files in directory | `path: str, recursive: bool = False` |
| `search_files` | Search for pattern in files | `pattern: str, path: str = ".", file_glob: str = "*"` |
| `run_command` | Execute shell command | `command: str, cwd: str = "."` |
| `create_directory` | Create directory | `path: str` |
| `delete_file` | Delete a file | `path: str` |

#### Tool Call Format

Junior uses OpenAI-style function calling if the model supports it. Otherwise, structured XML:

**Option 1: Function Calling (preferred if model supports it)**
```json
{
  "tool_calls": [
    {
      "function": {
        "name": "write_file",
        "arguments": "{\"path\": \"src/utils.py\", \"content\": \"def hello():\\n    return 'world'\"}"
      }
    }
  ]
}
```

**Option 2: XML Tags (fallback for models without function calling)**
```
I'll create the utility file now.

<tool_call>
<name>write_file</name>
<arguments>
{"path": "src/utils.py", "content": "def hello():\n    return 'world'"}
</arguments>
</tool_call>
```

#### Tool Execution Loop

When Junior uses tools, the orchestrator runs a loop:

```
1. Send task to Junior (with tool definitions in system prompt)
         │
         ▼
2. Junior responds (may include tool calls)
         │
         ├── No tool calls ──→ Done, return response
         │
         └── Tool calls found
                   │
                   ▼
3. Execute each tool call
         │
         ▼
4. Format results:
   "Tool: write_file
    Result: Successfully wrote 25 bytes to src/utils.py"
         │
         ▼
5. Send results back to Junior:
   "The tool executed. Here are the results: ..."
         │
         ▼
6. Junior continues (may make more tool calls)
         │
         └── Loop back to step 2 (max iterations to prevent runaway)
```

#### Safety & Sandboxing

- **Path validation**: All file paths must be within the workspace directory
- **Command allowlist**: `run_command` only allows safe commands (configurable)
- **Confirmation prompts**: Destructive operations can require user confirmation
- **Max iterations**: Tool loop has a max iteration limit (default: 10)

```python
ALLOWED_COMMANDS = [
    "ls", "cat", "head", "tail", "grep", "find", "wc",
    "git status", "git diff", "git log",
    "python", "pytest", "pip list",
    "npm test", "npm run",
    "cargo test", "cargo check",
]

BLOCKED_PATTERNS = [
    "rm -rf", "sudo", "> /dev", "| sh", "| bash",
    "curl | ", "wget | ", "chmod 777",
]
```

#### Integration with Orchestrator

The orchestrator handles tool execution for Junior:

```python
async def _delegate_to_junior(self, task: str) -> str:
    """Send task to Junior, handle tool calls, return final response."""
    
    memory_context = self.memory.get_context_for_prompt()
    tool_definitions = self.tool_executor.get_tool_definitions()
    
    enhanced_prompt = f"""
{memory_context}

## Available Tools
{tool_definitions}

---
Task from Senior Engineer: {task}
"""
    
    messages = [{"role": "user", "content": enhanced_prompt}]
    
    for iteration in range(self.max_tool_iterations):
        response = await self.local_agent.chat(messages)
        
        tool_calls = self.tool_executor.parse_tool_calls(response)
        
        if not tool_calls:
            # No more tool calls, Junior is done
            return response
            
        # Execute tools
        results = await self.tool_executor.execute_all(tool_calls)
        
        # Add response and results to conversation
        messages.append({"role": "assistant", "content": response})
        messages.append({"role": "user", "content": f"Tool results:\n{results}"})
    
    return "Max tool iterations reached. " + response
```

---

## File Structure

```
weaver-code/
├── src/weaver/
│   ├── __init__.py       # Package exports
│   ├── agents.py         # ClaudeCodeAgent, LocalModelAgent
│   ├── cli.py            # Terminal interface, /setup command
│   ├── config.py         # Configuration loading/saving
│   ├── conversation.py   # Conversation history management
│   ├── memory.py         # Shared notepad (JSON only)
│   ├── orchestrator.py   # Main Weaver class
│   ├── prompts.py        # System prompts
│   ├── router.py         # Message routing (simplified)
│   ├── setup.py          # Setup wizard logic
│   └── tools.py          # Tool executor for Junior
├── tests/
│   ├── __init__.py
│   ├── test_agents.py
│   ├── test_memory.py
│   ├── test_orchestrator.py
│   └── test_tools.py
├── pyproject.toml
├── README.md
├── LICENSE               # MIT
├── ARCHITECTURE.md       # This file
└── .gitignore
```

---

## Module Specifications

### agents.py

```python
class AgentType(Enum):
    CLAUDE_CODE = "claude_code"
    LOCAL = "local"

class ClaudeCodeAgent:
    """Spawns Claude CLI as subprocess."""
    
    async def chat(self, message: str, history: list = None) -> str:
        """Non-streaming chat."""
        # Uses: claude -p --output-format json
        
    async def chat_stream(self, message: str, history: list = None):
        """Streaming chat - yields chunks."""
        # Uses: claude -p --output-format stream-json

class LocalModelAgent:
    """HTTP client for OpenAI-compatible APIs."""
    
    def __init__(self, base_url: str, model: str):
        # Default: http://localhost:11434 (Ollama)
        # Or: http://localhost:1234 (LM Studio)
        
    async def chat(self, message: str, history: list = None) -> str:
        """Call /v1/chat/completions endpoint."""
```

### memory.py

```python
class SharedMemory:
    """JSON-based shared notepad. No ArangoDB."""
    
    def __init__(self, path: str = "~/.weaver/shared.json"):
        self.path = Path(path).expanduser()
        
    def add_note(self, author: str, content: str, tags: list = None):
        """Add note from 'claude' or 'junior'."""
        
    def get_notes(self, limit: int = 10) -> list[dict]:
        """Get recent notes."""
        
    def get_context_for_prompt(self) -> str:
        """Generate context string for prompt injection."""
        # Returns formatted string like:
        # ## Shared Notes
        # - [claude] The API uses JWT tokens
        # - [junior] Found config at src/config.yaml
        
    def clear(self):
        """Clear all notes."""
```

### orchestrator.py

```python
class Weaver:
    """Main orchestrator - coordinates agents and memory."""
    
    def __init__(self, config: dict = None):
        self.claude_agent = ClaudeCodeAgent()
        self.local_agent = None  # Set up via /setup or config
        self.memory = SharedMemory()
        self.conversation = ConversationManager()
        self.router = Router()
        
    async def chat(self, message: str) -> str:
        """Non-streaming chat with memory injection."""
        memory_context = self.memory.get_context_for_prompt()
        enhanced = f"{memory_context}\n\n---\n{message}"
        
        response = await self.claude_agent.chat(enhanced, self.conversation.history)
        self._extract_notes(response, "claude")
        
        # Check for delegation
        if "/local" in response:
            task = self._extract_local_task(response)
            junior_response = await self._delegate_to_junior(task)
            response = await self._get_claude_review(junior_response)
            
        return response
        
    async def chat_stream(self, message: str):
        """Streaming version - same logic, yields chunks."""
        
    async def _delegate_to_junior(self, task: str) -> str:
        """Send task to local model with memory context."""
        memory_context = self.memory.get_context_for_prompt()
        enhanced = f"{memory_context}\n\n---\nTask from Senior Engineer: {task}"
        
        response = await self.local_agent.chat(enhanced)
        self._extract_notes(response, "junior")
        return response
        
    async def _get_claude_review(self, junior_response: str) -> str:
        """Auto-route Junior's work back to Claude for review."""
        review_prompt = f"""
Junior Engineer completed the delegated task.

## Junior's Response:
{junior_response}

## Your Task:
Review this work and provide the final response to the user.
"""
        return await self.claude_agent.chat(review_prompt)
        
    def _extract_notes(self, response: str, author: str):
        """Parse natural language notes from response."""
        # Patterns: "Note for Junior: ...", "Remember: ...", etc.
```

### router.py

```python
class Router:
    """Simplified router - all user messages go to Claude."""
    
    def route(self, message: str) -> tuple[AgentType, str]:
        """
        Returns (agent_type, reason).
        
        Rules:
        - /local or /junior command → LOCAL (explicit override)
        - Everything else → CLAUDE_CODE
        """
        if message.strip().startswith(("/local", "/junior")):
            return AgentType.LOCAL, "Explicit /local command"
        return AgentType.CLAUDE_CODE, "Default routing to Claude"
```

### config.py

```python
def get_config_path() -> Path:
    """~/.weaver/config.yaml"""
    
def load_config() -> dict:
    """Load config, return defaults if missing."""
    
def save_config(config: dict):
    """Save config to file."""
    
DEFAULT_CONFIG = {
    "version": 1,
    "junior": {
        "service": None,  # "ollama", "lmstudio", "openai_compatible"
        "url": None,
        "model": None,
    },
    "ui": {
        "show_handoffs": True,
    },
    "memory": {
        "path": "~/.weaver/shared.json",
        "max_notes": 50,
    }
}
```

### setup.py

```python
SETUP_WIZARD_PROMPT = """
You are helping the user set up Weaver Code's local model integration.

Guide them through:
1. What local AI service they have (Ollama, LM Studio, other)
2. Detect if service is running using curl/shell commands
3. List available models
4. Optionally test model capabilities
5. Save configuration

Be conversational. Use appropriate commands for the user's OS.
If detection fails, ask for manual URL entry.
"""

async def run_setup_wizard(weaver: Weaver) -> dict:
    """Run interactive setup, return config dict."""
```

### prompts.py

```python
SENIOR_ENGINEER_PROMPT = """
You are a Senior Engineer working with a Junior Engineer (local AI model).

Your responsibilities:
1. Handle complex tasks requiring deep reasoning
2. Delegate simple tasks to Junior using: /local <task>
3. Review Junior's work before presenting to user

Tasks suitable for Junior:
- File searches and grep operations
- Running tests
- Simple code generation (boilerplate, utilities)
- Formatting and linting
- Reading file contents

Tasks you should handle yourself:
- Architecture decisions
- Security-sensitive code
- Complex debugging
- Multi-step reasoning

When delegating, be specific about what you need.

You can leave notes using natural language:
- "Note for Junior: The project uses pytest"
- "Remember: API keys are in .env file"
"""

JUNIOR_ENGINEER_PROMPT = """
You are a Junior Engineer assisting a Senior Engineer.

Your role:
- Execute specific tasks assigned by Senior
- Report results clearly and completely
- Stay focused on the assigned task

You have access to tools for file operations and running commands.
Use them to complete your tasks. When you need to:
- Read a file: use the read_file tool
- Write/create a file: use the write_file tool
- Edit a file: use the edit_file tool
- List files: use the list_directory tool
- Search: use the search_files tool
- Run a command: use the run_command tool

Look for "## Shared Notes" at the start of your prompt for context.

You can leave notes:
- "Note for Senior: Found 3 test files"
- "Remember: The config is at src/config.yaml"

Complete your assigned task. Don't go beyond what was asked.
"""
```

### tools.py

```python
from dataclasses import dataclass
from pathlib import Path
from typing import Any
import json
import subprocess
import re

@dataclass
class ToolCall:
    name: str
    arguments: dict[str, Any]

@dataclass  
class ToolResult:
    tool_name: str
    success: bool
    output: str
    error: str | None = None

class ToolExecutor:
    """Executes tools for the local model (Junior)."""
    
    def __init__(self, workspace: Path = None, config: dict = None):
        self.workspace = workspace or Path.cwd()
        self.config = config or {}
        self.max_file_size = self.config.get("max_file_size", 100_000)  # 100KB
        
    def get_tool_definitions(self) -> str:
        """Return tool definitions for the system prompt."""
        return '''
You have access to these tools:

1. read_file(path: str) - Read contents of a file
2. write_file(path: str, content: str) - Create or overwrite a file
3. edit_file(path: str, edits: list) - Apply edits to a file
   edits format: [{"old": "text to find", "new": "replacement text"}, ...]
4. list_directory(path: str, recursive: bool = false) - List files in directory
5. search_files(pattern: str, path: str = ".", file_glob: str = "*") - Search for pattern
6. run_command(command: str, cwd: str = ".") - Run a shell command
7. create_directory(path: str) - Create a directory
8. delete_file(path: str) - Delete a file

To use a tool, format your response like this:
<tool_call>
<name>tool_name</name>
<arguments>{"param": "value"}</arguments>
</tool_call>

You can make multiple tool calls in one response.
Wait for tool results before continuing.
'''
        
    def parse_tool_calls(self, response: str) -> list[ToolCall]:
        """Extract tool calls from model response."""
        tool_calls = []
        
        # Try OpenAI function calling format first (if present in JSON)
        # Then fall back to XML tags
        
        # XML format: <tool_call><name>...</name><arguments>...</arguments></tool_call>
        pattern = r'<tool_call>\s*<name>(\w+)</name>\s*<arguments>(.*?)</arguments>\s*</tool_call>'
        matches = re.findall(pattern, response, re.DOTALL)
        
        for name, args_str in matches:
            try:
                arguments = json.loads(args_str.strip())
                tool_calls.append(ToolCall(name=name, arguments=arguments))
            except json.JSONDecodeError:
                # Try to parse as simple key=value if JSON fails
                pass
                
        return tool_calls
        
    async def execute_all(self, tool_calls: list[ToolCall]) -> str:
        """Execute all tool calls and return formatted results."""
        results = []
        for tc in tool_calls:
            result = await self.execute(tc)
            results.append(result)
        return self._format_results(results)
        
    async def execute(self, tool_call: ToolCall) -> ToolResult:
        """Execute a single tool call."""
        name = tool_call.name
        args = tool_call.arguments
        
        try:
            if name == "read_file":
                return await self._read_file(args["path"])
            elif name == "write_file":
                return await self._write_file(args["path"], args["content"])
            elif name == "edit_file":
                return await self._edit_file(args["path"], args["edits"])
            elif name == "list_directory":
                return await self._list_directory(args["path"], args.get("recursive", False))
            elif name == "search_files":
                return await self._search_files(args["pattern"], args.get("path", "."), args.get("file_glob", "*"))
            elif name == "run_command":
                return await self._run_command(args["command"], args.get("cwd", "."))
            elif name == "create_directory":
                return await self._create_directory(args["path"])
            elif name == "delete_file":
                return await self._delete_file(args["path"])
            else:
                return ToolResult(name, False, "", f"Unknown tool: {name}")
        except Exception as e:
            return ToolResult(name, False, "", str(e))
            
    def _validate_path(self, path: str) -> Path:
        """Ensure path is within workspace."""
        resolved = (self.workspace / path).resolve()
        if not str(resolved).startswith(str(self.workspace.resolve())):
            raise ValueError(f"Path {path} is outside workspace")
        return resolved
        
    async def _read_file(self, path: str) -> ToolResult:
        resolved = self._validate_path(path)
        if not resolved.exists():
            return ToolResult("read_file", False, "", f"File not found: {path}")
        content = resolved.read_text()
        return ToolResult("read_file", True, content)
        
    async def _write_file(self, path: str, content: str) -> ToolResult:
        resolved = self._validate_path(path)
        resolved.parent.mkdir(parents=True, exist_ok=True)
        resolved.write_text(content)
        return ToolResult("write_file", True, f"Wrote {len(content)} bytes to {path}")
        
    async def _edit_file(self, path: str, edits: list) -> ToolResult:
        resolved = self._validate_path(path)
        if not resolved.exists():
            return ToolResult("edit_file", False, "", f"File not found: {path}")
        content = resolved.read_text()
        for edit in edits:
            if edit["old"] not in content:
                return ToolResult("edit_file", False, "", f"Text not found: {edit['old'][:50]}...")
            content = content.replace(edit["old"], edit["new"], 1)
        resolved.write_text(content)
        return ToolResult("edit_file", True, f"Applied {len(edits)} edit(s) to {path}")
        
    async def _list_directory(self, path: str, recursive: bool) -> ToolResult:
        resolved = self._validate_path(path)
        if not resolved.exists():
            return ToolResult("list_directory", False, "", f"Directory not found: {path}")
        if recursive:
            files = [str(f.relative_to(resolved)) for f in resolved.rglob("*") if f.is_file()]
        else:
            files = [f.name for f in resolved.iterdir()]
        return ToolResult("list_directory", True, "\n".join(sorted(files)))
        
    async def _search_files(self, pattern: str, path: str, file_glob: str) -> ToolResult:
        resolved = self._validate_path(path)
        results = []
        for file_path in resolved.rglob(file_glob):
            if file_path.is_file():
                try:
                    content = file_path.read_text()
                    for i, line in enumerate(content.splitlines(), 1):
                        if pattern in line:
                            rel_path = file_path.relative_to(resolved)
                            results.append(f"{rel_path}:{i}: {line.strip()}")
                except:
                    pass
        return ToolResult("search_files", True, "\n".join(results[:100]))  # Limit results
        
    async def _run_command(self, command: str, cwd: str) -> ToolResult:
        # Safety check
        if self._is_dangerous_command(command):
            return ToolResult("run_command", False, "", f"Command not allowed: {command}")
        resolved_cwd = self._validate_path(cwd)
        result = subprocess.run(
            command, shell=True, cwd=resolved_cwd,
            capture_output=True, text=True, timeout=30
        )
        output = result.stdout + result.stderr
        return ToolResult("run_command", result.returncode == 0, output)
        
    async def _create_directory(self, path: str) -> ToolResult:
        resolved = self._validate_path(path)
        resolved.mkdir(parents=True, exist_ok=True)
        return ToolResult("create_directory", True, f"Created directory: {path}")
        
    async def _delete_file(self, path: str) -> ToolResult:
        resolved = self._validate_path(path)
        if not resolved.exists():
            return ToolResult("delete_file", False, "", f"File not found: {path}")
        if resolved.is_dir():
            return ToolResult("delete_file", False, "", "Cannot delete directory with delete_file")
        resolved.unlink()
        return ToolResult("delete_file", True, f"Deleted: {path}")
        
    def _is_dangerous_command(self, command: str) -> bool:
        """Check if command matches dangerous patterns."""
        dangerous = ["rm -rf", "sudo", "> /dev", "| sh", "| bash", 
                     "curl |", "wget |", "chmod 777", "mkfs", "dd if="]
        return any(d in command.lower() for d in dangerous)
        
    def _format_results(self, results: list[ToolResult]) -> str:
        """Format tool results for the model."""
        lines = []
        for r in results:
            if r.success:
                lines.append(f"✓ {r.tool_name}: {r.output}")
            else:
                lines.append(f"✗ {r.tool_name} failed: {r.error}")
        return "\n".join(lines)
```

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `/setup` | Run configuration wizard (conversational) |
| `/agents` | Show active agents and status |
| `/memory` | Display shared notes |
| `/memory clear` | Clear shared notes |
| `/history` | Show conversation with agent annotations |
| `/local <task>` | Force send task to Junior (bypass Claude) |
| `/clear` | Clear conversation history |
| `/help` | Show available commands |
| `/quit` | Exit |

---

## Configuration

`~/.weaver/config.yaml`

```yaml
version: 1

junior:
  service: ollama
  url: http://localhost:11434
  model: qwen2.5-coder:7b

ui:
  show_handoffs: true

memory:
  path: ~/.weaver/shared.json
  max_notes: 50
```

---

## Dependencies

```toml
[project]
name = "weaver-code"
version = "0.1.0"
description = "AI orchestration - weave local and cloud models together"
requires-python = ">=3.11"
license = {text = "MIT"}

dependencies = [
    "httpx[http2]>=0.27",
    "rich>=13.0",
    "pyyaml>=6.0",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.0",
    "pytest-asyncio>=0.23",
    "ruff>=0.4",
]

[project.scripts]
weaver = "weaver.cli:main"
```

---

## Migration from Old Code

### Files to Port (with modifications)

| Old File | Changes Required |
|----------|------------------|
| `conversation.py` | None - keep as-is |
| `agents.py` | Remove `LOCAL_SPECIALIZED`, remove `LOCAL_FAST` |
| `memory.py` | Remove all ArangoDB code |
| `router.py` | Remove `RoutingRule` class, simplify |
| `prompts.py` | Remove MCP tool references |
| `orchestrator.py` | Remove MCP config, add memory injection to `chat()` |
| `cli.py` | Fix `/claude` prefix bug, add `/setup`, fix docstring |

### Files to Delete (do not port)

| File | Reason |
|------|--------|
| `mcp_memory.py` | No MCP - using prompt injection |

### New Files to Create

| File | Purpose |
|------|---------|
| `config.py` | Configuration loading/saving |
| `setup.py` | Setup wizard logic |
| `tools.py` | Tool executor for Junior (file ops, shell, etc.) |

---

## Implementation Order

1. **Project setup**: pyproject.toml, structure, LICENSE
2. **conversation.py**: Port as-is
3. **memory.py**: Port without ArangoDB
4. **tools.py**: Create new - tool executor for Junior
5. **agents.py**: Port with simplified agent types
6. **router.py**: Port simplified version
7. **prompts.py**: Port without MCP references, add tool instructions for Junior
8. **config.py**: Create new
9. **orchestrator.py**: Port with memory injection fixes + tool execution loop
10. **cli.py**: Port with bug fixes, add /setup
11. **setup.py**: Create new
12. **Tests**: Basic coverage
13. **README.md**: Documentation

---

## Success Criteria

- [ ] `weaver` command starts CLI
- [ ] `/setup` runs conversational wizard
- [ ] Claude can delegate with `/local <task>`
- [ ] Junior can execute tools (read/write files, run commands)
- [ ] Tool execution is sandboxed to workspace
- [ ] Junior's response auto-returns to Claude for review
- [ ] Shared memory works via prompt injection (both agents)
- [ ] Works with Ollama (localhost:11434)
- [ ] Works with LM Studio (localhost:1234)
- [ ] No MCP code anywhere
- [ ] No ArangoDB code anywhere
- [ ] No unused dependencies

---

## Brand Note

**Weaver Code** is named for its function: weaving together local and cloud AI models to create efficient development workflows.

---

*This architecture prioritizes simplicity and reliability over features. Get the core loop working first, then iterate.*
