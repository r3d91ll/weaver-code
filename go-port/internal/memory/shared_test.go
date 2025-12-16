package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestMemory(t *testing.T) (*SharedMemory, func()) {
	t.Helper()

	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "weaver-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Override the path
	sm := &SharedMemory{
		path:  filepath.Join(tmpDir, "shared.json"),
		notes: make([]Note, 0),
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return sm, cleanup
}

func TestSharedMemoryWrite(t *testing.T) {
	sm, cleanup := setupTestMemory(t)
	defer cleanup()

	id := sm.Write("claude", "Test note content", []string{"test"})

	if id == "" {
		t.Error("Expected non-empty ID")
	}

	// Verify note was added
	notes := sm.List(10, "", nil)
	if len(notes) != 1 {
		t.Errorf("Expected 1 note, got %d", len(notes))
	}

	if notes[0].Content != "Test note content" {
		t.Errorf("Expected content 'Test note content', got '%s'", notes[0].Content)
	}

	if notes[0].Author != "claude" {
		t.Errorf("Expected author 'claude', got '%s'", notes[0].Author)
	}
}

func TestSharedMemoryRead(t *testing.T) {
	sm, cleanup := setupTestMemory(t)
	defer cleanup()

	id := sm.Write("junior", "Read me", nil)

	note, found := sm.Read(id)
	if !found {
		t.Error("Expected to find note by ID")
	}

	if note.Content != "Read me" {
		t.Errorf("Expected content 'Read me', got '%s'", note.Content)
	}

	// Test reading non-existent note
	_, found = sm.Read("nonexistent")
	if found {
		t.Error("Expected not to find non-existent note")
	}
}

func TestSharedMemoryList(t *testing.T) {
	sm, cleanup := setupTestMemory(t)
	defer cleanup()

	sm.Write("claude", "Note 1", []string{"tag1"})
	sm.Write("junior", "Note 2", []string{"tag2"})
	sm.Write("claude", "Note 3", []string{"tag1", "tag2"})

	// List all
	notes := sm.List(10, "", nil)
	if len(notes) != 3 {
		t.Errorf("Expected 3 notes, got %d", len(notes))
	}

	// Filter by author
	claudeNotes := sm.List(10, "claude", nil)
	if len(claudeNotes) != 2 {
		t.Errorf("Expected 2 claude notes, got %d", len(claudeNotes))
	}

	// Filter by tag
	tag1Notes := sm.List(10, "", []string{"tag1"})
	if len(tag1Notes) != 2 {
		t.Errorf("Expected 2 notes with tag1, got %d", len(tag1Notes))
	}

	// Test limit
	limitedNotes := sm.List(2, "", nil)
	if len(limitedNotes) != 2 {
		t.Errorf("Expected 2 notes with limit, got %d", len(limitedNotes))
	}
}

func TestSharedMemoryDelete(t *testing.T) {
	sm, cleanup := setupTestMemory(t)
	defer cleanup()

	id := sm.Write("claude", "Delete me", nil)

	// Verify exists
	_, found := sm.Read(id)
	if !found {
		t.Error("Note should exist before delete")
	}

	// Delete
	deleted := sm.Delete(id)
	if !deleted {
		t.Error("Expected Delete to return true")
	}

	// Verify gone
	_, found = sm.Read(id)
	if found {
		t.Error("Note should not exist after delete")
	}

	// Delete non-existent
	deleted = sm.Delete("nonexistent")
	if deleted {
		t.Error("Expected Delete to return false for non-existent note")
	}
}

func TestSharedMemoryClear(t *testing.T) {
	sm, cleanup := setupTestMemory(t)
	defer cleanup()

	sm.Write("claude", "Note 1", nil)
	sm.Write("junior", "Note 2", nil)

	sm.Clear()

	notes := sm.List(10, "", nil)
	if len(notes) != 0 {
		t.Errorf("Expected 0 notes after clear, got %d", len(notes))
	}
}

func TestSharedMemoryFormatForPrompt(t *testing.T) {
	sm, cleanup := setupTestMemory(t)
	defer cleanup()

	// Empty memory
	prompt := sm.FormatForPrompt(5)
	if prompt != "" {
		t.Errorf("Expected empty prompt for empty memory, got '%s'", prompt)
	}

	// Add notes
	sm.Write("claude", "Remember this", nil)
	sm.Write("junior", "And this", nil)

	prompt = sm.FormatForPrompt(5)
	if prompt == "" {
		t.Error("Expected non-empty prompt")
	}

	// Should contain header
	if len(prompt) < 10 {
		t.Error("Prompt too short")
	}
}

func TestSharedMemoryMaxNotes(t *testing.T) {
	sm, cleanup := setupTestMemory(t)
	defer cleanup()

	// Write more than 50 notes
	for i := 0; i < 60; i++ {
		sm.Write("test", "Note content", nil)
	}

	notes := sm.List(100, "", nil)
	if len(notes) > 50 {
		t.Errorf("Expected max 50 notes, got %d", len(notes))
	}
}
