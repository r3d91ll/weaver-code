package junior

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewModel(t *testing.T) {
	cfg := Config{
		Name:         "test-model",
		BaseURL:      "http://localhost:1234/v1",
		Model:        "llama3",
		ContextLimit: 8000,
		MaxTokens:    2048,
		Temperature:  0.5,
	}

	m := New(cfg)

	if m.Name() != "test-model" {
		t.Errorf("Expected name 'test-model', got '%s'", m.Name())
	}

	if m.ContextLimit() != 8000 {
		t.Errorf("Expected context limit 8000, got %d", m.ContextLimit())
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("Unexpected default BaseURL: %s", cfg.BaseURL)
	}

	if cfg.ContextLimit != 131072 {
		t.Errorf("Expected default ContextLimit=131072, got %d", cfg.ContextLimit)
	}

	if cfg.Model != "gpt-oss:20b-weaver" {
		t.Errorf("Expected default Model='gpt-oss:20b-weaver', got '%s'", cfg.Model)
	}
}

func TestModelIsAvailable(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"models": []string{}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := Config{
		Name:    "test",
		BaseURL: server.URL,
		Model:   "test",
	}
	m := New(cfg)

	if !m.IsAvailable() {
		t.Error("Expected model to be available")
	}

	// Test with bad server
	badCfg := Config{
		Name:    "test",
		BaseURL: "http://localhost:99999/v1",
		Model:   "test",
	}
	badModel := New(badCfg)

	if badModel.IsAvailable() {
		t.Error("Expected model to be unavailable with bad URL")
	}
}

func TestModelChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" && r.Method == "POST" {
			resp := map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"message": map[string]string{
							"role":    "assistant",
							"content": "Hello! How can I help?",
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := Config{
		Name:        "test",
		BaseURL:     server.URL,
		Model:       "test-model",
		MaxTokens:   100,
		Temperature: 0.7,
	}
	m := New(cfg)

	ctx := context.Background()
	response, err := m.Chat(ctx, "Hello", nil)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if response != "Hello! How can I help?" {
		t.Errorf("Unexpected response: %s", response)
	}
}

func TestModelChatWithHistory(t *testing.T) {
	var receivedMessages []chatMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			var req chatRequest
			json.NewDecoder(r.Body).Decode(&req)
			receivedMessages = req.Messages

			resp := map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"role": "assistant", "content": "OK"}},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := Config{
		Name:         "test",
		BaseURL:      server.URL,
		Model:        "test",
		SystemPrompt: "You are a helper",
	}
	m := New(cfg)

	history := []Message{
		{Role: "user", Content: "Previous question"},
		{Role: "assistant", Content: "Previous answer"},
	}

	ctx := context.Background()
	_, err := m.Chat(ctx, "New question", history)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify message order: system, history, current
	if len(receivedMessages) != 4 {
		t.Errorf("Expected 4 messages (system + 2 history + current), got %d", len(receivedMessages))
	}

	if receivedMessages[0].Role != "system" {
		t.Errorf("First message should be system, got %s", receivedMessages[0].Role)
	}

	if receivedMessages[3].Content != "New question" {
		t.Errorf("Last message should be current question")
	}
}

func TestModelChatError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal error"))
	}))
	defer server.Close()

	cfg := Config{
		Name:    "test",
		BaseURL: server.URL,
		Model:   "test",
	}
	m := New(cfg)

	ctx := context.Background()
	_, err := m.Chat(ctx, "Hello", nil)

	if err == nil {
		t.Error("Expected error for 500 response")
	}
}

func TestBuildMessages(t *testing.T) {
	cfg := Config{
		Name:         "test",
		BaseURL:      "http://localhost",
		Model:        "test",
		SystemPrompt: "You are a test",
	}
	m := New(cfg)

	history := []Message{
		{Role: "user", Content: "Hello"},
	}

	messages := m.buildMessages("World", history)

	if len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages))
	}

	if messages[0].Role != "system" {
		t.Error("First message should be system")
	}

	if messages[1].Content != "Hello" {
		t.Error("Second message should be history")
	}

	if messages[2].Content != "World" {
		t.Error("Third message should be current")
	}
}

func TestModelChatStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" && r.Method == "POST" {
			// Verify streaming was requested
			var req chatRequest
			json.NewDecoder(r.Body).Decode(&req)
			if !req.Stream {
				t.Error("Expected stream=true")
			}

			// Send SSE response
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			chunks := []string{"Hello", " ", "World", "!"}
			for _, chunk := range chunks {
				data := map[string]interface{}{
					"choices": []map[string]interface{}{
						{"delta": map[string]string{"content": chunk}},
					},
				}
				jsonData, _ := json.Marshal(data)
				w.Write([]byte("data: " + string(jsonData) + "\n\n"))
				w.(http.Flusher).Flush()
			}
			w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := Config{
		Name:    "test",
		BaseURL: server.URL,
		Model:   "test-model",
	}
	m := New(cfg)

	ctx := context.Background()
	chunks, errs := m.ChatStream(ctx, "Hello", nil)

	var response string
	for chunk := range chunks {
		response += chunk
	}

	if err := <-errs; err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if response != "Hello World!" {
		t.Errorf("Expected 'Hello World!', got '%s'", response)
	}
}

func TestModelChatStreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Service unavailable"))
	}))
	defer server.Close()

	cfg := Config{
		Name:    "test",
		BaseURL: server.URL,
		Model:   "test",
	}
	m := New(cfg)

	ctx := context.Background()
	chunks, errs := m.ChatStream(ctx, "Hello", nil)

	// Drain chunks (should be empty)
	for range chunks {
	}

	err := <-errs
	if err == nil {
		t.Error("Expected error for 503 response")
	}
}

func TestExtractCodeFromToolCalls(t *testing.T) {
	tests := []struct {
		name      string
		toolCalls []ToolCall
		expected  string
	}{
		{
			name: "write_file with content",
			toolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: ToolFunction{
						Name:      "write_file",
						Arguments: `{"path": "test.py", "content": "def hello():\n    print('world')"}`,
					},
				},
			},
			expected: "def hello():\n    print('world')",
		},
		{
			name: "non-write_file tool call",
			toolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: ToolFunction{
						Name:      "read_file",
						Arguments: `{"path": "test.py"}`,
					},
				},
			},
			expected: "",
		},
		{
			name:      "empty tool calls",
			toolCalls: []ToolCall{},
			expected:  "",
		},
		{
			name: "multiple tool calls with write_file second",
			toolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: ToolFunction{
						Name:      "read_file",
						Arguments: `{"path": "input.txt"}`,
					},
				},
				{
					ID:   "call_2",
					Type: "function",
					Function: ToolFunction{
						Name:      "write_file",
						Arguments: `{"path": "output.py", "content": "x = 42"}`,
					},
				},
			},
			expected: "x = 42",
		},
		{
			name: "context_write fallback when no write_file",
			toolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: ToolFunction{
						Name:      "context_write",
						Arguments: `{"content": "Analyzing the bug: the issue is that the loop doesn't handle empty input correctly.", "tags": "debugging"}`,
					},
				},
			},
			expected: "Analyzing the bug: the issue is that the loop doesn't handle empty input correctly.",
		},
		{
			name: "write_file takes priority over context_write",
			toolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: ToolFunction{
						Name:      "context_write",
						Arguments: `{"content": "Starting task...", "tags": "progress"}`,
					},
				},
				{
					ID:   "call_2",
					Type: "function",
					Function: ToolFunction{
						Name:      "write_file",
						Arguments: `{"path": "result.py", "content": "def solve(): return 42"}`,
					},
				},
			},
			expected: "def solve(): return 42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCodeFromToolCalls(tt.toolCalls)
			if result != tt.expected {
				t.Errorf("extractCodeFromToolCalls() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestModelChatExtractsFromToolCalls(t *testing.T) {
	// Simulates Devstral returning code via write_file tool call with empty content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "",
					"tool_calls": [{
						"id": "call_123",
						"type": "function",
						"function": {
							"name": "write_file",
							"arguments": "{\"path\": \"fibonacci.py\", \"content\": \"def fib(n):\\n    if n <= 1:\\n        return n\\n    return fib(n-1) + fib(n-2)\"}"
						}
					}]
				},
				"finish_reason": "tool_calls"
			}]
		}`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(resp))
	}))
	defer server.Close()

	cfg := Config{
		Name:    "test",
		BaseURL: server.URL,
		Model:   "devstral",
	}
	m := New(cfg)

	ctx := context.Background()
	response, err := m.Chat(ctx, "Write fibonacci", nil)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := "def fib(n):\n    if n <= 1:\n        return n\n    return fib(n-1) + fib(n-2)"
	if response != expected {
		t.Errorf("Chat() = %q, want %q", response, expected)
	}
}
