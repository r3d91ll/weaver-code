"""Main orchestrator that ties together agents, routing, and conversation.

This is the primary interface for the Multi-Agent Weaver.
"""

from __future__ import annotations

import json
import logging
import sys
from collections.abc import AsyncIterator
from pathlib import Path
from typing import Any

from weaver.agents import (
    AgentConfig,
    AgentType,
    BaseAgent,
    ClaudeCodeAgent,
    LocalModelAgent,
    create_agent,
)
from weaver.conversation import ConversationManager
from weaver.memory import WeaverMemory
from weaver.prompts import JUNIOR_ENGINEER_PROMPT, SENIOR_ENGINEER_PROMPT
from weaver.router import Router, create_default_router

logger = logging.getLogger(__name__)


def get_mcp_memory_config() -> str:
    """Generate MCP config JSON for the memory server.

    Returns:
        JSON string with MCP server configuration for Claude CLI.
    """
    # The mao-memory command is the entry point for the MCP server
    python_path = sys.executable
    config = {
        "mcpServers": {
            "memrag": {
                "command": python_path,
                "args": ["-m", "weaver.mcp_memory"],
            }
        }
    }
    return json.dumps(config)


class Weaver:
    """Multi-Agent Weaver - unified interface for multiple LLM backends.

    Routes conversations between Claude Code (subprocess with Max subscription)
    and local models (OpenAI-compatible HTTP APIs) based on task complexity.

    Example:
        # Create with defaults
        orch = Weaver()

        # Chat with automatic routing
        response = await orch.chat("Write a Python function to parse CSV")
        # -> Routes to Claude Code

        response = await orch.chat("What is 2+2?")
        # -> Routes to local model

        # Force specific agent
        response = await orch.chat("/claude explain this code...")
        response = await orch.chat("/local summarize this...")

        # Stream responses
        async for chunk in orch.chat_stream("Write hello world"):
            print(chunk, end="", flush=True)
    """

    def __init__(
        self,
        claude_config: AgentConfig | None = None,
        local_config: AgentConfig | None = None,
        router: Router | None = None,
        conversation: ConversationManager | None = None,
    ) -> None:
        """Initialize the orchestrator.

        Args:
            claude_config: Configuration for Claude Code agent
            local_config: Configuration for local model agent
            router: Custom router (uses default if not provided)
            conversation: Existing conversation to continue
        """
        # Initialize agents
        self.agents: dict[AgentType, BaseAgent] = {}

        # Get MCP config for memory server
        mcp_config = get_mcp_memory_config()

        # Claude Code agent (always available with Max subscription)
        if claude_config:
            # Apply default system prompt if not provided
            if claude_config.system_prompt is None:
                claude_config.system_prompt = SENIOR_ENGINEER_PROMPT
            # Add MCP config if not already set
            if claude_config.mcp_config is None:
                claude_config.mcp_config = mcp_config
            self.agents[AgentType.CLAUDE_CODE] = create_agent(claude_config)
        else:
            self.agents[AgentType.CLAUDE_CODE] = ClaudeCodeAgent(
                AgentConfig(
                    name="claude-code",
                    agent_type=AgentType.CLAUDE_CODE,
                    description="Claude Code via subprocess (uses Max subscription)",
                    system_prompt=SENIOR_ENGINEER_PROMPT,
                    mcp_config=mcp_config,
                )
            )

        # Local model agent (optional)
        if local_config:
            # Apply default system prompt if not provided
            if local_config.system_prompt is None:
                local_config.system_prompt = JUNIOR_ENGINEER_PROMPT
            self.agents[AgentType.LOCAL_FAST] = create_agent(local_config)
        else:
            # Try to connect to default LM Studio endpoint
            try:
                default_local = LocalModelAgent(
                    AgentConfig(
                        name="local-model",
                        agent_type=AgentType.LOCAL_FAST,
                        description="Local model via LM Studio",
                        base_url="http://localhost:1234/v1",
                        model="local-model",
                        system_prompt=JUNIOR_ENGINEER_PROMPT,
                    )
                )
                if default_local.is_available():
                    self.agents[AgentType.LOCAL_FAST] = default_local
                    logger.info("Connected to local model at localhost:1234")
            except Exception as e:
                logger.debug(f"No local model available: {e}")

        # Router
        self.router = router or create_default_router()

        # Conversation manager
        self.conversation = conversation or ConversationManager()

        # Shared memory (optional - gracefully handles unavailable ArangoDB)
        self.memory = WeaverMemory(author="orchestrator")
        if self.memory.is_available:
            logger.info("Shared memory connected")
        else:
            logger.info("Shared memory unavailable (ArangoDB not running?)")

        # Track current agent for continuity
        self._current_agent: AgentType | None = None

    def add_agent(self, agent_type: AgentType, agent: BaseAgent) -> None:
        """Add or replace an agent."""
        self.agents[agent_type] = agent

    def get_agent(self, agent_type: AgentType) -> BaseAgent | None:
        """Get an agent by type."""
        return self.agents.get(agent_type)

    def list_agents(self) -> list[dict[str, Any]]:
        """List all configured agents."""
        return [
            {
                "type": agent_type.value,
                "name": agent.name,
                "available": agent.is_available(),
            }
            for agent_type, agent in self.agents.items()
        ]

    def _get_memory_context_for_local(self) -> str:
        """Get memory context to inject into local model prompts.

        Since local models can't use MCP tools, we inject shared memory
        context directly into their prompts.
        """
        if not self.memory.is_available:
            return ""

        notes = self.memory.list_shared(limit=5)
        if not notes:
            return ""

        lines = ["## Shared Notepad Context", "Recent notes from shared memory:", ""]
        for note in notes:
            author = note.get("author", "unknown")
            preview = note.get("preview", "")
            note_id = note.get("id", "")
            lines.append(f"[{author}] ({note_id}): {preview}")

        lines.append("")
        return "\n".join(lines)

    async def chat(
        self,
        message: str,
        system_prompt: str | None = None,
        force_agent: AgentType | None = None,
    ) -> str:
        """Send a message and get a response.

        Args:
            message: The user's message
            system_prompt: Optional system prompt override
            force_agent: Force a specific agent (bypasses routing)

        Returns:
            The agent's response
        """
        # Route to appropriate agent
        if force_agent:
            agent_type = force_agent
            reason = "forced by caller"
        else:
            agent_type, reason = self.router.route(
                message,
                self.conversation.get_context_summary(),
            )

        # Get the agent
        agent = self.agents.get(agent_type)
        if not agent or not agent.is_available():
            # Fall back to another available agent
            for fallback_type, fallback_agent in self.agents.items():
                if fallback_agent.is_available():
                    logger.warning(
                        f"Agent {agent_type.value} unavailable, "
                        f"falling back to {fallback_type.value}"
                    )
                    agent = fallback_agent
                    agent_type = fallback_type
                    break
            else:
                raise RuntimeError("No agents available")

        logger.info(f"Routing to {agent_type.value}: {reason}")

        # Add user message to conversation
        self.conversation.add_user_message(message, routing_reason=reason)

        # Get context for the agent
        history = self.conversation.get_context_for_agent(
            agent_type,
            max_messages=50 if agent_type == AgentType.CLAUDE_CODE else 20,
        )

        # Call the agent
        try:
            response = await agent.chat(
                message,
                history=history[:-1],  # Exclude current message (already in prompt)
                system_prompt=system_prompt,
            )
        except Exception as e:
            logger.error(f"Agent {agent_type.value} failed: {e}")
            raise

        # Add response to conversation
        self.conversation.add_assistant_message(response, agent_type)
        self._current_agent = agent_type

        return response

    async def chat_stream(
        self,
        message: str,
        system_prompt: str | None = None,
        force_agent: AgentType | None = None,
    ) -> AsyncIterator[str]:
        """Stream a response from an agent.

        Args:
            message: The user's message
            system_prompt: Optional system prompt override
            force_agent: Force a specific agent

        Yields:
            Response chunks as they arrive
        """
        # Route to appropriate agent
        if force_agent:
            agent_type = force_agent
            reason = "forced by caller"
        else:
            agent_type, reason = self.router.route(
                message,
                self.conversation.get_context_summary(),
            )

        agent = self.agents.get(agent_type)
        if not agent or not agent.is_available():
            for fallback_type, fallback_agent in self.agents.items():
                if fallback_agent.is_available():
                    agent = fallback_agent
                    agent_type = fallback_type
                    break
            else:
                raise RuntimeError("No agents available")

        logger.info(f"Streaming from {agent_type.value}: {reason}")

        self.conversation.add_user_message(message, routing_reason=reason)

        history = self.conversation.get_context_for_agent(
            agent_type,
            max_messages=50 if agent_type == AgentType.CLAUDE_CODE else 20,
        )

        # For local models, inject memory context into the message
        effective_message = message
        if agent_type in (AgentType.LOCAL_FAST, AgentType.LOCAL_SPECIALIZED):
            memory_context = self._get_memory_context_for_local()
            if memory_context:
                effective_message = f"{memory_context}\n---\n\n{message}"

        # Collect full response for history
        full_response = ""

        try:
            async for chunk in agent.chat_stream(
                effective_message,
                history=history[:-1],
                system_prompt=system_prompt,
            ):
                full_response += chunk
                yield chunk
        except Exception as e:
            logger.error(f"Agent {agent_type.value} stream failed: {e}")
            raise
        finally:
            # Save full response to conversation
            if full_response:
                self.conversation.add_assistant_message(full_response, agent_type)
            self._current_agent = agent_type

    def export_session(self, path: str | Path) -> None:
        """Export the conversation to a file."""
        self.conversation.export_session(path)

    def clear_conversation(self, keep_system: bool = True) -> None:
        """Clear the conversation history."""
        self.conversation.clear(keep_system=keep_system)

    @property
    def current_agent(self) -> AgentType | None:
        """The agent that handled the last message."""
        return self._current_agent

    async def close(self) -> None:
        """Close all agents and release resources."""
        for agent in self.agents.values():
            if hasattr(agent, "close"):
                await agent.close()


async def quick_chat(message: str) -> str:
    """Quick one-shot chat using defaults.

    Convenience function for simple use cases.

    Example:
        response = await quick_chat("Write hello world in Python")
    """
    orch = Weaver()
    try:
        return await orch.chat(message)
    finally:
        await orch.close()
