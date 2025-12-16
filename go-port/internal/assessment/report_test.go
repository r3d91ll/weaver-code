package assessment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateMarkdown(t *testing.T) {
	report := &AssessmentReport{
		Timestamp:  time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		ModelName:  "test-model",
		TotalScore: 45,
		MaxScore:   60,
		Percentage: 75,
		CategoryScores: map[string]CategoryScore{
			"algorithms":      {Name: "Basic Algorithms", Score: 10, MaxScore: 12, Percentage: 83},
			"data_structures": {Name: "Data Structures", Score: 9, MaxScore: 12, Percentage: 75},
			"code_quality":    {Name: "Code Quality", Score: 10, MaxScore: 12, Percentage: 83},
			"real_world":      {Name: "Real-World Tasks", Score: 8, MaxScore: 12, Percentage: 67},
			"problem_solving": {Name: "Problem Solving", Score: 8, MaxScore: 12, Percentage: 67},
		},
		Strengths:      []string{"Good algorithm skills"},
		Weaknesses:     []string{"Needs improvement in real-world tasks"},
		RecommendTasks: []string{"Boilerplate code", "Unit tests"},
		AvoidTasks:     []string{"Security-critical code"},
		ChallengeResults: []ChallengeResult{
			{
				ChallengeID: "fizzbuzz",
				Category:    "algorithms",
				Name:        "FizzBuzz",
				Score:       4,
				MaxScore:    4,
				Evaluation:  "Excellent implementation",
				Duration:    2 * time.Second,
			},
		},
	}

	markdown := GenerateMarkdown(report)

	// Check key elements are present
	checks := []string{
		"## Junior Engineer Assessment",
		"test-model",
		"45/60",
		"75%",
		"Basic Algorithms",
		"### Strengths",
		"### Weaknesses",
		"### Delegation Guidelines",
		"**Recommended Tasks:**",
		"**Avoid Delegating:**",
		"### Challenge Details",
	}

	for _, check := range checks {
		if !strings.Contains(markdown, check) {
			t.Errorf("expected markdown to contain %q", check)
		}
	}
}

func TestWriteReport_NewFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "assessment-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	report := &AssessmentReport{
		Timestamp:  time.Now(),
		ModelName:  "test-model",
		TotalScore: 30,
		MaxScore:   60,
		Percentage: 50,
		CategoryScores: map[string]CategoryScore{
			"algorithms": {Name: "Basic Algorithms", Score: 6, MaxScore: 12, Percentage: 50},
		},
	}

	err = WriteReport(report, tmpDir)
	if err != nil {
		t.Fatalf("WriteReport failed: %v", err)
	}

	// Check file was created
	content, err := os.ReadFile(filepath.Join(tmpDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}

	if !strings.Contains(string(content), "## Junior Engineer Assessment") {
		t.Error("file doesn't contain assessment section")
	}

	if !strings.Contains(string(content), "test-model") {
		t.Error("file doesn't contain model name")
	}
}

func TestWriteReport_UpdateExisting(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "assessment-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing CLAUDE.md with some content
	existingContent := `# CLAUDE.md

## Project Overview

This is an existing project.

## Junior Engineer Assessment

**Last Assessment:** 2025-01-01 00:00:00
**Model:** old-model
**Score:** 20/60 (33%)

## Other Section

Some other content.
`
	err = os.WriteFile(filepath.Join(tmpDir, "CLAUDE.md"), []byte(existingContent), 0644)
	if err != nil {
		t.Fatalf("failed to write existing file: %v", err)
	}

	// Write new assessment
	report := &AssessmentReport{
		Timestamp:  time.Now(),
		ModelName:  "new-model",
		TotalScore: 45,
		MaxScore:   60,
		Percentage: 75,
		CategoryScores: map[string]CategoryScore{
			"algorithms": {Name: "Basic Algorithms", Score: 9, MaxScore: 12, Percentage: 75},
		},
	}

	err = WriteReport(report, tmpDir)
	if err != nil {
		t.Fatalf("WriteReport failed: %v", err)
	}

	// Check file was updated
	content, err := os.ReadFile(filepath.Join(tmpDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}

	contentStr := string(content)

	// Should preserve project overview
	if !strings.Contains(contentStr, "## Project Overview") {
		t.Error("lost Project Overview section")
	}

	// Should have new model info
	if !strings.Contains(contentStr, "new-model") {
		t.Error("doesn't contain new model name")
	}

	// Should NOT have old model info
	if strings.Contains(contentStr, "old-model") {
		t.Error("still contains old model name")
	}

	// Should preserve Other Section
	if !strings.Contains(contentStr, "## Other Section") {
		t.Error("lost Other Section")
	}
}

func TestUpdateAssessmentSection(t *testing.T) {
	tests := []struct {
		name          string
		existing      string
		newAssessment string
		wantContains  []string
		wantNotContain []string
	}{
		{
			name:          "append to file without assessment",
			existing:      "# CLAUDE.md\n\nSome content.\n",
			newAssessment: "## Junior Engineer Assessment\n\nNew assessment content.\n",
			wantContains:  []string{"Some content", "Junior Engineer Assessment", "New assessment content"},
		},
		{
			name: "replace existing assessment",
			existing: `# CLAUDE.md

## Junior Engineer Assessment

Old assessment.

## Other Section

Other content.
`,
			newAssessment: "## Junior Engineer Assessment\n\nNew assessment.\n",
			wantContains:  []string{"New assessment", "## Other Section", "Other content"},
			wantNotContain: []string{"Old assessment"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := updateAssessmentSection(tt.existing, tt.newAssessment)

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("result should contain %q", want)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if strings.Contains(result, notWant) {
					t.Errorf("result should NOT contain %q", notWant)
				}
			}
		})
	}
}

func TestGenerateSummary(t *testing.T) {
	report := &AssessmentReport{
		ModelName:  "test-model",
		TotalScore: 45,
		MaxScore:   60,
		Percentage: 75,
		Strengths:  []string{"Good algorithms"},
		Weaknesses: []string{"Poor real-world"},
	}

	summary := GenerateSummary(report)

	checks := []string{
		"test-model",
		"45/60",
		"75%",
		"CLAUDE.md",
		"JUNIOR.md",
	}

	for _, check := range checks {
		if !strings.Contains(summary, check) {
			t.Errorf("summary should contain %q", check)
		}
	}
}

func TestGenerateJuniorMarkdown(t *testing.T) {
	report := &AssessmentReport{
		Timestamp:  time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		ModelName:  "test-model",
		TotalScore: 45,
		MaxScore:   60,
		Percentage: 75,
		CategoryScores: map[string]CategoryScore{
			"algorithms":      {Name: "Basic Algorithms", Score: 10, MaxScore: 12, Percentage: 83},
			"data_structures": {Name: "Data Structures", Score: 9, MaxScore: 12, Percentage: 75},
			"code_quality":    {Name: "Code Quality", Score: 10, MaxScore: 12, Percentage: 83},
			"real_world":      {Name: "Real-World Tasks", Score: 8, MaxScore: 12, Percentage: 67},
			"problem_solving": {Name: "Problem Solving", Score: 8, MaxScore: 12, Percentage: 67},
		},
		RecommendTasks: []string{"Boilerplate code", "Unit tests"},
		AvoidTasks:     []string{"Security-critical code"},
	}

	markdown := GenerateJuniorMarkdown(report)

	// Check key elements are present
	checks := []string{
		"# JUNIOR.md",
		"test-model",
		"## Your Identity",
		"## Your Strengths",
		"## Areas Needing Improvement",
		"## Task Guidelines",
		"Tasks You Should Accept",
		"Tasks to Escalate to Senior",
		"## How to Use This Information",
	}

	for _, check := range checks {
		if !strings.Contains(markdown, check) {
			t.Errorf("expected JUNIOR.md to contain %q", check)
		}
	}
}

func TestWriteJuniorReport(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "assessment-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	report := &AssessmentReport{
		Timestamp:  time.Now(),
		ModelName:  "test-model",
		TotalScore: 30,
		MaxScore:   60,
		Percentage: 50,
		CategoryScores: map[string]CategoryScore{
			"algorithms": {Name: "Basic Algorithms", Score: 6, MaxScore: 12, Percentage: 50},
		},
	}

	err = WriteJuniorReport(report, tmpDir)
	if err != nil {
		t.Fatalf("WriteJuniorReport failed: %v", err)
	}

	// Check file was created
	content, err := os.ReadFile(filepath.Join(tmpDir, "JUNIOR.md"))
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}

	if !strings.Contains(string(content), "# JUNIOR.md") {
		t.Error("file doesn't contain JUNIOR.md header")
	}

	if !strings.Contains(string(content), "test-model") {
		t.Error("file doesn't contain model name")
	}
}
