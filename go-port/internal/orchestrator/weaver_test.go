package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/r3d91ll/weaver-code/internal/senior"
)

// mockSeniorAdapter implements senior.Adapter for testing
type mockSeniorAdapter struct {
	responses    []string
	responseIdx  int
	lastMessage  string
	lastHistory  []senior.Message
	available    bool
	contextLimit int
}

func newMockSenior(responses ...string) *mockSeniorAdapter {
	return &mockSeniorAdapter{
		responses:    responses,
		available:    true,
		contextLimit: 100000,
	}
}

func (m *mockSeniorAdapter) Provider() senior.Provider {
	return senior.ProviderClaudeCode
}

func (m *mockSeniorAdapter) Name() string {
	return "Mock Senior"
}

func (m *mockSeniorAdapter) IsAvailable() bool {
	return m.available
}

func (m *mockSeniorAdapter) ContextLimit() int {
	return m.contextLimit
}

func (m *mockSeniorAdapter) Chat(ctx context.Context, message string, history []senior.Message) (string, error) {
	m.lastMessage = message
	m.lastHistory = history

	if m.responseIdx >= len(m.responses) {
		return "default response", nil
	}

	resp := m.responses[m.responseIdx]
	m.responseIdx++
	return resp, nil
}

func (m *mockSeniorAdapter) ChatStream(ctx context.Context, message string, history []senior.Message) (<-chan string, <-chan error) {
	chunks := make(chan string, 1)
	errs := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errs)

		resp, _ := m.Chat(ctx, message, history)
		chunks <- resp
	}()

	return chunks, errs
}

// createTestWeaver creates a Weaver with mock senior and real junior (mock HTTP server)
func createTestWeaver(t *testing.T, mockSenior *mockSeniorAdapter, juniorServer *httptest.Server) *Weaver {
	t.Helper()

	// Create temp directory for memory
	tmpDir, err := os.MkdirTemp("", "weaver-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	// Override memory path
	os.Setenv("HOME", tmpDir)
	defer os.Unsetenv("HOME")

	cfg := Config{
		SeniorProvider:     senior.ProviderClaudeCode,
		SeniorPrompt:       "Test senior prompt",
		JuniorURL:          juniorServer.URL,
		JuniorModel:        "test-model",
		JuniorContextLimit: 8000,
		JuniorPrompt:       "Test junior prompt",
	}

	w, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create weaver: %v", err)
	}

	// Replace senior with mock
	w.senior = mockSenior

	return w
}

// createMockJuniorServer creates a mock OpenAI-compatible server
func createMockJuniorServer(t *testing.T, responses map[string]string) *httptest.Server {
	t.Helper()

	callCount := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			json.NewEncoder(w).Encode(map[string]interface{}{"models": []string{"test-model"}})

		case "/chat/completions":
			callCount++

			var req struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			json.NewDecoder(r.Body).Decode(&req)

			// Get last user message
			lastMsg := ""
			for _, m := range req.Messages {
				lastMsg = m.Content
			}

			// Find matching response
			response := "Junior default response"
			for pattern, resp := range responses {
				if strings.Contains(lastMsg, pattern) {
					response = resp
					break
				}
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"role": "assistant", "content": response}},
				},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestWeaverChat_DirectResponse(t *testing.T) {
	// Senior responds directly without delegation
	mockSenior := newMockSenior("Here's your answer: 42")

	juniorServer := createMockJuniorServer(t, nil)
	defer juniorServer.Close()

	w := createTestWeaver(t, mockSenior, juniorServer)

	ctx := context.Background()
	response, err := w.Chat(ctx, "What is the meaning of life?")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if response != "Here's your answer: 42" {
		t.Errorf("Unexpected response: %s", response)
	}

	if w.CurrentAgent() != "senior" {
		t.Errorf("Expected current agent 'senior', got '%s'", w.CurrentAgent())
	}
}

func TestWeaverChat_DelegationToJunior(t *testing.T) {
	// Senior delegates with /local, then reviews
	mockSenior := newMockSenior(
		"I'll delegate this. /local Write a hello world function",
		"Junior did a good job. Here's the reviewed code: func hello() {}",
	)

	juniorServer := createMockJuniorServer(t, map[string]string{
		"hello world": "func hello() { fmt.Println(\"Hello\") }",
	})
	defer juniorServer.Close()

	w := createTestWeaver(t, mockSenior, juniorServer)

	ctx := context.Background()
	response, err := w.Chat(ctx, "Write a hello world function")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should get senior's review response
	if !strings.Contains(response, "reviewed") {
		t.Errorf("Expected review response, got: %s", response)
	}
}

func TestWeaverChat_JuniorUnavailable(t *testing.T) {
	// Senior delegates but junior is down
	mockSenior := newMockSenior(
		"I'll delegate this. /local Do something",
	)

	// Create server that returns 500
	juniorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer juniorServer.Close()

	w := createTestWeaver(t, mockSenior, juniorServer)

	ctx := context.Background()
	response, err := w.Chat(ctx, "Do something")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should include junior unavailable message
	if !strings.Contains(response, "Junior unavailable") {
		t.Errorf("Expected 'Junior unavailable' message, got: %s", response)
	}
}

func TestWeaverListAgents(t *testing.T) {
	mockSenior := newMockSenior()

	juniorServer := createMockJuniorServer(t, nil)
	defer juniorServer.Close()

	w := createTestWeaver(t, mockSenior, juniorServer)

	agents := w.ListAgents()

	if len(agents) != 2 {
		t.Errorf("Expected 2 agents, got %d", len(agents))
	}

	// Check senior
	found := false
	for _, a := range agents {
		if a.Role == "senior" {
			found = true
			if !a.Available {
				t.Error("Senior should be available")
			}
		}
	}
	if !found {
		t.Error("Senior agent not found")
	}
}

func TestWeaverClearContext(t *testing.T) {
	mockSenior := newMockSenior("Response 1", "Response 2")

	juniorServer := createMockJuniorServer(t, nil)
	defer juniorServer.Close()

	w := createTestWeaver(t, mockSenior, juniorServer)

	ctx := context.Background()

	// Send some messages
	w.Chat(ctx, "Hello")
	w.Chat(ctx, "World")

	// Clear
	w.ClearContext()

	// Verify context is cleared
	if len(w.seniorCtx.Messages()) != 0 {
		t.Errorf("Expected 0 senior messages after clear, got %d", len(w.seniorCtx.Messages()))
	}

	if len(w.juniorCtx.Messages()) != 0 {
		t.Errorf("Expected 0 junior messages after clear, got %d", len(w.juniorCtx.Messages()))
	}
}

func TestWeaverMemory(t *testing.T) {
	mockSenior := newMockSenior()

	juniorServer := createMockJuniorServer(t, nil)
	defer juniorServer.Close()

	// Create temp home for memory file
	tmpDir, err := os.MkdirTemp("", "weaver-mem-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	weaverDir := filepath.Join(tmpDir, ".weaver")
	os.MkdirAll(weaverDir, 0755)

	cfg := Config{
		JuniorURL:   juniorServer.URL,
		JuniorModel: "test",
	}

	// Temporarily change HOME
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	w, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create weaver: %v", err)
	}
	w.senior = mockSenior

	// Test memory operations
	mem := w.Memory()

	id := mem.Write("test", "Test note", nil)
	if id == "" {
		t.Error("Expected non-empty note ID")
	}

	notes := mem.List(10, "", nil)
	if len(notes) != 1 {
		t.Errorf("Expected 1 note, got %d", len(notes))
	}
}

func TestExtractLocalTask(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"No delegation here", ""},
		{"/local Write a function", "Write a function"},
		{"/LOCAL Write a function", "Write a function"},
		// Now captures everything after /local to end of message
		{"Some text\n/local Do this task\nMore text", "Do this task\nMore text"},
		{"Talk about /local in the middle", "in the middle"},
		// Multi-line task (the bug case)
		{"/local Task:\n\n**Step 1:**\nDo something\n\n**Step 2:**\nDo more", "Task:\n\n**Step 1:**\nDo something\n\n**Step 2:**\nDo more"},
		// Preamble then delegation
		{"I'll delegate this.\n\n/local Write code:\n```python\ndef foo():\n    pass\n```", "Write code:\n```python\ndef foo():\n    pass\n```"},
	}

	for _, tt := range tests {
		got := extractLocalTask(tt.input)
		if got != tt.expected {
			t.Errorf("extractLocalTask(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestWeaverChatStream(t *testing.T) {
	mockSenior := newMockSenior("Streaming response here")

	juniorServer := createMockJuniorServer(t, nil)
	defer juniorServer.Close()

	w := createTestWeaver(t, mockSenior, juniorServer)

	ctx := context.Background()
	chunks, errs := w.ChatStream(ctx, "Hello")

	var response strings.Builder
	for chunk := range chunks {
		response.WriteString(chunk)
	}

	if err := <-errs; err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if response.String() != "Streaming response here" {
		t.Errorf("Unexpected response: %s", response.String())
	}
}

func TestWeaverDelegationFlow_FullRoundTrip(t *testing.T) {
	// This tests the complete delegation flow:
	// 1. User asks senior to do something
	// 2. Senior delegates to junior with /local
	// 3. Junior completes the task
	// 4. Senior reviews junior's work
	// 5. Senior provides final response

	mockSenior := newMockSenior(
		"I'll have my junior assistant help with this. /local Write a Python function to calculate fibonacci",
		"Junior's fibonacci implementation looks correct. The function uses memoization for efficiency.",
	)

	juniorResponse := `def fibonacci(n, memo={}):
    if n in memo:
        return memo[n]
    if n <= 1:
        return n
    memo[n] = fibonacci(n-1, memo) + fibonacci(n-2, memo)
    return memo[n]`

	juniorServer := createMockJuniorServer(t, map[string]string{
		"fibonacci": juniorResponse,
	})
	defer juniorServer.Close()

	w := createTestWeaver(t, mockSenior, juniorServer)

	ctx := context.Background()
	response, err := w.Chat(ctx, "Can you write me a fibonacci function?")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify the review was called - check that senior saw junior's response
	// The second call to senior should have the junior's fibonacci code in the message
	if !strings.Contains(mockSenior.lastMessage, "fibonacci") {
		t.Errorf("Senior should have reviewed junior's fibonacci code, got: %s", mockSenior.lastMessage)
	}

	// Verify final response mentions the review
	if !strings.Contains(response, "fibonacci") {
		t.Errorf("Expected review response mentioning fibonacci, got: %s", response)
	}
}

func TestWeaverConversationHistory(t *testing.T) {
	// Test that conversation history is properly tracked
	// Note: The current user message is added to context BEFORE Chat is called,
	// so history includes the current message being processed plus all prior messages
	mockSenior := newMockSenior("Response 1", "Response 2", "Response 3")

	juniorServer := createMockJuniorServer(t, nil)
	defer juniorServer.Close()

	w := createTestWeaver(t, mockSenior, juniorServer)

	ctx := context.Background()

	// First message - history includes only the current message
	w.Chat(ctx, "Hello")
	if len(mockSenior.lastHistory) != 1 { // current "Hello" message
		t.Errorf("Expected 1 history message on first call, got %d", len(mockSenior.lastHistory))
	}

	// Second message - history includes: Hello, Response 1, How are you?
	w.Chat(ctx, "How are you?")
	if len(mockSenior.lastHistory) != 3 { // 2 from first exchange + current
		t.Errorf("Expected 3 history messages, got %d", len(mockSenior.lastHistory))
	}

	// Third message - history includes: Hello, R1, How are you?, R2, Tell me more
	w.Chat(ctx, "Tell me more")
	if len(mockSenior.lastHistory) != 5 { // 4 from prior exchanges + current
		t.Errorf("Expected 5 history messages, got %d", len(mockSenior.lastHistory))
	}
}

func TestWeaverMultipleDelegations(t *testing.T) {
	// Test multiple delegations in a conversation
	mockSenior := newMockSenior(
		"/local Task 1",
		"Reviewed task 1",
		"/local Task 2",
		"Reviewed task 2",
	)

	taskCount := 0
	juniorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			json.NewEncoder(w).Encode(map[string]interface{}{"models": []string{"test"}})
		case "/chat/completions":
			taskCount++
			json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"role": "assistant", "content": "Completed task " + string(rune('0'+taskCount))}},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer juniorServer.Close()

	w := createTestWeaver(t, mockSenior, juniorServer)

	ctx := context.Background()

	// First delegation
	resp1, _ := w.Chat(ctx, "Do task 1")
	if !strings.Contains(resp1, "Reviewed task 1") {
		t.Errorf("Expected 'Reviewed task 1', got: %s", resp1)
	}

	// Second delegation
	resp2, _ := w.Chat(ctx, "Do task 2")
	if !strings.Contains(resp2, "Reviewed task 2") {
		t.Errorf("Expected 'Reviewed task 2', got: %s", resp2)
	}

	// Verify junior was called twice
	if taskCount != 2 {
		t.Errorf("Expected junior to be called 2 times, got %d", taskCount)
	}
}

func TestWeaverRecursiveDelegation(t *testing.T) {
	// Test that Senior's review containing /local triggers another delegation
	// This simulates Senior sending multiple tasks in sequence:
	// Senior: "Review of task 1... /local Task 2"
	// Junior: "Task 2 response"
	// Senior: "Review of task 2... /local Task 3"
	// Junior: "Task 3 response"
	// Senior: "Final review" (no /local, stops)

	mockSenior := newMockSenior(
		"I'll run multiple tasks.\n\n/local Task 1: Write hello world",
		"Task 1 looks good! Now:\n\n/local Task 2: Write goodbye world",
		"Task 2 looks good! Now:\n\n/local Task 3: Write final message",
		"All 3 tasks complete. Great work Junior!",
	)

	taskCount := 0
	juniorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			json.NewEncoder(w).Encode(map[string]interface{}{"models": []string{"test"}})
		case "/chat/completions":
			taskCount++
			response := fmt.Sprintf("Completed task %d", taskCount)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"role": "assistant", "content": response}},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer juniorServer.Close()

	w := createTestWeaver(t, mockSenior, juniorServer)

	ctx := context.Background()
	resp, err := w.Chat(ctx, "Run 3 tasks please")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify junior was called 3 times (one for each /local in chain)
	if taskCount != 3 {
		t.Errorf("Expected junior to be called 3 times, got %d", taskCount)
	}

	// Verify final response contains the completion message
	if !strings.Contains(resp, "All 3 tasks complete") {
		t.Errorf("Expected final review message, got: %s", resp)
	}
}

func TestWeaverRecursiveDelegationStream(t *testing.T) {
	// Same test but for streaming
	mockSenior := newMockSenior(
		"First task:\n\n/local Write code A",
		"Code A reviewed. Next:\n\n/local Write code B",
		"Code B reviewed. All done!",
	)

	taskCount := 0
	juniorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			json.NewEncoder(w).Encode(map[string]interface{}{"models": []string{"test"}})
		case "/chat/completions":
			taskCount++
			response := fmt.Sprintf("Code %c implementation", 'A'+taskCount-1)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"role": "assistant", "content": response}},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer juniorServer.Close()

	w := createTestWeaver(t, mockSenior, juniorServer)

	ctx := context.Background()
	chunks, errs := w.ChatStreamStructured(ctx, "Write two pieces of code")

	// Collect all chunks
	var seniorChunks, juniorChunks []string
	for chunk := range chunks {
		if chunk.Content != "" {
			if chunk.Agent == "senior" {
				seniorChunks = append(seniorChunks, chunk.Content)
			} else if chunk.Agent == "junior" {
				juniorChunks = append(juniorChunks, chunk.Content)
			}
		}
	}

	if err := <-errs; err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should have 2 junior responses
	if len(juniorChunks) != 2 {
		t.Errorf("Expected 2 junior chunks, got %d: %v", len(juniorChunks), juniorChunks)
	}

	// Should have 3 senior responses (initial + 2 reviews)
	if len(seniorChunks) != 3 {
		t.Errorf("Expected 3 senior chunks, got %d: %v", len(seniorChunks), seniorChunks)
	}

	// Verify junior was called twice
	if taskCount != 2 {
		t.Errorf("Expected junior to be called 2 times, got %d", taskCount)
	}
}
