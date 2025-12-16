package context

import (
	"testing"
	"time"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"test", 1},           // 4 chars = 1 token
		{"hello world", 2},    // 11 chars = 2 tokens
		{"a", 0},              // 1 char = 0 tokens (integer division)
		{"abcdefgh", 2},       // 8 chars = 2 tokens
	}

	for _, tt := range tests {
		got := EstimateTokens(tt.input)
		if got != tt.expected {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestManagerAdd(t *testing.T) {
	cfg := Config{MaxTokens: 1000, CompactAt: 0.8, MinForCompact: 0}
	m := NewManager(cfg)

	msg := Message{Role: "user", Content: "hello world", Timestamp: time.Now()}
	m.Add(msg)

	if len(m.Messages()) != 1 {
		t.Errorf("Expected 1 message, got %d", len(m.Messages()))
	}

	if m.EstimatedTokens() != EstimateTokens("hello world") {
		t.Errorf("Token count mismatch")
	}
}

func TestManagerShouldCompact(t *testing.T) {
	cfg := Config{MaxTokens: 100, CompactAt: 0.8, MinForCompact: 0}
	m := NewManager(cfg)

	// Add messages until we exceed 80% of 100 tokens
	for i := 0; i < 20; i++ {
		m.Add(Message{Role: "user", Content: "hello world test message", Timestamp: time.Now()})
	}

	if !m.ShouldCompact() {
		t.Error("Expected ShouldCompact() to be true after adding many messages")
	}
}

func TestManagerShouldNotCompactSmallContext(t *testing.T) {
	// Model with context smaller than MinForCompact
	cfg := Config{MaxTokens: 100, CompactAt: 0.8, MinForCompact: 32000}
	m := NewManager(cfg)

	// Add messages to exceed 80% of 100 tokens (80 tokens)
	// Each "hello world test message" is ~6 tokens (24 chars / 4)
	for i := 0; i < 20; i++ {
		m.Add(Message{Role: "user", Content: "hello world test message", Timestamp: time.Now()})
	}

	// Should NOT compact because model context is too small
	if m.ShouldCompact() {
		t.Error("Expected ShouldCompact() to be false for small context model")
	}

	// But SHOULD truncate because we're over threshold
	if !m.ShouldTruncate() {
		t.Errorf("Expected ShouldTruncate() to be true for small context model over limit (tokens: %d, threshold: 80)", m.EstimatedTokens())
	}
}

func TestManagerTruncate(t *testing.T) {
	cfg := Config{MaxTokens: 100, CompactAt: 0.8, MinForCompact: 32000}
	m := NewManager(cfg)

	// Add system message
	m.Add(Message{Role: "system", Content: "you are a helper", Timestamp: time.Now()})

	// Add many user/assistant messages
	for i := 0; i < 50; i++ {
		m.Add(Message{Role: "user", Content: "hello world", Timestamp: time.Now()})
	}

	originalCount := len(m.Messages())
	m.Truncate()

	// Should have fewer messages
	if len(m.Messages()) >= originalCount {
		t.Error("Expected truncation to reduce message count")
	}

	// System message should still be first
	if m.Messages()[0].Role != "system" {
		t.Error("Expected system message to be preserved at front")
	}
}

func TestManagerClear(t *testing.T) {
	cfg := Config{MaxTokens: 1000, CompactAt: 0.8, MinForCompact: 0}
	m := NewManager(cfg)

	m.Add(Message{Role: "user", Content: "hello", Timestamp: time.Now()})
	m.Add(Message{Role: "assistant", Content: "hi", Timestamp: time.Now()})

	m.Clear()

	if len(m.Messages()) != 0 {
		t.Errorf("Expected 0 messages after Clear, got %d", len(m.Messages()))
	}

	if m.EstimatedTokens() != 0 {
		t.Errorf("Expected 0 tokens after Clear, got %d", m.EstimatedTokens())
	}
}

func TestManagerResetWithSummary(t *testing.T) {
	cfg := Config{MaxTokens: 1000, CompactAt: 0.8, MinForCompact: 0}
	m := NewManager(cfg)

	m.Add(Message{Role: "user", Content: "hello", Timestamp: time.Now()})
	m.Add(Message{Role: "assistant", Content: "hi", Timestamp: time.Now()})

	m.ResetWithSummary("This was a greeting exchange")

	if len(m.Messages()) != 1 {
		t.Errorf("Expected 1 message after reset, got %d", len(m.Messages()))
	}

	if m.Messages()[0].Role != "system" {
		t.Error("Expected system message after reset")
	}
}

func TestDefaultConfigs(t *testing.T) {
	claudeCfg := DefaultClaudeConfig()
	if claudeCfg.MaxTokens != 200000 {
		t.Errorf("Expected Claude MaxTokens=200000, got %d", claudeCfg.MaxTokens)
	}

	localCfg := DefaultLocalConfig(32000)
	if localCfg.MaxTokens != 32000 {
		t.Errorf("Expected local MaxTokens=32000, got %d", localCfg.MaxTokens)
	}
	if localCfg.MinForCompact != 32000 {
		t.Errorf("Expected local MinForCompact=32000, got %d", localCfg.MinForCompact)
	}
}
