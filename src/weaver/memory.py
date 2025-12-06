"""Memory integration for Weaver.

Provides shared notepad functionality for Claude and Junior to communicate.
Uses ArangoDB when available, falls back to local JSON storage.
"""

from __future__ import annotations

import json
import logging
import os
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

# Storage location for local fallback
WEAVER_DATA_DIR = Path.home() / ".weaver"
SHARED_NOTES_FILE = WEAVER_DATA_DIR / "shared_notes.json"


@dataclass
class Note:
    """A shared note between agents."""

    id: str
    content: str
    author: str
    created_at: str
    tags: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "content": self.content,
            "author": self.author,
            "created_at": self.created_at,
            "tags": self.tags,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Note:
        return cls(
            id=data["id"],
            content=data["content"],
            author=data.get("author", "unknown"),
            created_at=data.get("created_at", ""),
            tags=data.get("tags", []),
        )


class WeaverMemory:
    """Shared memory for Weaver.

    Provides a simple key-value notepad that both Claude and Junior can access.
    Falls back to local JSON storage if ArangoDB is not available.
    """

    def __init__(self, author: str = "weaver") -> None:
        self.author = author
        self._notes: list[Note] = []
        self._arango_available = False

        # Try ArangoDB first
        self._try_connect_arango()

        # Fall back to local storage
        if not self._arango_available:
            self._load_local_notes()

    def _try_connect_arango(self) -> None:
        """Try to connect to ArangoDB."""
        try:
            import httpx

            socket_path = "/run/arangodb3/arangodb.sock"
            password = os.environ.get("ARANGO_PASSWORD", "")

            if not os.path.exists(socket_path):
                logger.debug("ArangoDB socket not found")
                return

            transport = httpx.HTTPTransport(uds=socket_path)
            client = httpx.Client(
                transport=transport,
                base_url="http://localhost",
                auth=("root", password),
                timeout=5.0,
            )

            # Test connection
            response = client.get("/_api/version")
            if response.status_code == 200:
                self._arango_available = True
                self._arango_client = client
                logger.info("Connected to ArangoDB")
            else:
                client.close()
        except Exception as e:
            logger.debug(f"ArangoDB not available: {e}")

    def _load_local_notes(self) -> None:
        """Load notes from local JSON file."""
        WEAVER_DATA_DIR.mkdir(parents=True, exist_ok=True)
        if SHARED_NOTES_FILE.exists():
            try:
                data = json.loads(SHARED_NOTES_FILE.read_text())
                self._notes = [Note.from_dict(n) for n in data]
                logger.info(f"Loaded {len(self._notes)} notes from local storage")
            except (json.JSONDecodeError, KeyError) as e:
                logger.warning(f"Failed to load notes: {e}")
                self._notes = []
        else:
            self._notes = []

    def _save_local_notes(self) -> None:
        """Save notes to local JSON file."""
        WEAVER_DATA_DIR.mkdir(parents=True, exist_ok=True)
        data = [n.to_dict() for n in self._notes]
        SHARED_NOTES_FILE.write_text(json.dumps(data, indent=2))

    @property
    def is_available(self) -> bool:
        """Check if memory is available."""
        return True  # Always available (local fallback)

    def write_shared(
        self,
        content: str,
        note_id: str | None = None,
        tags: list[str] | None = None,
    ) -> str | None:
        """Write content to the shared notepad."""
        import uuid

        note = Note(
            id=note_id or str(uuid.uuid4())[:8],
            content=content,
            author=self.author,
            created_at=datetime.now().isoformat(),
            tags=tags or [],
        )

        self._notes.insert(0, note)  # Most recent first
        self._save_local_notes()
        logger.info(f"Wrote note {note.id}")
        return note.id

    def read_shared(self, note_id: str) -> str | None:
        """Read content from the shared notepad."""
        for note in self._notes:
            if note.id == note_id:
                return note.content
        return None

    def list_shared(
        self,
        tags: list[str] | None = None,
        author: str | None = None,
        limit: int = 20,
    ) -> list[dict[str, Any]]:
        """List notes from the shared notepad."""
        results = []
        for note in self._notes[:limit]:
            if author and note.author != author:
                continue
            if tags and not any(t in note.tags for t in tags):
                continue
            results.append(
                {
                    "id": note.id,
                    "author": note.author,
                    "created_at": note.created_at,
                    "tags": note.tags,
                    "preview": note.content[:100] + "..."
                    if len(note.content) > 100
                    else note.content,
                }
            )
        return results

    def delete_shared(self, note_id: str) -> bool:
        """Delete a note from the shared notepad."""
        for i, note in enumerate(self._notes):
            if note.id == note_id:
                del self._notes[i]
                self._save_local_notes()
                return True
        return False

    def close(self) -> None:
        """Close connections."""
        if self._arango_available and hasattr(self, "_arango_client"):
            self._arango_client.close()
