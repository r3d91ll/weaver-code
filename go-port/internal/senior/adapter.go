// Package senior defines the interface for senior model adapters.
//
// Senior models are the primary reasoning engines (Claude Code, Anthropic API,
// OpenAI, etc.). They handle complex tasks and delegate simpler work to junior
// models running locally.
//
// Currently implemented:
//   - Claude Code (subprocess) - default
//
// Planned:
//   - Anthropic API (direct HTTP)
package senior

import (
	"context"
	"time"
)

// Provider identifies which senior model backend is in use.
type Provider string

const (
	ProviderClaudeCode  Provider = "claude_code"   // Claude Code CLI subprocess
	ProviderAnthropicAPI Provider = "anthropic_api" // Direct Anthropic API
	// Future: ProviderOpenAI, ProviderGemini, etc.
)

// Message represents a single message in a conversation.
type Message struct {
	Role      string            `json:"role"`      // "user", "assistant", "system"
	Content   string            `json:"content"`   // Message text
	Timestamp time.Time         `json:"timestamp"` // When the message was created
	Metadata  map[string]string `json:"metadata"`  // Optional metadata
}

// Adapter is the interface that all senior model backends must implement.
//
// This abstraction allows Weaver to work with different foundation models
// while keeping the orchestration logic unchanged.
type Adapter interface {
	// Provider returns which backend this adapter uses.
	Provider() Provider

	// Name returns a human-readable name for display.
	Name() string

	// IsAvailable checks if the backend is ready to handle requests.
	// For Claude Code, this checks if the CLI is installed.
	// For API backends, this could check authentication.
	IsAvailable() bool

	// ContextLimit returns the maximum context window in tokens.
	ContextLimit() int

	// Chat sends a message and returns the complete response.
	Chat(ctx context.Context, message string, history []Message) (string, error)

	// ChatStream sends a message and streams the response.
	// Returns two channels: one for content chunks, one for errors.
	// Both channels are closed when the response is complete.
	ChatStream(ctx context.Context, message string, history []Message) (<-chan string, <-chan error)
}

// Config holds configuration common to all adapters.
type Config struct {
	SystemPrompt string  // System prompt to use
	MaxTokens    int     // Max response tokens
	Temperature  float64 // 0.0 - 1.0
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxTokens:   16000,
		Temperature: 0.7,
	}
}
