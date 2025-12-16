package junior

import (
	"testing"
)

func TestIsMistralToolCallFormat(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "valid mistral format",
			content:  `[TOOL_CALLS]write_file[ARGS]{"path": "test.py"}`,
			expected: true,
		},
		{
			name:     "regular content",
			content:  "def hello(): print('world')",
			expected: false,
		},
		{
			name:     "empty content",
			content:  "",
			expected: false,
		},
		{
			name:     "partial marker",
			content:  "[TOOL_CALLS]",
			expected: true, // Contains marker, even if incomplete
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsMistralToolCallFormat(tt.content)
			if result != tt.expected {
				t.Errorf("IsMistralToolCallFormat(%q) = %v, want %v", tt.content, result, tt.expected)
			}
		})
	}
}

func TestParseMistralToolCalls(t *testing.T) {
	tests := []struct {
		name              string
		content           string
		expectedToolCalls int
		expectedFuncName  string
		expectedRemaining string
	}{
		{
			name:              "single tool call",
			content:           `[TOOL_CALLS]write_file[ARGS]{"path": "test.py", "content": "print('hello')"}`,
			expectedToolCalls: 1,
			expectedFuncName:  "write_file",
			expectedRemaining: "",
		},
		{
			name:              "tool call with trailing content",
			content:           `[TOOL_CALLS]read_file[ARGS]{"path": "test.py"} some remaining text`,
			expectedToolCalls: 1,
			expectedFuncName:  "read_file",
			expectedRemaining: "some remaining text",
		},
		{
			name:              "no tool call",
			content:           "just regular content",
			expectedToolCalls: 0,
			expectedFuncName:  "",
			expectedRemaining: "just regular content",
		},
		{
			name: "multiline json",
			content: `[TOOL_CALLS]write_file[ARGS]{
				"path": "test.py",
				"content": "def foo():\n    return 42"
			}`,
			expectedToolCalls: 1,
			expectedFuncName:  "write_file",
			expectedRemaining: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolCalls, remaining := ParseMistralToolCalls(tt.content)

			if len(toolCalls) != tt.expectedToolCalls {
				t.Errorf("ParseMistralToolCalls() got %d tool calls, want %d", len(toolCalls), tt.expectedToolCalls)
			}

			if tt.expectedToolCalls > 0 && toolCalls[0].Function.Name != tt.expectedFuncName {
				t.Errorf("ParseMistralToolCalls() got func name %q, want %q", toolCalls[0].Function.Name, tt.expectedFuncName)
			}

			if remaining != tt.expectedRemaining {
				t.Errorf("ParseMistralToolCalls() remaining = %q, want %q", remaining, tt.expectedRemaining)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedJSON string
		expectedRest string
		expectedOK   bool
	}{
		{
			name:         "simple json",
			input:        `{"key": "value"}`,
			expectedJSON: `{"key": "value"}`,
			expectedRest: "",
			expectedOK:   true,
		},
		{
			name:         "json with trailing content",
			input:        `{"key": "value"} more stuff`,
			expectedJSON: `{"key": "value"}`,
			expectedRest: " more stuff",
			expectedOK:   true,
		},
		{
			name:         "nested json",
			input:        `{"outer": {"inner": "value"}}`,
			expectedJSON: `{"outer": {"inner": "value"}}`,
			expectedRest: "",
			expectedOK:   true,
		},
		{
			name:         "json with escaped quotes",
			input:        `{"code": "print(\"hello\")"}`,
			expectedJSON: `{"code": "print(\"hello\")"}`,
			expectedRest: "",
			expectedOK:   true,
		},
		{
			name:         "not json",
			input:        `not json at all`,
			expectedJSON: "",
			expectedRest: "not json at all",
			expectedOK:   false,
		},
		{
			name:         "incomplete json",
			input:        `{"key": "value`,
			expectedJSON: `{"key": "value`,
			expectedRest: "",
			expectedOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			json, rest, ok := extractJSON(tt.input)
			if json != tt.expectedJSON {
				t.Errorf("extractJSON() json = %q, want %q", json, tt.expectedJSON)
			}
			if rest != tt.expectedRest {
				t.Errorf("extractJSON() rest = %q, want %q", rest, tt.expectedRest)
			}
			if ok != tt.expectedOK {
				t.Errorf("extractJSON() ok = %v, want %v", ok, tt.expectedOK)
			}
		})
	}
}

func TestExtractCodeFromMistralResponse(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "write_file with code",
			content:  `[TOOL_CALLS]write_file[ARGS]{"path": "test.py", "content": "def hello():\n    print('world')"}`,
			expected: "def hello():\n    print('world')",
		},
		{
			name:     "regular content passthrough",
			content:  "def hello(): print('world')",
			expected: "def hello(): print('world')",
		},
		{
			name:     "escaped newlines",
			content:  `[TOOL_CALLS]write_file[ARGS]{"path": "test.py", "content": "line1\nline2\nline3"}`,
			expected: "line1\nline2\nline3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractCodeFromMistralResponse(tt.content)
			if result != tt.expected {
				t.Errorf("ExtractCodeFromMistralResponse() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestConvertMistralResponse(t *testing.T) {
	content := `[TOOL_CALLS]read_file[ARGS]{"path": "main.go"}`

	remaining, toolCalls := ConvertMistralResponse(content, nil)

	if len(toolCalls) != 1 {
		t.Fatalf("ConvertMistralResponse() got %d tool calls, want 1", len(toolCalls))
	}

	if toolCalls[0].Function.Name != "read_file" {
		t.Errorf("ConvertMistralResponse() func name = %q, want %q", toolCalls[0].Function.Name, "read_file")
	}

	if toolCalls[0].ID != "mistral-0" {
		t.Errorf("ConvertMistralResponse() tool ID = %q, want %q", toolCalls[0].ID, "mistral-0")
	}

	if remaining != "" {
		t.Errorf("ConvertMistralResponse() remaining = %q, want empty", remaining)
	}
}

// TestTruncatedMistralOutput tests handling of truncated output (max_tokens hit)
func TestTruncatedMistralOutput(t *testing.T) {
	// Simulates truncated output from LM Studio logs where max_tokens was reached
	truncated := `[TOOL_CALLS]write_file[ARGS]{"path": "refactored_function.py", "content": "def filter_active_high_scorers(data):\n    result = []\n    for item in data:\n        if item.get(\"status\") == \"active\":\n            result.append({\"id\": item[\"id\"`

	// Should still extract partial code
	code := ExtractCodeFromMistralResponse(truncated)

	if !contains(code, "def filter_active_high_scorers") {
		t.Errorf("Should extract partial function definition, got: %q", code)
	}

	if !contains(code, "result = []") {
		t.Errorf("Should contain code body, got: %q", code)
	}
}

// TestRealWorldMistralOutput tests with actual Devstral output from LM Studio logs
func TestRealWorldMistralOutput(t *testing.T) {
	// This is the actual format seen in LM Studio logs
	content := `[TOOL_CALLS]write_file[ARGS]{"content": "def filter_active_high_scoring(items: list[dict]) -> list[dict]:\n    result = []\n    for item in items:\n        if item.get(\"status\") == \"active\" and item.get(\"score\", 0) > 50:\n            result.append({\"id\": item[\"id\"], \"n\": item[\"name\"], \"s\": item[\"score\"]})\n    return result"}`

	toolCalls, remaining := ParseMistralToolCalls(content)

	if len(toolCalls) != 1 {
		t.Fatalf("ParseMistralToolCalls() got %d tool calls, want 1", len(toolCalls))
	}

	if toolCalls[0].Function.Name != "write_file" {
		t.Errorf("Function name = %q, want %q", toolCalls[0].Function.Name, "write_file")
	}

	// Extract code should get the actual code
	code := ExtractCodeFromMistralResponse(content)
	if !contains(code, "def filter_active_high_scoring") {
		t.Errorf("ExtractCodeFromMistralResponse() should contain function definition, got: %q", code)
	}

	_ = remaining // unused in this test
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
