"""Weaver Code - Claude Code with a Junior Engineer.

Give Claude a local model assistant for delegation.

Usage:
    from weaver import Weaver

    weaver = Weaver()
    response = await weaver.chat("Build a REST API")

CLI:
    weaver  # Start interactive session
"""

from weaver.agents import AgentType, ClaudeCodeAgent, LocalModelAgent
from weaver.conversation import ConversationManager, Message
from weaver.orchestrator import Weaver
from weaver.prompts import (
    JUNIOR_ENGINEER_PROMPT,
    SENIOR_ENGINEER_PROMPT,
    get_default_prompt,
)
from weaver.router import Router, RoutingRule

__all__ = [
    "Weaver",
    "AgentType",
    "ClaudeCodeAgent",
    "LocalModelAgent",
    "Router",
    "RoutingRule",
    "ConversationManager",
    "Message",
    "SENIOR_ENGINEER_PROMPT",
    "JUNIOR_ENGINEER_PROMPT",
    "get_default_prompt",
]
