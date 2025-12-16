package senior

import (
	"testing"
	"time"
)

func TestProviderConstants(t *testing.T) {
	if ProviderClaudeCode != "claude_code" {
		t.Errorf("Unexpected ProviderClaudeCode value: %s", ProviderClaudeCode)
	}

	if ProviderAnthropicAPI != "anthropic_api" {
		t.Errorf("Unexpected ProviderAnthropicAPI value: %s", ProviderAnthropicAPI)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxTokens != 16000 {
		t.Errorf("Expected MaxTokens=16000, got %d", cfg.MaxTokens)
	}

	if cfg.Temperature != 0.7 {
		t.Errorf("Expected Temperature=0.7, got %f", cfg.Temperature)
	}
}

func TestMessageStruct(t *testing.T) {
	now := time.Now()
	msg := Message{
		Role:      "user",
		Content:   "Hello",
		Timestamp: now,
		Metadata:  map[string]string{"key": "value"},
	}

	if msg.Role != "user" {
		t.Error("Role mismatch")
	}

	if msg.Content != "Hello" {
		t.Error("Content mismatch")
	}

	if msg.Timestamp != now {
		t.Error("Timestamp mismatch")
	}

	if msg.Metadata["key"] != "value" {
		t.Error("Metadata mismatch")
	}
}
