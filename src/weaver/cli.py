"""CLI interface for the Multi-Agent Weaver.

Provides a rich terminal UI for chatting with multiple agents.

Usage:
    poetry run mao                    # Start interactive session
    poetry run mao --local-only       # Only use local model
    poetry run mao --claude-only      # Only use Claude Code
    echo "hello" | poetry run mao -p  # Non-interactive mode
"""

from __future__ import annotations

import argparse
import asyncio
import logging
import re
import sys
from pathlib import Path

from rich.console import Console
from rich.live import Live
from rich.markdown import Markdown
from rich.panel import Panel
from rich.prompt import Prompt
from rich.table import Table

from weaver.agents import AgentConfig, AgentType
from weaver.orchestrator import Weaver

console = Console()
logger = logging.getLogger(__name__)

# Pattern to detect agent-to-agent commands in responses
AGENT_COMMAND_PATTERN = re.compile(
    r"\n(/(?:claude|local|senior|junior)\s+.+?)$", re.IGNORECASE | re.DOTALL
)


def setup_logging(debug: bool = False) -> None:
    """Configure logging."""
    level = logging.DEBUG if debug else logging.WARNING
    logging.basicConfig(
        level=level,
        format="%(asctime)s - %(levelname)s - %(name)s - %(message)s",
    )
    # Suppress noisy HTTP logs
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("httpcore").setLevel(logging.WARNING)


def extract_agent_command(response: str) -> tuple[str, str | None]:
    """Extract an agent-to-agent command from the end of a response.

    Args:
        response: The agent's full response text

    Returns:
        Tuple of (clean_response, command) where command is None if not found
    """
    # Look for /claude or /local command at the end of the response
    match = AGENT_COMMAND_PATTERN.search(response)
    if match:
        command = match.group(1).strip()
        clean_response = response[: match.start()].rstrip()
        return clean_response, command
    return response, None


def print_welcome() -> None:
    """Print welcome banner."""
    console.print()
    console.print(
        Panel(
            "[bold blue]Claude Code + Junior Engineer[/]\n"
            "[dim]Claude with a local model assistant for delegation[/]",
            border_style="blue",
        )
    )
    console.print()


def print_help() -> None:
    """Print help message."""
    console.print("[dim]All your messages go to Claude (Senior Engineer).[/]")
    console.print("[dim]Claude delegates to the local model (Junior) as needed.[/]")
    console.print()

    help_table = Table(show_header=False, box=None, padding=(0, 2))
    help_table.add_column("Command", style="cyan")
    help_table.add_column("Description")

    help_table.add_row("[message]", "Talk to Claude (default)")
    help_table.add_row("/agents", "List available agents")
    help_table.add_row("/memory", "Show shared memory status")
    help_table.add_row("/memory list", "List shared notepad")
    help_table.add_row("/history", "Show conversation history")
    help_table.add_row("/clear", "Clear conversation history")
    help_table.add_row("/help", "Show this help")
    help_table.add_row("/quit", "Exit")

    console.print(Panel(help_table, title="Commands", border_style="dim"))


async def handle_command(
    command: str,
    orch: Weaver,
) -> bool:
    """Handle a slash command.

    Returns:
        True if the command was handled, False if it should be treated as a message
    """
    parts = command.split(maxsplit=1)
    cmd = parts[0].lower()
    arg = parts[1] if len(parts) > 1 else ""

    if cmd == "/help":
        print_help()
        return True

    if cmd in ("/quit", "/exit", "/q"):
        console.print("[dim]Goodbye![/]")
        raise KeyboardInterrupt

    if cmd == "/agents":
        agents = orch.list_agents()
        table = Table(title="Available Agents")
        table.add_column("Type", style="cyan")
        table.add_column("Name")
        table.add_column("Status")

        for agent in agents:
            status = "[green]available[/]" if agent["available"] else "[red]unavailable[/]"
            table.add_row(agent["type"], agent["name"], status)

        console.print(table)
        return True

    if cmd == "/rules":
        rules = orch.router.list_rules()
        table = Table(title="Routing Rules")
        table.add_column("Priority", style="cyan", justify="right")
        table.add_column("Pattern")
        table.add_column("Agent", style="green")
        table.add_column("Description")

        for rule in rules:
            table.add_row(
                str(rule["priority"]),
                rule["pattern"][:30] + "..." if len(rule["pattern"]) > 30 else rule["pattern"],
                rule["agent"],
                rule["description"],
            )

        console.print(table)
        return True

    if cmd == "/history":
        if not orch.conversation.messages:
            console.print("[dim]No conversation history[/]")
            return True

        for msg in orch.conversation.messages[-10:]:
            role_style = "green" if msg.role == "user" else "blue"
            agent_info = f" [{msg.agent.value}]" if msg.agent else ""
            console.print(f"[{role_style}]{msg.role}{agent_info}:[/] {msg.content[:100]}...")

        return True

    if cmd == "/export":
        if not arg:
            arg = f"session_{orch.conversation.session_id}.json"
        path = Path(arg)
        orch.export_session(path)
        console.print(f"[green]Exported to {path}[/]")
        return True

    if cmd == "/clear":
        orch.clear_conversation()
        console.print("[dim]Conversation cleared[/]")
        return True

    # Memory commands
    if cmd == "/memory":
        if not arg:
            # Show memory status
            if orch.memory.is_available:
                notes = orch.memory.list_shared(limit=5)
                console.print("[green]Shared memory: connected[/]")
                console.print(f"[dim]Recent notes: {len(notes)}[/]")
                for note in notes:
                    console.print(f"  [{note['author']}] {note['id']}: {note['preview'][:50]}...")
            else:
                console.print("[yellow]Shared memory: not available[/]")
                console.print("[dim]Start ArangoDB to enable shared memory[/]")
            return True

        # Parse subcommand
        subparts = arg.split(maxsplit=1)
        subcmd = subparts[0].lower()
        subarg = subparts[1] if len(subparts) > 1 else ""

        if subcmd == "write":
            if not subarg:
                console.print("[yellow]Usage: /memory write <content>[/]")
                return True
            if not orch.memory.is_available:
                console.print("[red]Shared memory not available[/]")
                return True
            note_id = orch.memory.write_shared(subarg, tags=["user"])
            if note_id:
                console.print(f"[green]Written to shared notepad: {note_id}[/]")
            else:
                console.print("[red]Failed to write to shared notepad[/]")
            return True

        if subcmd == "read":
            if not subarg:
                console.print("[yellow]Usage: /memory read <note_id>[/]")
                return True
            if not orch.memory.is_available:
                console.print("[red]Shared memory not available[/]")
                return True
            content = orch.memory.read_shared(subarg)
            if content:
                console.print(Panel(content, title=f"Note: {subarg}", border_style="dim"))
            else:
                console.print(f"[red]Note not found: {subarg}[/]")
            return True

        if subcmd == "list":
            if not orch.memory.is_available:
                console.print("[red]Shared memory not available[/]")
                return True
            notes = orch.memory.list_shared(limit=20)
            if notes:
                table = Table(title="Shared Notepad")
                table.add_column("ID", style="cyan")
                table.add_column("Author")
                table.add_column("Tags")
                table.add_column("Preview")
                for note in notes:
                    table.add_row(
                        note["id"],
                        note["author"],
                        ", ".join(note.get("tags", [])),
                        note["preview"][:40] + "..."
                        if len(note["preview"]) > 40
                        else note["preview"],
                    )
                console.print(table)
            else:
                console.print("[dim]No notes in shared notepad[/]")
            return True

        if subcmd == "delete":
            if not subarg:
                console.print("[yellow]Usage: /memory delete <note_id>[/]")
                return True
            if not orch.memory.is_available:
                console.print("[red]Shared memory not available[/]")
                return True
            if orch.memory.delete_shared(subarg):
                console.print(f"[green]Deleted note: {subarg}[/]")
            else:
                console.print(f"[red]Note not found: {subarg}[/]")
            return True

        console.print(f"[yellow]Unknown memory command: {subcmd}[/]")
        return True

    # Unknown command - let it through as a message to Claude
    return False


async def process_message(
    orch: Weaver,
    message: str,
    show_routing: bool = True,
    max_hops: int = 5,
) -> None:
    """Process a message, handling agent-to-agent communication.

    Args:
        orch: The orchestrator instance
        message: The message to process
        show_routing: Whether to show routing info
        max_hops: Maximum number of agent-to-agent hops to prevent infinite loops
    """
    current_message = message
    hop_count = 0

    while current_message and hop_count < max_hops:
        hop_count += 1

        # Route and get response
        agent_type, reason = orch.router.route(
            current_message,
            orch.conversation.get_context_summary(),
        )

        if show_routing:
            if hop_count > 1:
                console.print(f"[dim cyan]↪ Agent handoff to {agent_type.value}[/]")
            else:
                console.print(f"[dim]→ Routing to {agent_type.value} ({reason})[/]")

        # Stream response with live display
        console.print()
        response_text = ""

        with Live(console=console, refresh_per_second=10, transient=True) as live:
            try:
                async for chunk in orch.chat_stream(current_message):
                    response_text += chunk
                    live.update(Markdown(response_text))
            except Exception as e:
                console.print(f"[red]Error: {e}[/]")
                return

        # Check for agent-to-agent command in response
        clean_response, next_command = extract_agent_command(response_text)

        # Display the clean response (without the command)
        console.print()
        agent_label = f"[blue]{orch.current_agent.value}[/]" if orch.current_agent else ""
        console.print(
            Panel(
                Markdown(clean_response),
                title=f"Assistant {agent_label}",
                border_style="blue",
            )
        )
        console.print()

        # If there's a command for another agent, process it
        if next_command:
            console.print(f"[dim yellow]📨 Agent sending: {next_command[:60]}...[/]")
            current_message = next_command
        elif agent_type == AgentType.LOCAL_FAST or agent_type == AgentType.LOCAL_SPECIALIZED:
            # Local model responses always go back to Claude (this is Claude's tool)
            console.print("[dim yellow]📨 Auto-routing local response back to Claude...[/]")
            # Send the local model's response to Claude for review/continuation
            current_message = f"/claude The Junior Engineer responded:\n\n{clean_response}"
        else:
            break

    if hop_count >= max_hops:
        console.print("[yellow]⚠ Maximum agent hops reached, stopping chain[/]")


async def run_interactive(
    orch: Weaver,
    show_routing: bool = True,
) -> None:
    """Run the interactive chat loop."""
    print_welcome()
    print_help()
    console.print()

    while True:
        try:
            # Get user input
            user_input = Prompt.ask("[bold green]You[/]")

            if not user_input.strip():
                continue

            # Handle commands
            if user_input.startswith("/"):
                if await handle_command(user_input, orch):
                    continue

            # Process the message (handles agent-to-agent communication)
            await process_message(orch, user_input, show_routing)

        except KeyboardInterrupt:
            break
        except EOFError:
            break


async def run_noninteractive(
    orch: Weaver,
    message: str,
    stream: bool = False,
) -> None:
    """Run in non-interactive mode (pipe-friendly)."""
    if stream:
        async for chunk in orch.chat_stream(message):
            print(chunk, end="", flush=True)
        print()  # Final newline
    else:
        response = await orch.chat(message)
        print(response)


def main() -> int:
    """Main entry point for the CLI."""
    parser = argparse.ArgumentParser(
        description="Multi-Agent Weaver - Route between Claude Code and local models",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    mao                              # Interactive mode
    mao "Write hello world"          # Quick query
    echo "What is 2+2?" | mao -p     # Pipe mode
    mao --local-only                 # Only use local model
    mao --claude-only                # Only use Claude Code
        """,
    )
    parser.add_argument(
        "message",
        nargs="?",
        help="Message to send (runs non-interactively if provided)",
    )
    parser.add_argument(
        "-p",
        "--print",
        action="store_true",
        help="Print mode - read from stdin, print response, exit",
    )
    parser.add_argument(
        "--stream",
        action="store_true",
        help="Stream output (for pipe mode)",
    )
    parser.add_argument(
        "--local-only",
        action="store_true",
        help="Only use local model (no Claude Code)",
    )
    parser.add_argument(
        "--claude-only",
        action="store_true",
        help="Only use Claude Code (no local model)",
    )
    parser.add_argument(
        "--local-url",
        default="http://localhost:1234/v1",
        help="Local model API URL (default: http://localhost:1234/v1)",
    )
    parser.add_argument(
        "--local-model",
        default="local-model",
        help="Local model name (default: local-model)",
    )
    parser.add_argument(
        "--no-routing",
        action="store_true",
        help="Disable routing info display",
    )
    parser.add_argument(
        "--debug",
        action="store_true",
        help="Enable debug logging",
    )

    args = parser.parse_args()
    setup_logging(args.debug)

    # Create orchestrator
    claude_config = None if args.local_only else None  # Use defaults
    local_config = None

    if not args.claude_only:
        local_config = AgentConfig(
            name="local-model",
            agent_type=AgentType.LOCAL_FAST,
            base_url=args.local_url,
            model=args.local_model,
        )

    try:
        orch = Weaver(
            claude_config=claude_config,
            local_config=local_config,
        )
    except Exception as e:
        console.print(f"[red]Failed to initialize orchestrator: {e}[/]")
        return 1

    # Determine mode
    if args.print:
        # Pipe mode - read from stdin
        message = sys.stdin.read().strip()
        if not message:
            console.print("[red]No input provided[/]")
            return 1
        asyncio.run(run_noninteractive(orch, message, stream=args.stream))
    elif args.message:
        # Quick query mode
        asyncio.run(run_noninteractive(orch, args.message, stream=args.stream))
    else:
        # Interactive mode
        try:
            asyncio.run(run_interactive(orch, show_routing=not args.no_routing))
        except KeyboardInterrupt:
            console.print("\n[dim]Interrupted[/]")

    return 0


if __name__ == "__main__":
    sys.exit(main())
