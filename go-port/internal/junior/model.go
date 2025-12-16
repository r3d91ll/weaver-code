// Package junior provides clients for local models used as junior engineers.
//
// Junior models handle delegated tasks from the senior model. They connect
// to local inference servers via OpenAI-compatible HTTP APIs.
//
// Supported backends:
//   - Ollama (default port 11434)
//   - LM Studio (default port 1234)
//   - vLLM, LocalAI, or any OpenAI-compatible server
package junior

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/r3d91ll/weaver-code/internal/telemetry"
)

// Message represents a chat message for the junior model.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Model represents a local model client.
type Model struct {
	name         string
	baseURL      string
	model        string
	systemPrompt string
	contextLimit int
	maxTokens    int
	temperature  float64
	httpClient   *http.Client
	tools        []map[string]interface{} // Tool schemas for function calling
	toolsEnabled bool
}

// Config holds configuration for a junior model.
type Config struct {
	Name         string        // Display name
	BaseURL      string        // API endpoint (e.g., http://localhost:11434/v1)
	Model        string        // Model name (e.g., qwen2.5-coder:7b)
	SystemPrompt string        // System prompt for junior role
	ContextLimit int           // Context window size
	MaxTokens    int           // Max response tokens
	Temperature  float64       // Sampling temperature
	Timeout      time.Duration // HTTP request timeout (default 120s)
}

// DefaultConfig returns sensible defaults for a local model.
// Uses Ollama by default (port 11434) with gpt-oss:20b-weaver model.
func DefaultConfig() Config {
	return Config{
		Name:         "gpt-oss:20b-weaver",
		BaseURL:      "http://localhost:11434/v1",
		Model:        "gpt-oss:20b-weaver",
		ContextLimit: 131072,
		MaxTokens:    8192,
		Temperature:  0.7,
	}
}

// New creates a new junior model client.
func New(cfg Config) *Model {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second // Default timeout
	}

	return &Model{
		name:         cfg.Name,
		baseURL:      strings.TrimSuffix(cfg.BaseURL, "/"),
		model:        cfg.Model,
		systemPrompt: cfg.SystemPrompt,
		contextLimit: cfg.ContextLimit,
		maxTokens:    cfg.MaxTokens,
		temperature:  cfg.Temperature,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Name returns the model's display name.
func (m *Model) Name() string {
	return m.name
}

// BaseURL returns the model's API endpoint URL.
func (m *Model) BaseURL() string {
	return m.baseURL
}

// ContextLimit returns the model's context window size.
func (m *Model) ContextLimit() int {
	return m.contextLimit
}

// SetSystemPrompt updates the model's system prompt.
// This is used to inject JUNIOR.md content after assessment.
func (m *Model) SetSystemPrompt(prompt string) {
	m.systemPrompt = prompt
}

// SystemPrompt returns the current system prompt.
func (m *Model) SystemPrompt() string {
	return m.systemPrompt
}

// SetTools sets the available tools for the model.
func (m *Model) SetTools(tools []map[string]interface{}) {
	m.tools = tools
	m.toolsEnabled = len(tools) > 0
}

// ToolsEnabled returns whether tools are enabled.
func (m *Model) ToolsEnabled() bool {
	return m.toolsEnabled
}

// IsAvailable checks if the model server is reachable.
func (m *Model) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", m.baseURL+"/models", nil)
	if err != nil {
		return false
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// Chat sends a message and returns the complete response.
func (m *Model) Chat(ctx context.Context, message string, history []Message) (string, error) {
	// Start telemetry span
	ctx, span := telemetry.StartLLMSpan(ctx, "junior.chat", m.model, "junior")
	defer span.End()
	span.SetInput(message)

	messages := m.buildMessages(message, history)

	reqBody := chatRequest{
		Model:       m.model,
		Messages:    messages,
		MaxTokens:   m.maxTokens,
		Temperature: m.temperature,
		Stream:      false,
		Tools:       m.tools, // Include tools if set
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		span.SetError(err)
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		span.SetError(err)
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		span.SetError(err)
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
		span.SetError(err)
		return "", err
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		span.SetError(err)
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		err := fmt.Errorf("no choices in response")
		span.SetError(err)
		return "", err
	}

	response := chatResp.Choices[0].Message.Content

	// Check if Devstral wrapped the response in tool call format
	// This can happen when the model tries to use write_file to output code
	if IsMistralToolCallFormat(response) {
		response = ExtractCodeFromMistralResponse(response)
	}

	// If content is empty but tool_calls exist, extract code from write_file
	// Devstral often returns code via tool calls even for non-tool challenges
	if response == "" && len(chatResp.Choices[0].Message.ToolCalls) > 0 {
		response = extractCodeFromToolCalls(chatResp.Choices[0].Message.ToolCalls)
	}

	span.SetOutput(response)
	span.SetTokens(telemetry.EstimateTokens(message), telemetry.EstimateTokens(response))

	return response, nil
}

// ChatResponse contains the model's response, including any tool calls.
type ChatResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}

// ChatWithTools sends a message and returns response with tool calls.
// Use this for tool-enabled conversations where the model may request tool execution.
func (m *Model) ChatWithTools(ctx context.Context, messages []chatMessage) (*ChatResponse, error) {
	// Start telemetry span
	ctx, span := telemetry.StartLLMSpan(ctx, "junior.chat_with_tools", m.model, "junior")
	defer span.End()

	reqBody := chatRequest{
		Model:       m.model,
		Messages:    messages,
		MaxTokens:   m.maxTokens,
		Temperature: m.temperature,
		Stream:      false,
		Tools:       m.tools,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		span.SetError(err)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		span.SetError(err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		span.SetError(err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
		span.SetError(err)
		return nil, err
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		span.SetError(err)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		err := fmt.Errorf("no choices in response")
		span.SetError(err)
		return nil, err
	}

	content := chatResp.Choices[0].Message.Content
	toolCalls := chatResp.Choices[0].Message.ToolCalls

	// Check for Mistral tool call format in content
	// Devstral models use [TOOL_CALLS]func[ARGS]{json} format
	if len(toolCalls) == 0 && IsMistralToolCallFormat(content) {
		content, toolCalls = ConvertMistralResponse(content, toolCalls)
	}

	result := &ChatResponse{
		Content:      content,
		ToolCalls:    toolCalls,
		FinishReason: chatResp.Choices[0].FinishReason,
	}

	span.SetOutput(result.Content)
	return result, nil
}

// BuildMessages creates a message list with system prompt for tool-enabled chat.
func (m *Model) BuildMessages(userMessage string, history []Message) []chatMessage {
	return m.buildMessages(userMessage, history)
}

// CreateToolResultMessage creates a message containing tool results.
func CreateToolResultMessage(toolCallID, content string) chatMessage {
	return chatMessage{
		Role:       "tool",
		Content:    content,
		ToolCallID: toolCallID,
	}
}

// CreateAssistantMessage creates an assistant message (for adding tool calls to history).
func CreateAssistantMessage(content string, toolCalls []ToolCall) chatMessage {
	return chatMessage{
		Role:      "assistant",
		Content:   content,
		ToolCalls: toolCalls,
	}
}

// ChatStream sends a message and streams the response.
func (m *Model) ChatStream(ctx context.Context, message string, history []Message) (<-chan string, <-chan error) {
	chunks := make(chan string, 100)
	errs := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errs)

		messages := m.buildMessages(message, history)

		reqBody := chatRequest{
			Model:       m.model,
			Messages:    messages,
			MaxTokens:   m.maxTokens,
			Temperature: m.temperature,
			Stream:      true,
		}

		body, err := json.Marshal(reqBody)
		if err != nil {
			errs <- fmt.Errorf("failed to marshal request: %w", err)
			return
		}

		req, err := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			errs <- fmt.Errorf("failed to create request: %w", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")

		resp, err := m.httpClient.Do(req)
		if err != nil {
			errs <- fmt.Errorf("request failed: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			errs <- fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
			return
		}

		// Parse SSE stream
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var streamResp chatStreamResponse
			if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
				continue
			}

			if len(streamResp.Choices) > 0 {
				content := streamResp.Choices[0].Delta.Content
				if content != "" {
					chunks <- content
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errs <- fmt.Errorf("error reading stream: %w", err)
		}
	}()

	return chunks, errs
}

// buildMessages constructs the messages array for the API.
func (m *Model) buildMessages(message string, history []Message) []chatMessage {
	var messages []chatMessage

	// Add system prompt if configured
	if m.systemPrompt != "" {
		messages = append(messages, chatMessage{
			Role:    "system",
			Content: m.systemPrompt,
		})
	}

	// Add history
	for _, msg := range history {
		messages = append(messages, chatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Add current message
	messages = append(messages, chatMessage{
		Role:    "user",
		Content: message,
	})

	return messages
}

// OpenAI API types

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a function call from the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction contains the function name and arguments.
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatRequest struct {
	Model       string                   `json:"model"`
	Messages    []chatMessage            `json:"messages"`
	MaxTokens   int                      `json:"max_tokens"`
	Temperature float64                  `json:"temperature"`
	Stream      bool                     `json:"stream"`
	Tools       []map[string]interface{} `json:"tools,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
}

type chatStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// extractCodeFromToolCalls extracts code content from OpenAI-format tool calls.
// This handles the case where models like Devstral return code via write_file
// tool calls with empty content field.
// Falls back to context_write content if no write_file is found.
func extractCodeFromToolCalls(toolCalls []ToolCall) string {
	// First priority: look for write_file tool calls (actual code output)
	for _, tc := range toolCalls {
		if tc.Function.Name == "write_file" {
			code := extractContentFromWriteFileArgs(tc.Function.Arguments)
			if code != "" {
				return code
			}
		}
	}

	// Second priority: look for context_write tool calls (may contain code or analysis)
	// Devstral often uses context_write to report progress/results
	for _, tc := range toolCalls {
		if tc.Function.Name == "context_write" {
			content := extractContentFieldFromArgs(tc.Function.Arguments)
			if content != "" {
				return content
			}
		}
	}

	return ""
}

// extractContentFieldFromArgs extracts the "content" field from any tool arguments.
func extractContentFieldFromArgs(argsJSON string) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	if content, ok := args["content"].(string); ok {
		return content
	}
	return ""
}

// extractContentFromWriteFileArgs extracts the "content" field from write_file arguments.
func extractContentFromWriteFileArgs(argsJSON string) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		// Try extracting from potentially truncated JSON
		return extractContentFromArgs(argsJSON)
	}
	if content, ok := args["content"].(string); ok {
		return content
	}
	return ""
}
