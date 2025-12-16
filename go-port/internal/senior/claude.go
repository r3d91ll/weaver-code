package senior

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/r3d91ll/weaver-code/internal/telemetry"
)

// ClaudeCode wraps the Claude Code CLI as a senior model adapter.
//
// This is the default and recommended adapter for Weaver. It spawns the
// `claude` CLI as a subprocess, leveraging existing authentication and
// the full Claude Code toolset (file ops, shell, git, etc.).
type ClaudeCode struct {
	config       Config
	systemPrompt string
	contextLimit int
}

// NewClaudeCode creates a new Claude Code adapter.
func NewClaudeCode(systemPrompt string) *ClaudeCode {
	return &ClaudeCode{
		config:       DefaultConfig(),
		systemPrompt: systemPrompt,
		contextLimit: 200000, // Claude's context window
	}
}

// Provider returns the adapter type.
func (c *ClaudeCode) Provider() Provider {
	return ProviderClaudeCode
}

// Name returns a human-readable name.
func (c *ClaudeCode) Name() string {
	return "Claude Code"
}

// IsAvailable checks if the claude CLI is installed and accessible.
func (c *ClaudeCode) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "--version")
	err := cmd.Run()
	return err == nil
}

// ContextLimit returns Claude's context window size.
func (c *ClaudeCode) ContextLimit() int {
	return c.contextLimit
}

// Chat sends a message and returns the complete response.
func (c *ClaudeCode) Chat(ctx context.Context, message string, history []Message) (string, error) {
	// Start telemetry span
	ctx, span := telemetry.StartLLMSpan(ctx, "senior.chat", "claude", "senior")
	defer span.End()
	span.SetInput(message)

	prompt := c.buildPrompt(message, history)

	cmd := exec.CommandContext(ctx, "claude", "-p", "--output-format", "json")
	if c.systemPrompt != "" {
		cmd.Args = append(cmd.Args, "--system-prompt", c.systemPrompt)
	}

	cmd.Stdin = strings.NewReader(prompt)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			err := fmt.Errorf("claude error: %s", string(exitErr.Stderr))
			span.SetError(err)
			return "", err
		}
		span.SetError(err)
		return "", fmt.Errorf("failed to run claude: %w", err)
	}

	// Parse JSON response
	var resp struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		// If not JSON, return raw output
		response := strings.TrimSpace(string(output))
		span.SetOutput(response)
		span.SetTokens(telemetry.EstimateTokens(prompt), telemetry.EstimateTokens(response))
		return response, nil
	}

	span.SetOutput(resp.Result)
	span.SetTokens(telemetry.EstimateTokens(prompt), telemetry.EstimateTokens(resp.Result))
	return resp.Result, nil
}

// ChatStream sends a message and streams the response.
func (c *ClaudeCode) ChatStream(ctx context.Context, message string, history []Message) (<-chan string, <-chan error) {
	chunks := make(chan string, 100)
	errs := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errs)

		// Start telemetry span
		_, span := telemetry.StartLLMSpan(ctx, "senior.chat_stream", "claude", "senior")
		defer span.End()
		span.SetInput(message)

		prompt := c.buildPrompt(message, history)
		var fullResponse strings.Builder

		cmd := exec.CommandContext(ctx, "claude",
			"-p",
			"--verbose",
			"--output-format", "stream-json",
			"--dangerously-skip-permissions",
		)
		if c.systemPrompt != "" {
			cmd.Args = append(cmd.Args, "--system-prompt", c.systemPrompt)
		}

		stdin, err := cmd.StdinPipe()
		if err != nil {
			span.SetError(err)
			errs <- fmt.Errorf("failed to get stdin pipe: %w", err)
			return
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			span.SetError(err)
			errs <- fmt.Errorf("failed to get stdout pipe: %w", err)
			return
		}

		if err := cmd.Start(); err != nil {
			span.SetError(err)
			errs <- fmt.Errorf("failed to start claude: %w", err)
			return
		}

		// Send prompt
		_, err = stdin.Write([]byte(prompt))
		if err != nil {
			span.SetError(err)
			errs <- fmt.Errorf("failed to write prompt: %w", err)
			return
		}
		stdin.Close()

		// Read streaming response
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var event struct {
				Type  string `json:"type"`
				Delta struct {
					Text string `json:"text"`
				} `json:"delta"`
				Result string `json:"result"`
			}

			if err := json.Unmarshal([]byte(line), &event); err != nil {
				// Not JSON, send as-is
				chunks <- line
				fullResponse.WriteString(line)
				continue
			}

			switch event.Type {
			case "content_block_delta":
				if event.Delta.Text != "" {
					chunks <- event.Delta.Text
					fullResponse.WriteString(event.Delta.Text)
				}
			case "message_delta":
				// End of message
			default:
				if event.Result != "" {
					chunks <- event.Result
					fullResponse.WriteString(event.Result)
				}
			}
		}

		if err := scanner.Err(); err != nil {
			span.SetError(err)
			errs <- fmt.Errorf("error reading output: %w", err)
			return
		}

		if err := cmd.Wait(); err != nil {
			// Don't report error if context was cancelled
			if ctx.Err() == nil {
				span.SetError(err)
				errs <- fmt.Errorf("claude exited with error: %w", err)
			}
		}

		// Record the full response
		response := fullResponse.String()
		span.SetOutput(response)
		span.SetTokens(telemetry.EstimateTokens(prompt), telemetry.EstimateTokens(response))
	}()

	return chunks, errs
}

// buildPrompt constructs the full prompt with conversation history.
func (c *ClaudeCode) buildPrompt(message string, history []Message) string {
	var parts []string

	for _, msg := range history {
		switch msg.Role {
		case "user":
			parts = append(parts, fmt.Sprintf("User: %s", msg.Content))
		case "assistant":
			parts = append(parts, fmt.Sprintf("Assistant: %s", msg.Content))
		}
	}

	parts = append(parts, fmt.Sprintf("User: %s", message))
	parts = append(parts, "Assistant:")

	return strings.Join(parts, "\n\n")
}
