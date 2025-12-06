"""MCP Memory Server for Weaver Code.

Provides Claude Code with access to the shared notepad via the Model Context Protocol (MCP).

This is a simplified MCP server focused on memory tools for agent coordination.

Usage:
    Configure in Claude Code settings:
    {
        "mcpServers": {
            "weaver": {
                "command": "python",
                "args": ["-m", "weaver.mcp_memory"]
            }
        }
    }
"""

from __future__ import annotations

import json
import logging
import sys
from typing import Any

from weaver.memory import WeaverMemory

logger = logging.getLogger(__name__)


class MemoryMCPServer:
    """MCP Server for shared notepad.

    Tools provided:
        Shared Notepad (both agents can access):
        - write_shared: Write to shared space
        - read_shared: Read from shared space
        - list_shared: List recent notes
        - delete_shared: Remove a note
    """

    def __init__(self) -> None:
        self._memory = WeaverMemory(author="claude")
        logger.info("Weaver Memory MCP server initialized")

    # ========================================================================
    # MCP Protocol
    # ========================================================================

    def handle_request(self, request: dict[str, Any]) -> dict[str, Any]:
        method = request.get("method", "")
        params = request.get("params", {})
        request_id = request.get("id")

        try:
            if method == "initialize":
                result = self._handle_initialize()
            elif method == "tools/list":
                result = self._handle_tools_list()
            elif method == "tools/call":
                result = self._handle_tools_call(params)
            elif method == "shutdown":
                result = {}
            else:
                return self._error_response(request_id, -32601, f"Method not found: {method}")

            return self._success_response(request_id, result)
        except Exception as e:
            logger.exception(f"Error handling {method}")
            return self._error_response(request_id, -32603, str(e))

    def _success_response(self, request_id: Any, result: Any) -> dict[str, Any]:
        return {"jsonrpc": "2.0", "id": request_id, "result": result}

    def _error_response(self, request_id: Any, code: int, message: str) -> dict[str, Any]:
        return {"jsonrpc": "2.0", "id": request_id, "error": {"code": code, "message": message}}

    def _handle_initialize(self) -> dict[str, Any]:
        return {
            "protocolVersion": "2024-11-05",
            "capabilities": {"tools": {}},
            "serverInfo": {"name": "weaver-memory", "version": "0.1.0"},
        }

    def _handle_tools_list(self) -> dict[str, Any]:
        tools = [
            {
                "name": "write_shared",
                "description": (
                    "Write content to the shared notepad. "
                    "Both Claude and local model can read this."
                ),
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "content": {"type": "string", "description": "The content to write"},
                        "note_id": {"type": "string", "description": "Optional custom ID"},
                        "tags": {
                            "type": "array",
                            "items": {"type": "string"},
                            "description": "Optional tags",
                        },
                    },
                    "required": ["content"],
                },
            },
            {
                "name": "read_shared",
                "description": "Read a note from the shared notepad by ID.",
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "note_id": {"type": "string", "description": "The note ID to read"},
                    },
                    "required": ["note_id"],
                },
            },
            {
                "name": "list_shared",
                "description": "List recent notes from the shared notepad.",
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "tags": {
                            "type": "array",
                            "items": {"type": "string"},
                            "description": "Filter by tags",
                        },
                        "author": {
                            "type": "string",
                            "description": "Filter by author ('claude' or 'local')",
                        },
                        "limit": {
                            "type": "integer",
                            "description": "Maximum results (default: 20)",
                        },
                    },
                },
            },
            {
                "name": "delete_shared",
                "description": "Delete a note from the shared notepad.",
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "note_id": {"type": "string", "description": "The note ID to delete"},
                    },
                    "required": ["note_id"],
                },
            },
        ]
        return {"tools": tools}

    def _handle_tools_call(self, params: dict[str, Any]) -> dict[str, Any]:
        tool_name = params.get("name", "")
        args = params.get("arguments", {})

        handlers = {
            "write_shared": self._tool_write_shared,
            "read_shared": self._tool_read_shared,
            "list_shared": self._tool_list_shared,
            "delete_shared": self._tool_delete_shared,
        }

        handler = handlers.get(tool_name)
        if handler:
            return handler(args)
        return {
            "content": [{"type": "text", "text": f"Unknown tool: {tool_name}"}],
            "isError": True,
        }

    # ========================================================================
    # Shared Notepad Tools
    # ========================================================================

    def _tool_write_shared(self, args: dict[str, Any]) -> dict[str, Any]:
        try:
            note_id = self._memory.write_shared(
                content=args["content"],
                note_id=args.get("note_id"),
                tags=args.get("tags", []),
            )
            return {
                "content": [
                    {
                        "type": "text",
                        "text": json.dumps(
                            {
                                "status": "written",
                                "note_id": note_id,
                                "length": len(args["content"]),
                            }
                        ),
                    }
                ]
            }
        except Exception as e:
            return {
                "content": [{"type": "text", "text": json.dumps({"error": str(e)})}],
                "isError": True,
            }

    def _tool_read_shared(self, args: dict[str, Any]) -> dict[str, Any]:
        try:
            content = self._memory.read_shared(args["note_id"])
            if content is None:
                return {
                    "content": [{"type": "text", "text": json.dumps({"error": "Note not found"})}],
                    "isError": True,
                }
            return {
                "content": [
                    {
                        "type": "text",
                        "text": json.dumps(
                            {
                                "note_id": args["note_id"],
                                "content": content,
                            }
                        ),
                    }
                ]
            }
        except Exception as e:
            return {
                "content": [{"type": "text", "text": json.dumps({"error": str(e)})}],
                "isError": True,
            }

    def _tool_list_shared(self, args: dict[str, Any]) -> dict[str, Any]:
        try:
            notes = self._memory.list_shared(
                tags=args.get("tags"),
                author=args.get("author"),
                limit=args.get("limit", 20),
            )
            return {
                "content": [
                    {
                        "type": "text",
                        "text": json.dumps(
                            {
                                "notes": notes,
                                "count": len(notes),
                            }
                        ),
                    }
                ]
            }
        except Exception as e:
            return {
                "content": [{"type": "text", "text": json.dumps({"error": str(e)})}],
                "isError": True,
            }

    def _tool_delete_shared(self, args: dict[str, Any]) -> dict[str, Any]:
        try:
            deleted = self._memory.delete_shared(args["note_id"])
            if deleted:
                return {
                    "content": [
                        {
                            "type": "text",
                            "text": json.dumps(
                                {
                                    "status": "deleted",
                                    "note_id": args["note_id"],
                                }
                            ),
                        }
                    ]
                }
            return {
                "content": [{"type": "text", "text": json.dumps({"error": "Note not found"})}],
                "isError": True,
            }
        except Exception as e:
            return {
                "content": [{"type": "text", "text": json.dumps({"error": str(e)})}],
                "isError": True,
            }

    # ========================================================================
    # Server Loop
    # ========================================================================

    def run(self) -> None:
        logger.info("Weaver Memory MCP server starting...")
        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue
            try:
                request = json.loads(line)
                response = self.handle_request(request)
                print(json.dumps(response), flush=True)
            except json.JSONDecodeError as e:
                print(
                    json.dumps(self._error_response(None, -32700, f"Parse error: {e}")), flush=True
                )

    def close(self) -> None:
        self._memory.close()


def main() -> None:
    import argparse

    parser = argparse.ArgumentParser(description="Weaver Memory MCP Server")
    parser.add_argument("--debug", action="store_true", help="Enable debug logging")
    args = parser.parse_args()

    log_level = logging.DEBUG if args.debug else logging.WARNING
    logging.basicConfig(
        level=log_level, format="%(asctime)s - %(levelname)s - %(message)s", stream=sys.stderr
    )

    server = MemoryMCPServer()
    try:
        server.run()
    finally:
        server.close()


if __name__ == "__main__":
    main()
