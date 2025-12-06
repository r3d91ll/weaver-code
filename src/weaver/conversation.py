"""Conversation management for the Multi-Agent Orchestrator.

Maintains conversation state across agent switches, supporting:
- Full message history with agent attribution
- Context windowing for different agent capabilities
- Session export/import for persistence
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any

from weaver.agents import AgentType


@dataclass
class Message:
    """A single message in the conversation."""

    role: str  # "user", "assistant", "system"
    content: str
    agent: AgentType | None = None
    timestamp: datetime = field(default_factory=datetime.now)
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for serialization."""
        return {
            "role": self.role,
            "content": self.content,
            "agent": self.agent.value if self.agent else None,
            "timestamp": self.timestamp.isoformat(),
            "metadata": self.metadata,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Message:
        """Create from dictionary."""
        agent = AgentType(data["agent"]) if data.get("agent") else None
        timestamp = (
            datetime.fromisoformat(data["timestamp"]) if data.get("timestamp") else datetime.now()
        )
        return cls(
            role=data["role"],
            content=data["content"],
            agent=agent,
            timestamp=timestamp,
            metadata=data.get("metadata", {}),
        )

    def to_api_format(self) -> dict[str, str]:
        """Convert to OpenAI API message format."""
        return {"role": self.role, "content": self.content}


class ConversationManager:
    """Manages conversation state across multiple agents.

    Features:
    - Tracks which agent handled each message
    - Provides context windowing for different agent capabilities
    - Supports session export/import
    - Can fork conversations for parallel exploration

    Example:
        conv = ConversationManager()
        conv.add_user_message("Hello")
        conv.add_assistant_message("Hi there!", AgentType.CLAUDE_CODE)

        # Get context for Claude (can handle large context)
        claude_context = conv.get_context_for_agent(AgentType.CLAUDE_CODE)

        # Get context for local model (may need truncation)
        local_context = conv.get_context_for_agent(AgentType.LOCAL_FAST, max_messages=10)
    """

    def __init__(self, max_history: int = 100) -> None:
        """Initialize the conversation manager.

        Args:
            max_history: Maximum messages to keep in history
        """
        self.messages: list[Message] = []
        self.max_history = max_history
        self.session_id = datetime.now().strftime("%Y%m%d_%H%M%S")

    def add_user_message(self, content: str, **metadata: Any) -> None:
        """Add a user message to the conversation."""
        msg = Message(
            role="user",
            content=content,
            agent=None,
            metadata=metadata,
        )
        self.messages.append(msg)
        self._trim_history()

    def add_assistant_message(
        self,
        content: str,
        agent: AgentType,
        **metadata: Any,
    ) -> None:
        """Add an assistant message to the conversation."""
        msg = Message(
            role="assistant",
            content=content,
            agent=agent,
            metadata=metadata,
        )
        self.messages.append(msg)
        self._trim_history()

    def add_system_message(self, content: str) -> None:
        """Add a system message."""
        msg = Message(role="system", content=content)
        self.messages.append(msg)

    def _trim_history(self) -> None:
        """Trim history to max_history, keeping system messages."""
        if len(self.messages) <= self.max_history:
            return

        # Keep all system messages and most recent non-system messages
        system_msgs = [m for m in self.messages if m.role == "system"]
        non_system = [m for m in self.messages if m.role != "system"]

        keep_count = self.max_history - len(system_msgs)
        if keep_count > 0:
            self.messages = system_msgs + non_system[-keep_count:]
        else:
            self.messages = system_msgs[-self.max_history :]

    def get_context_for_agent(
        self,
        agent: AgentType,
        max_messages: int | None = None,
        max_chars: int | None = None,
    ) -> list[dict[str, str]]:
        """Get context window formatted for an agent.

        Args:
            agent: The agent that will receive this context
            max_messages: Maximum number of messages to include
            max_chars: Maximum total characters (approximate)

        Returns:
            List of messages in OpenAI API format
        """
        messages = self.messages.copy()

        # Apply message limit
        if max_messages and len(messages) > max_messages:
            # Keep system messages and most recent
            system_msgs = [m for m in messages if m.role == "system"]
            non_system = [m for m in messages if m.role != "system"]
            keep_count = max_messages - len(system_msgs)
            messages = system_msgs + non_system[-keep_count:]

        # Apply character limit
        if max_chars:
            total_chars = sum(len(m.content) for m in messages)
            while total_chars > max_chars and len(messages) > 1:
                # Remove oldest non-system message
                for i, m in enumerate(messages):
                    if m.role != "system":
                        total_chars -= len(m.content)
                        messages.pop(i)
                        break

        return [m.to_api_format() for m in messages]

    def get_context_summary(self) -> dict[str, Any]:
        """Get a summary of the conversation context.

        Useful for routing decisions.
        """
        if not self.messages:
            return {"message_count": 0, "agents_used": [], "last_agent": None}

        agents_used = list({m.agent.value for m in self.messages if m.agent})
        last_assistant = next(
            (m for m in reversed(self.messages) if m.role == "assistant"),
            None,
        )

        return {
            "message_count": len(self.messages),
            "agents_used": agents_used,
            "last_agent": last_assistant.agent.value
            if last_assistant and last_assistant.agent
            else None,
            "last_user_message": next(
                (m.content[:100] for m in reversed(self.messages) if m.role == "user"),
                None,
            ),
        }

    def get_last_n_messages(self, n: int) -> list[Message]:
        """Get the last N messages."""
        return self.messages[-n:] if n < len(self.messages) else self.messages.copy()

    def clear(self, keep_system: bool = True) -> None:
        """Clear conversation history.

        Args:
            keep_system: If True, keep system messages
        """
        if keep_system:
            self.messages = [m for m in self.messages if m.role == "system"]
        else:
            self.messages = []

    def export_session(self, path: str | Path) -> None:
        """Export conversation to JSON file."""
        path = Path(path)
        path.parent.mkdir(parents=True, exist_ok=True)

        data = {
            "session_id": self.session_id,
            "exported_at": datetime.now().isoformat(),
            "message_count": len(self.messages),
            "messages": [m.to_dict() for m in self.messages],
        }

        path.write_text(json.dumps(data, indent=2))

    @classmethod
    def import_session(cls, path: str | Path) -> ConversationManager:
        """Import conversation from JSON file."""
        path = Path(path)
        data = json.loads(path.read_text())

        conv = cls()
        conv.session_id = data.get("session_id", conv.session_id)
        conv.messages = [Message.from_dict(m) for m in data.get("messages", [])]

        return conv

    def fork(self) -> ConversationManager:
        """Create a branch of the conversation for parallel exploration.

        Returns:
            A new ConversationManager with a copy of the current state
        """
        forked = ConversationManager(max_history=self.max_history)
        forked.messages = [
            Message(
                role=m.role,
                content=m.content,
                agent=m.agent,
                timestamp=m.timestamp,
                metadata=m.metadata.copy(),
            )
            for m in self.messages
        ]
        forked.session_id = f"{self.session_id}_fork_{datetime.now().strftime('%H%M%S')}"
        return forked

    def __len__(self) -> int:
        return len(self.messages)

    def __iter__(self):
        return iter(self.messages)
