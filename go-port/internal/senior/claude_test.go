package senior

import (
	"testing"
)

func TestNewClaudeCode(t *testing.T) {
	prompt := "You are a test assistant"
	c := NewClaudeCode(prompt)

	if c.Provider() != ProviderClaudeCode {
		t.Errorf("Expected provider %s, got %s", ProviderClaudeCode, c.Provider())
	}

	if c.Name() != "Claude Code" {
		t.Errorf("Expected name 'Claude Code', got '%s'", c.Name())
	}

	if c.ContextLimit() != 200000 {
		t.Errorf("Expected context limit 200000, got %d", c.ContextLimit())
	}

	if c.systemPrompt != prompt {
		t.Errorf("System prompt not set correctly")
	}
}

func TestClaudeCodeBuildPrompt(t *testing.T) {
	c := NewClaudeCode("")

	// Test with no history
	prompt := c.buildPrompt("Hello", nil)
	expected := "User: Hello\n\nAssistant:"
	if prompt != expected {
		t.Errorf("Expected '%s', got '%s'", expected, prompt)
	}

	// Test with history
	history := []Message{
		{Role: "user", Content: "Previous"},
		{Role: "assistant", Content: "Response"},
	}
	prompt = c.buildPrompt("New message", history)

	if len(prompt) == 0 {
		t.Error("Expected non-empty prompt")
	}

	// Should contain history
	if !containsSubstring(prompt, "Previous") {
		t.Error("Prompt should contain history")
	}

	if !containsSubstring(prompt, "Response") {
		t.Error("Prompt should contain assistant response")
	}

	if !containsSubstring(prompt, "New message") {
		t.Error("Prompt should contain current message")
	}
}

func TestClaudeCodeIsAvailable(t *testing.T) {
	c := NewClaudeCode("")

	// This test checks if claude CLI is available
	// On systems without claude installed, this will be false
	available := c.IsAvailable()

	// We just verify it doesn't panic
	t.Logf("Claude CLI available: %v", available)
}

// Helper function
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
