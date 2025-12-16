// Package context provides context window management and compaction.
package context

import (
	"fmt"
	"time"
)

// Message represents a message in context history.
type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// Config defines thresholds for context management.
type Config struct {
	MaxTokens     int     // Model's context limit
	CompactAt     float64 // Trigger compaction at this % (e.g., 0.8 = 80%)
	MinForCompact int     // Don't compact if model has less than this context
}

// DefaultClaudeConfig returns config for Claude Code (~200k context).
func DefaultClaudeConfig() Config {
	return Config{
		MaxTokens:     200000,
		CompactAt:     0.8,
		MinForCompact: 0, // Always allow compaction
	}
}

// DefaultLocalConfig returns config for local models.
func DefaultLocalConfig(contextSize int) Config {
	return Config{
		MaxTokens:     contextSize,
		CompactAt:     0.8,
		MinForCompact: 32000, // Only compact if model has 32k+ context
	}
}

// Manager tracks context usage and handles compaction.
type Manager struct {
	config          Config
	messages        []Message
	estimatedTokens int
}

// NewManager creates a new context manager.
func NewManager(cfg Config) *Manager {
	return &Manager{
		config:   cfg,
		messages: make([]Message, 0),
	}
}

// Add adds a message to the context, updating token estimate.
func (m *Manager) Add(msg Message) {
	m.messages = append(m.messages, msg)
	m.estimatedTokens += EstimateTokens(msg.Content)
}

// Messages returns all messages in the context.
func (m *Manager) Messages() []Message {
	return m.messages
}

// EstimatedTokens returns the current token estimate.
func (m *Manager) EstimatedTokens() int {
	return m.estimatedTokens
}

// ShouldCompact returns true if context should be compacted.
func (m *Manager) ShouldCompact() bool {
	// Don't compact if model context is too small
	if m.config.MaxTokens < m.config.MinForCompact {
		return false
	}
	threshold := float64(m.config.MaxTokens) * m.config.CompactAt
	return float64(m.estimatedTokens) > threshold
}

// ShouldTruncate returns true if we should truncate (for small context models).
func (m *Manager) ShouldTruncate() bool {
	// For small context models, truncate instead of compact
	if m.config.MaxTokens < m.config.MinForCompact {
		threshold := float64(m.config.MaxTokens) * m.config.CompactAt
		return float64(m.estimatedTokens) > threshold
	}
	return false
}

// Truncate removes oldest non-system messages to fit within limit.
func (m *Manager) Truncate() {
	targetTokens := int(float64(m.config.MaxTokens) * 0.6) // Truncate to 60%

	// Separate system messages from others
	var system, other []Message
	for _, msg := range m.messages {
		if msg.Role == "system" {
			system = append(system, msg)
		} else {
			other = append(other, msg)
		}
	}

	// Remove from beginning of non-system messages until under target
	for m.estimatedTokens > targetTokens && len(other) > 2 {
		removed := other[0]
		other = other[1:]
		m.estimatedTokens -= EstimateTokens(removed.Content)
	}

	// Reconstruct messages: system first, then remaining others
	m.messages = append(system, other...)
}

// CompactionPrompt returns the prompt to ask the model to summarize.
func (m *Manager) CompactionPrompt() string {
	return `Summarize this conversation so far. Preserve:
- Key decisions made
- Current task state
- Important code/file references
- File paths mentioned
- Any errors or issues encountered
- Notes for Junior/Senior Engineer

Keep it dense but complete. This summary will start the next conversation.`
}

// ResetWithSummary clears context and starts fresh with a summary.
func (m *Manager) ResetWithSummary(summary string) {
	m.messages = []Message{
		{
			Role:      "system",
			Content:   fmt.Sprintf("## Previous Session Summary\n\n%s\n\n---\nContinuing conversation...", summary),
			Timestamp: time.Now(),
		},
	}
	m.estimatedTokens = EstimateTokens(summary) + 50 // +50 for wrapper text
}

// Clear removes all messages.
func (m *Manager) Clear() {
	m.messages = make([]Message, 0)
	m.estimatedTokens = 0
}

// EstimateTokens provides a rough token count for a string.
// Uses ~4 chars per token heuristic (works reasonably for English).
func EstimateTokens(s string) int {
	return len(s) / 4
}
