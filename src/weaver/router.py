"""Routing logic for the Multi-Agent Orchestrator.

Simple routing:
- User messages ALWAYS go to Claude (Senior Engineer)
- Only /local commands (from Claude's responses) go to the local model
- Claude decides when to delegate to the local model

This is Claude's tool for managing a local assistant, not a general router.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Any

from weaver.agents import AgentType

logger = logging.getLogger(__name__)


@dataclass
class RoutingRule:
    """A rule for routing messages to agents (kept for API compatibility)."""

    pattern: str
    agent: AgentType
    priority: int = 0
    description: str = ""


class Router:
    """Routes messages to the appropriate agent.

    Simple routing logic:
    - All user messages go to Claude (Senior Engineer)
    - /local commands (from agent responses) go to local model
    - /claude commands explicitly go to Claude

    Claude is the primary interface. The user talks to Claude,
    and Claude delegates to the local model as needed.
    """

    def __init__(
        self,
        rules: list[RoutingRule] | None = None,
        default: AgentType = AgentType.CLAUDE_CODE,
    ) -> None:
        """Initialize the router.

        Args:
            rules: Ignored (kept for API compatibility)
            default: Ignored (always defaults to Claude)
        """
        self.rules = rules or []
        self.default = AgentType.CLAUDE_CODE  # Always Claude

    def route(
        self,
        message: str,
        context: dict[str, Any] | None = None,
    ) -> tuple[AgentType, str]:
        """Determine which agent should handle a message.

        Args:
            message: The message to route
            context: Optional conversation context (unused)

        Returns:
            Tuple of (agent_type, reason)
        """
        message_lower = message.lower().strip()

        # /local commands go to local model (used by Claude to delegate)
        if message_lower.startswith("/local") or message_lower.startswith("/junior"):
            return AgentType.LOCAL_FAST, "delegation to Junior Engineer"

        # Everything else goes to Claude (Senior Engineer)
        # Including explicit /claude commands (just strips the prefix)
        return AgentType.CLAUDE_CODE, "Senior Engineer"

    def add_rule(self, rule: RoutingRule) -> None:
        """Add a routing rule (no-op, kept for API compatibility)."""
        self.rules.append(rule)

    def remove_rule(self, pattern: str) -> bool:
        """Remove a rule (no-op, kept for API compatibility)."""
        return False

    def list_rules(self) -> list[dict[str, Any]]:
        """Get routing info."""
        return [
            {
                "pattern": "*",
                "agent": "claude_code",
                "priority": 100,
                "description": "All user messages → Claude (Senior Engineer)",
            },
            {
                "pattern": "/local, /junior",
                "agent": "local_fast",
                "priority": 100,
                "description": "Delegation commands → Local (Junior Engineer)",
            },
        ]


def create_default_router() -> Router:
    """Create the default router (all messages to Claude)."""
    return Router()
