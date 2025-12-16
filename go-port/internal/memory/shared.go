// Package memory provides shared notepad functionality between agents.
package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Note represents a shared note between agents.
type Note struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Author    string    `json:"author"` // "claude" or "junior"
	CreatedAt time.Time `json:"created_at"`
	Tags      []string  `json:"tags,omitempty"`
}

// SharedMemory provides a JSON-backed notepad for agent coordination.
type SharedMemory struct {
	path  string
	notes []Note
	mu    sync.RWMutex
}

// NewSharedMemory creates or loads shared memory from disk.
func NewSharedMemory() (*SharedMemory, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	weaverDir := filepath.Join(homeDir, ".weaver")
	if err := os.MkdirAll(weaverDir, 0755); err != nil {
		return nil, err
	}

	path := filepath.Join(weaverDir, "shared.json")

	sm := &SharedMemory{
		path:  path,
		notes: make([]Note, 0),
	}

	// Load existing notes if file exists
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &sm.notes) // Ignore errors, start fresh if corrupt
	}

	return sm, nil
}

// Write adds a note to the shared memory.
func (sm *SharedMemory) Write(author, content string, tags []string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	id := generateID()
	note := Note{
		ID:        id,
		Content:   content,
		Author:    author,
		CreatedAt: time.Now(),
		Tags:      tags,
	}

	// Prepend (most recent first)
	sm.notes = append([]Note{note}, sm.notes...)

	// Keep only last 50 notes
	if len(sm.notes) > 50 {
		sm.notes = sm.notes[:50]
	}

	sm.save()
	return id
}

// Read retrieves a note by ID.
func (sm *SharedMemory) Read(id string) (Note, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, note := range sm.notes {
		if note.ID == id {
			return note, true
		}
	}
	return Note{}, false
}

// List returns recent notes, optionally filtered.
func (sm *SharedMemory) List(limit int, author string, tags []string) []Note {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var result []Note
	for _, note := range sm.notes {
		// Filter by author
		if author != "" && note.Author != author {
			continue
		}

		// Filter by tags
		if len(tags) > 0 && !hasAnyTag(note.Tags, tags) {
			continue
		}

		result = append(result, note)
		if len(result) >= limit {
			break
		}
	}

	return result
}

// Delete removes a note by ID.
func (sm *SharedMemory) Delete(id string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i, note := range sm.notes {
		if note.ID == id {
			sm.notes = append(sm.notes[:i], sm.notes[i+1:]...)
			sm.save()
			return true
		}
	}
	return false
}

// Clear removes all notes.
func (sm *SharedMemory) Clear() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.notes = make([]Note, 0)
	sm.save()
}

// FormatForPrompt returns notes formatted for injection into a prompt.
func (sm *SharedMemory) FormatForPrompt(limit int) string {
	notes := sm.List(limit, "", nil)
	if len(notes) == 0 {
		return ""
	}

	result := "## Shared Notes\n\n"
	for _, note := range notes {
		result += "- [" + note.Author + "] " + truncate(note.Content, 200) + "\n"
	}
	return result
}

// save persists notes to disk.
func (sm *SharedMemory) save() {
	data, err := json.MarshalIndent(sm.notes, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(sm.path, data, 0644)
}

// generateID creates a short unique ID.
func generateID() string {
	return time.Now().Format("20060102150405")
}

// hasAnyTag checks if note has any of the given tags.
func hasAnyTag(noteTags, filterTags []string) bool {
	for _, ft := range filterTags {
		for _, nt := range noteTags {
			if ft == nt {
				return true
			}
		}
	}
	return false
}

// truncate shortens a string to max length.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
