"""Agent implementations for the Multi-Agent Orchestrator.

This module provides agent wrappers for:
- Claude Code (subprocess using Max subscription)
- Local models (OpenAI-compatible HTTP APIs)
"""

from __future__ import annotations

import asyncio
import json
import logging
import subprocess
from abc import ABC, abstractmethod
from collections.abc import AsyncIterator
from dataclasses import dataclass, field
from enum import Enum

import httpx

logger = logging.getLogger(__name__)


class AgentType(Enum):
    """Types of agents available for routing."""

    CLAUDE_CODE = "claude_code"  # Complex reasoning, code generation
    LOCAL_FAST = "local_fast"  # Quick responses, simple queries
    LOCAL_SPECIALIZED = "local_specialized"  # Domain-specific tasks


@dataclass
class AgentConfig:
    """Configuration for an agent."""

    name: str
    agent_type: AgentType
    description: str = ""
    # For local models
    base_url: str | None = None
    model: str | None = None
    max_tokens: int = 4096
    temperature: float = 0.7
    # For Claude Code
    claude_args: list[str] = field(default_factory=list)
    system_prompt: str | None = None
    # MCP server config (JSON string or path)
    mcp_config: str | None = None


class BaseAgent(ABC):
    """Abstract base class for agents."""

    def __init__(self, config: AgentConfig) -> None:
        self.config = config
        self.name = config.name

    @abstractmethod
    async def chat(
        self,
        message: str,
        history: list[dict[str, str]] | None = None,
        system_prompt: str | None = None,
    ) -> str:
        """Send a message and get a complete response."""
        ...

    @abstractmethod
    async def chat_stream(
        self,
        message: str,
        history: list[dict[str, str]] | None = None,
        system_prompt: str | None = None,
    ) -> AsyncIterator[str]:
        """Send a message and stream the response."""
        ...

    def is_available(self) -> bool:
        """Check if this agent is available."""
        return True


class ClaudeCodeAgent(BaseAgent):
    """Agent that spawns Claude Code as a subprocess.

    Uses the `claude` CLI with `-p` flag for non-interactive mode.
    This leverages your Max subscription auth automatically.

    Example:
        agent = ClaudeCodeAgent(AgentConfig(
            name="claude",
            agent_type=AgentType.CLAUDE_CODE,
        ))
        response = await agent.chat("Write hello world in Python")
    """

    def __init__(self, config: AgentConfig) -> None:
        super().__init__(config)
        self._check_claude_available()

    def _check_claude_available(self) -> None:
        """Verify claude CLI is installed and authenticated."""
        try:
            result = subprocess.run(
                ["claude", "--version"],
                capture_output=True,
                text=True,
                timeout=5,
            )
            if result.returncode != 0:
                logger.warning(f"Claude CLI check failed: {result.stderr}")
        except FileNotFoundError:
            logger.error(
                "Claude CLI not found. Install with: npm install -g @anthropic-ai/claude-code"
            )
        except subprocess.TimeoutExpired:
            logger.warning("Claude CLI version check timed out")

    def is_available(self) -> bool:
        """Check if Claude CLI is available."""
        try:
            result = subprocess.run(
                ["claude", "--version"],
                capture_output=True,
                text=True,
                timeout=5,
            )
            return result.returncode == 0
        except (FileNotFoundError, subprocess.TimeoutExpired):
            return False

    def _build_prompt(
        self,
        message: str,
        history: list[dict[str, str]] | None = None,
    ) -> str:
        """Build the full prompt including conversation history."""
        parts = []

        if history:
            for msg in history:
                role = msg.get("role", "user")
                content = msg.get("content", "")
                if role == "user":
                    parts.append(f"User: {content}")
                elif role == "assistant":
                    parts.append(f"Assistant: {content}")

        parts.append(f"User: {message}")
        parts.append("Assistant:")

        return "\n\n".join(parts)

    async def chat(
        self,
        message: str,
        history: list[dict[str, str]] | None = None,
        system_prompt: str | None = None,
    ) -> str:
        """Send a message to Claude Code and get a complete response."""
        cmd = ["claude", "-p", "--output-format", "json"]

        # Add system prompt if provided
        if system_prompt or self.config.system_prompt:
            prompt = system_prompt or self.config.system_prompt
            cmd.extend(["--system-prompt", prompt])

        # Add any extra args from config
        cmd.extend(self.config.claude_args)

        # Build the full prompt with history
        full_prompt = self._build_prompt(message, history)

        logger.debug(f"Calling Claude Code: {' '.join(cmd[:4])}...")

        # Run in thread pool to not block
        loop = asyncio.get_event_loop()
        result = await loop.run_in_executor(
            None,
            lambda: subprocess.run(
                cmd,
                input=full_prompt,
                capture_output=True,
                text=True,
                timeout=300,  # 5 minute timeout
            ),
        )

        if result.returncode != 0:
            error_msg = result.stderr or "Unknown error"
            logger.error(f"Claude Code error: {error_msg}")
            raise RuntimeError(f"Claude Code failed: {error_msg}")

        # Parse JSON response
        try:
            response = json.loads(result.stdout)
            # The JSON format returns {"result": "...", ...}
            return response.get("result", result.stdout)
        except json.JSONDecodeError:
            # If not JSON, return raw output
            return result.stdout.strip()

    async def chat_stream(
        self,
        message: str,
        history: list[dict[str, str]] | None = None,
        system_prompt: str | None = None,
    ) -> AsyncIterator[str]:
        """Stream response from Claude Code."""
        # --verbose is required when using --output-format=stream-json with -p
        # --dangerously-skip-permissions allows MCP tools without prompting
        cmd = [
            "claude",
            "-p",
            "--verbose",
            "--output-format",
            "stream-json",
            "--dangerously-skip-permissions",
        ]

        if system_prompt or self.config.system_prompt:
            prompt = system_prompt or self.config.system_prompt
            cmd.extend(["--system-prompt", prompt])

        # Add MCP config if provided
        if self.config.mcp_config:
            cmd.extend(["--mcp-config", self.config.mcp_config])

        cmd.extend(self.config.claude_args)
        full_prompt = self._build_prompt(message, history)

        logger.debug(f"Streaming from Claude Code: {' '.join(cmd[:4])}...")

        # Start the process
        process = await asyncio.create_subprocess_exec(
            *cmd,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )

        # Send the prompt
        if process.stdin:
            process.stdin.write(full_prompt.encode())
            await process.stdin.drain()
            process.stdin.close()

        # Stream the output
        if process.stdout:
            async for line in process.stdout:
                line_str = line.decode().strip()
                if not line_str:
                    continue

                try:
                    data = json.loads(line_str)
                    # stream-json format has different event types
                    if data.get("type") == "content_block_delta":
                        delta = data.get("delta", {})
                        if "text" in delta:
                            yield delta["text"]
                    elif data.get("type") == "message_delta":
                        # End of message
                        pass
                    elif "result" in data:
                        # Final result
                        yield data["result"]
                except json.JSONDecodeError:
                    # Not JSON, yield as-is
                    yield line_str

        await process.wait()

        if process.returncode != 0:
            stderr = await process.stderr.read() if process.stderr else b""
            logger.error(f"Claude Code stream error: {stderr.decode()}")


class LocalModelAgent(BaseAgent):
    """Agent that calls local models via OpenAI-compatible HTTP APIs.

    Works with Ollama, vLLM, LM Studio, or any OpenAI-compatible server.

    Example:
        agent = LocalModelAgent(AgentConfig(
            name="local-llama",
            agent_type=AgentType.LOCAL_FAST,
            base_url="http://localhost:11434/v1",
            model="llama3.2",
        ))
        response = await agent.chat("What is 2+2?")
    """

    def __init__(self, config: AgentConfig) -> None:
        super().__init__(config)
        if not config.base_url:
            raise ValueError("LocalModelAgent requires base_url in config")
        if not config.model:
            raise ValueError("LocalModelAgent requires model in config")
        self._client: httpx.AsyncClient | None = None

    @property
    def client(self) -> httpx.AsyncClient:
        """Lazy-initialized HTTP client."""
        if self._client is None:
            self._client = httpx.AsyncClient(
                base_url=self.config.base_url,
                timeout=120.0,
            )
        return self._client

    def is_available(self) -> bool:
        """Check if the local model server is available."""
        try:
            with httpx.Client(base_url=self.config.base_url, timeout=5.0) as client:
                response = client.get("/models")
                return response.status_code == 200
        except Exception:
            return False

    def _build_messages(
        self,
        message: str,
        history: list[dict[str, str]] | None = None,
        system_prompt: str | None = None,
    ) -> list[dict[str, str]]:
        """Build messages array for the API."""
        messages = []

        # Add system prompt if provided
        if system_prompt or self.config.system_prompt:
            messages.append(
                {
                    "role": "system",
                    "content": system_prompt or self.config.system_prompt,
                }
            )

        # Add history
        if history:
            messages.extend(history)

        # Add current message
        messages.append({"role": "user", "content": message})

        return messages

    async def chat(
        self,
        message: str,
        history: list[dict[str, str]] | None = None,
        system_prompt: str | None = None,
    ) -> str:
        """Send a message and get a complete response."""
        messages = self._build_messages(message, history, system_prompt)

        response = await self.client.post(
            "/chat/completions",
            json={
                "model": self.config.model,
                "messages": messages,
                "max_tokens": self.config.max_tokens,
                "temperature": self.config.temperature,
                "stream": False,
            },
        )
        response.raise_for_status()

        data = response.json()
        return data["choices"][0]["message"]["content"]

    async def chat_stream(
        self,
        message: str,
        history: list[dict[str, str]] | None = None,
        system_prompt: str | None = None,
    ) -> AsyncIterator[str]:
        """Stream response from the local model."""
        messages = self._build_messages(message, history, system_prompt)

        async with self.client.stream(
            "POST",
            "/chat/completions",
            json={
                "model": self.config.model,
                "messages": messages,
                "max_tokens": self.config.max_tokens,
                "temperature": self.config.temperature,
                "stream": True,
            },
        ) as response:
            response.raise_for_status()

            async for line in response.aiter_lines():
                if not line or not line.startswith("data: "):
                    continue

                data_str = line[6:]  # Remove "data: " prefix
                if data_str == "[DONE]":
                    break

                try:
                    data = json.loads(data_str)
                    delta = data["choices"][0].get("delta", {})
                    if "content" in delta:
                        yield delta["content"]
                except json.JSONDecodeError:
                    continue

    async def close(self) -> None:
        """Close the HTTP client."""
        if self._client:
            await self._client.aclose()
            self._client = None


def create_agent(config: AgentConfig) -> BaseAgent:
    """Factory function to create an agent from config."""
    if config.agent_type == AgentType.CLAUDE_CODE:
        return ClaudeCodeAgent(config)
    else:
        return LocalModelAgent(config)
