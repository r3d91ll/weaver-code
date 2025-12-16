package assessment

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// mockJuniorClient is a mock implementation of JuniorClient for testing.
type mockJuniorClient struct {
	name      string
	available bool
	responses map[string]string // prompt -> response mapping
}

func (m *mockJuniorClient) Chat(ctx context.Context, message string) (string, error) {
	if !m.available {
		return "", fmt.Errorf("junior not available")
	}
	// Return a canned response based on challenge type
	if strings.Contains(message, "fizzbuzz") || strings.Contains(message, "FizzBuzz") {
		return `def fizzbuzz(n):
    result = []
    for i in range(1, n + 1):
        if i % 15 == 0:
            result.append("FizzBuzz")
        elif i % 3 == 0:
            result.append("Fizz")
        elif i % 5 == 0:
            result.append("Buzz")
        else:
            result.append(str(i))
    return result`, nil
	}
	return "def solution(): pass", nil
}

func (m *mockJuniorClient) Name() string {
	return m.name
}

func (m *mockJuniorClient) IsAvailable() bool {
	return m.available
}

// mockSeniorClient is a mock implementation of SeniorClient for testing.
type mockSeniorClient struct {
	defaultScore int
}

func (m *mockSeniorClient) Chat(ctx context.Context, message string) (string, error) {
	// Return a structured evaluation response
	return fmt.Sprintf("SCORE: %d\nEVALUATION: Test evaluation - the code looks reasonable.", m.defaultScore), nil
}

func TestChallengeSet(t *testing.T) {
	challenges := ChallengeSet()

	// Should have 20 challenges (15 original + 5 tool_use)
	if len(challenges) != 20 {
		t.Errorf("expected 20 challenges, got %d", len(challenges))
	}

	// Check categories are balanced
	categories := make(map[string]int)
	for _, c := range challenges {
		categories[c.Category]++
	}

	// Original categories have 3 each, tool_use has 5
	expectedCounts := map[string]int{
		"algorithms":      3,
		"data_structures": 3,
		"code_quality":    3,
		"real_world":      3,
		"tool_use":        5,
		"problem_solving": 3,
	}
	for cat, expectedCount := range expectedCounts {
		if categories[cat] != expectedCount {
			t.Errorf("expected %d challenges in category %s, got %d", expectedCount, cat, categories[cat])
		}
	}

	// Check all challenges have required fields
	for _, c := range challenges {
		if c.ID == "" {
			t.Error("challenge missing ID")
		}
		if c.Name == "" {
			t.Error("challenge missing Name")
		}
		if c.Prompt == "" {
			t.Error("challenge missing Prompt")
		}
		if c.MaxScore == 0 {
			t.Errorf("challenge %s has zero MaxScore", c.ID)
		}
	}
}

func TestChallengesByCategory(t *testing.T) {
	byCategory := ChallengesByCategory()

	expectedCategories := CategoryOrder()
	for _, cat := range expectedCategories {
		if _, ok := byCategory[cat]; !ok {
			t.Errorf("missing category: %s", cat)
		}
	}
}

func TestCategoryNames(t *testing.T) {
	names := CategoryNames()

	expectedCategories := CategoryOrder()
	for _, cat := range expectedCategories {
		if name, ok := names[cat]; !ok || name == "" {
			t.Errorf("missing or empty name for category: %s", cat)
		}
	}
}

func TestAssessor_JuniorUnavailable(t *testing.T) {
	junior := &mockJuniorClient{name: "test-model", available: false}
	senior := &mockSeniorClient{defaultScore: 3}

	assessor := NewAssessor(junior, senior)
	_, err := assessor.Run(context.Background())

	if err == nil {
		t.Error("expected error when Junior is unavailable")
	}
}

func TestAssessor_SingleChallenge(t *testing.T) {
	junior := &mockJuniorClient{name: "test-model", available: true}
	senior := &mockSeniorClient{defaultScore: 3}

	assessor := NewAssessor(junior, senior)

	// Test just the runChallenge method
	challenge := Challenge{
		ID:       "test",
		Category: "algorithms",
		Name:     "FizzBuzz Test",
		Prompt:   "Write fizzbuzz",
		MaxScore: 4,
	}

	result, err := assessor.runChallenge(context.Background(), challenge)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Score != 3 {
		t.Errorf("expected score 3, got %d", result.Score)
	}

	if result.JuniorCode == "" {
		t.Error("expected non-empty JuniorCode")
	}
}

func TestParseEvaluation(t *testing.T) {
	tests := []struct {
		name           string
		response       string
		expectedScore  int
		expectEvalText bool
	}{
		{
			name:           "structured response",
			response:       "SCORE: 3\nEVALUATION: Good implementation with minor issues.",
			expectedScore:  3,
			expectEvalText: true,
		},
		{
			name:           "perfect score",
			response:       "SCORE: 4\nEVALUATION: Excellent work!",
			expectedScore:  4,
			expectEvalText: true,
		},
		{
			name:           "zero score",
			response:       "SCORE: 0\nEVALUATION: The code doesn't work.",
			expectedScore:  0,
			expectEvalText: true,
		},
		{
			name:           "natural language excellent",
			response:       "This is an excellent solution that exceeds requirements.",
			expectedScore:  4,
			expectEvalText: false,
		},
		{
			name:           "natural language correct",
			response:       "The code is correct and works well.",
			expectedScore:  3,
			expectEvalText: false,
		},
		{
			name:           "natural language poor",
			response:       "This is poor quality code that needs work.",
			expectedScore:  1,
			expectEvalText: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, eval := parseEvaluation(tt.response)

			if score != tt.expectedScore {
				t.Errorf("expected score %d, got %d", tt.expectedScore, score)
			}

			if eval == "" {
				t.Error("expected non-empty evaluation")
			}
		})
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"Hello World", "world", true},
		{"Hello World", "HELLO", true},
		{"Hello World", "foo", false},
		{"", "test", false},
		{"test", "", true},
	}

	for _, tt := range tests {
		result := containsIgnoreCase(tt.s, tt.substr)
		if result != tt.expected {
			t.Errorf("containsIgnoreCase(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
		}
	}
}

func TestToLower(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello world"},
		{"UPPERCASE", "uppercase"},
		{"already lower", "already lower"},
		{"MiXeD CaSe", "mixed case"},
	}

	for _, tt := range tests {
		result := toLower(tt.input)
		if result != tt.expected {
			t.Errorf("toLower(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestAnalyzeResults(t *testing.T) {
	assessor := &Assessor{}
	report := &AssessmentReport{
		TotalScore: 45,
		MaxScore:   60,
		Percentage: 75,
		CategoryScores: map[string]CategoryScore{
			"algorithms":      {Name: "Basic Algorithms", Score: 10, MaxScore: 12, Percentage: 83},
			"data_structures": {Name: "Data Structures", Score: 8, MaxScore: 12, Percentage: 67},
			"code_quality":    {Name: "Code Quality", Score: 11, MaxScore: 12, Percentage: 92},
			"real_world":      {Name: "Real-World Tasks", Score: 8, MaxScore: 12, Percentage: 67},
			"problem_solving": {Name: "Problem Solving", Score: 8, MaxScore: 12, Percentage: 67},
		},
	}

	assessor.analyzeResults(report)

	// Should have some strengths (algorithms, code_quality)
	if len(report.Strengths) == 0 {
		t.Error("expected some strengths to be identified")
	}

	// Should have some recommendations
	if len(report.RecommendTasks) == 0 {
		t.Error("expected some task recommendations")
	}

	// Should have some avoid tasks
	if len(report.AvoidTasks) == 0 {
		t.Error("expected some tasks to avoid")
	}
}
