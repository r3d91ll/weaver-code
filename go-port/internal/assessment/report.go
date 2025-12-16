package assessment

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GenerateMarkdown generates the CLAUDE.md assessment report content.
func GenerateMarkdown(report *AssessmentReport) string {
	var sb strings.Builder

	sb.WriteString("## Junior Engineer Assessment\n\n")

	// Header info
	sb.WriteString(fmt.Sprintf("**Last Assessment:** %s\n", report.Timestamp.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**Model:** %s\n", report.ModelName))
	if report.ModelEndpoint != "" {
		sb.WriteString(fmt.Sprintf("**Endpoint:** %s\n", report.ModelEndpoint))
	}
	sb.WriteString("\n")

	// Overall score
	sb.WriteString(fmt.Sprintf("**Overall Score:** %d/%d (%.0f%%)\n\n",
		report.TotalScore, report.MaxScore, report.Percentage))

	// Category results table
	sb.WriteString("### Results by Category\n\n")
	sb.WriteString("| Category | Score | Percentage |\n")
	sb.WriteString("|----------|-------|------------|\n")

	for _, cat := range CategoryOrder() {
		if score, ok := report.CategoryScores[cat]; ok {
			sb.WriteString(fmt.Sprintf("| %s | %d/%d | %.0f%% |\n",
				score.Name, score.Score, score.MaxScore, score.Percentage))
		}
	}
	sb.WriteString(fmt.Sprintf("| **Total** | **%d/%d** | **%.0f%%** |\n\n",
		report.TotalScore, report.MaxScore, report.Percentage))

	// Strengths
	if len(report.Strengths) > 0 {
		sb.WriteString("### Strengths\n\n")
		for _, s := range report.Strengths {
			sb.WriteString(fmt.Sprintf("- %s\n", s))
		}
		sb.WriteString("\n")
	}

	// Weaknesses
	if len(report.Weaknesses) > 0 {
		sb.WriteString("### Weaknesses\n\n")
		for _, w := range report.Weaknesses {
			sb.WriteString(fmt.Sprintf("- %s\n", w))
		}
		sb.WriteString("\n")
	}

	// Delegation guidelines
	sb.WriteString("### Delegation Guidelines\n\n")

	if len(report.RecommendTasks) > 0 {
		sb.WriteString("**Recommended Tasks:**\n")
		for _, t := range report.RecommendTasks {
			sb.WriteString(fmt.Sprintf("- %s\n", t))
		}
		sb.WriteString("\n")
	}

	if len(report.AvoidTasks) > 0 {
		sb.WriteString("**Avoid Delegating:**\n")
		for _, t := range report.AvoidTasks {
			sb.WriteString(fmt.Sprintf("- %s\n", t))
		}
		sb.WriteString("\n")
	}

	// Detailed results (collapsed by default in most viewers)
	sb.WriteString("### Challenge Details\n\n")
	sb.WriteString("<details>\n")
	sb.WriteString("<summary>Click to expand individual challenge results</summary>\n\n")

	for _, result := range report.ChallengeResults {
		sb.WriteString(fmt.Sprintf("#### %s (%s)\n", result.Name, result.Category))
		sb.WriteString(fmt.Sprintf("**Score:** %d/%d\n\n", result.Score, result.MaxScore))
		if result.Evaluation != "" {
			sb.WriteString(fmt.Sprintf("**Evaluation:** %s\n\n", result.Evaluation))
		}
		if result.Duration > 0 {
			sb.WriteString(fmt.Sprintf("**Response Time:** %s\n\n", result.Duration.Round(100*1000000)))
		}
		sb.WriteString("---\n\n")
	}

	sb.WriteString("</details>\n")

	return sb.String()
}

// WriteReport writes the assessment report to CLAUDE.md.
// If the file exists, it updates the Junior Assessment section.
// If not, it creates a new file with the assessment.
func WriteReport(report *AssessmentReport, dir string) error {
	claudeMdPath := filepath.Join(dir, "CLAUDE.md")

	// Generate new assessment content
	newAssessment := GenerateMarkdown(report)

	// Check if file exists
	content, err := os.ReadFile(claudeMdPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new file
			newContent := "# CLAUDE.md\n\nThis file provides guidance to Claude Code when working in this project.\n\n" + newAssessment
			return os.WriteFile(claudeMdPath, []byte(newContent), 0644)
		}
		return fmt.Errorf("failed to read CLAUDE.md: %w", err)
	}

	// File exists - update or append the assessment section
	existingContent := string(content)
	updatedContent := updateAssessmentSection(existingContent, newAssessment)

	return os.WriteFile(claudeMdPath, []byte(updatedContent), 0644)
}

// updateAssessmentSection updates or appends the Junior Assessment section.
func updateAssessmentSection(existing, newAssessment string) string {
	// Look for existing assessment section
	startMarker := "## Junior Engineer Assessment"
	endMarkers := []string{"## ", "# "} // Next section starts

	startIdx := strings.Index(existing, startMarker)
	if startIdx == -1 {
		// No existing assessment, append it
		return existing + "\n" + newAssessment
	}

	// Find where the section ends (next ## or # heading)
	endIdx := len(existing)
	searchStart := startIdx + len(startMarker)

	for _, marker := range endMarkers {
		idx := strings.Index(existing[searchStart:], marker)
		if idx != -1 && searchStart+idx < endIdx {
			// Make sure it's at the start of a line
			checkIdx := searchStart + idx
			if checkIdx == 0 || existing[checkIdx-1] == '\n' {
				endIdx = checkIdx
			}
		}
	}

	// Replace the section
	return existing[:startIdx] + newAssessment + existing[endIdx:]
}

// GenerateJuniorMarkdown generates the JUNIOR.md content for the local model.
// This file is meant to be included in Junior's system prompt so it knows its capabilities.
func GenerateJuniorMarkdown(report *AssessmentReport) string {
	var sb strings.Builder

	sb.WriteString("# JUNIOR.md\n\n")
	sb.WriteString("This file describes your capabilities based on your assessment results.\n")
	sb.WriteString("Use this information to know what tasks you're good at and where you need Senior's help.\n\n")

	// Identity
	sb.WriteString("## Your Identity\n\n")
	sb.WriteString(fmt.Sprintf("- **Model:** %s\n", report.ModelName))
	sb.WriteString(fmt.Sprintf("- **Assessment Date:** %s\n", report.Timestamp.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("- **Overall Score:** %d/%d (%.0f%%)\n\n", report.TotalScore, report.MaxScore, report.Percentage))

	// Strengths - what Junior is good at
	sb.WriteString("## Your Strengths\n\n")
	sb.WriteString("You performed well in these areas. Accept tasks in these categories confidently:\n\n")

	hasStrengths := false
	for _, cat := range CategoryOrder() {
		if score, ok := report.CategoryScores[cat]; ok && score.Percentage >= 70 {
			hasStrengths = true
			sb.WriteString(fmt.Sprintf("- **%s** (%.0f%%): ", score.Name, score.Percentage))
			switch cat {
			case "algorithms":
				sb.WriteString("You can implement standard algorithms like sorting, searching, and basic data transformations.\n")
			case "data_structures":
				sb.WriteString("You understand and can implement common data structures correctly.\n")
			case "code_quality":
				sb.WriteString("You write clean, well-structured code with good error handling.\n")
			case "real_world":
				sb.WriteString("You can handle practical tasks like API endpoints, database queries, and config parsing.\n")
			case "problem_solving":
				sb.WriteString("You can design solutions for moderately complex problems.\n")
			}
		}
	}
	if !hasStrengths {
		sb.WriteString("- No categories scored above 70%. Ask Senior for guidance on most tasks.\n")
	}
	sb.WriteString("\n")

	// Weaknesses - where Junior needs help
	sb.WriteString("## Areas Needing Improvement\n\n")
	sb.WriteString("You struggled in these areas. Ask Senior for help or clarification:\n\n")

	hasWeaknesses := false
	for _, cat := range CategoryOrder() {
		if score, ok := report.CategoryScores[cat]; ok && score.Percentage < 60 {
			hasWeaknesses = true
			sb.WriteString(fmt.Sprintf("- **%s** (%.0f%%): ", score.Name, score.Percentage))
			switch cat {
			case "algorithms":
				sb.WriteString("You may make mistakes with algorithm implementation. Double-check your logic.\n")
			case "data_structures":
				sb.WriteString("Data structure implementations may have bugs. Ask Senior to review.\n")
			case "code_quality":
				sb.WriteString("Your code may need cleanup. Focus on error handling and clarity.\n")
			case "real_world":
				sb.WriteString("Practical tasks may need Senior's review for completeness.\n")
			case "problem_solving":
				sb.WriteString("Complex problems should be escalated to Senior.\n")
			}
		}
	}
	if !hasWeaknesses {
		sb.WriteString("- No major weaknesses identified. Continue doing good work!\n")
	}
	sb.WriteString("\n")

	// Guidelines
	sb.WriteString("## Task Guidelines\n\n")

	sb.WriteString("### Tasks You Should Accept\n\n")
	if len(report.RecommendTasks) > 0 {
		for _, t := range report.RecommendTasks {
			sb.WriteString(fmt.Sprintf("- %s\n", t))
		}
	} else {
		sb.WriteString("- Simple, well-defined coding tasks\n")
		sb.WriteString("- Documentation and comments\n")
		sb.WriteString("- Code formatting and cleanup\n")
	}
	sb.WriteString("\n")

	sb.WriteString("### Tasks to Escalate to Senior\n\n")
	if len(report.AvoidTasks) > 0 {
		for _, t := range report.AvoidTasks {
			sb.WriteString(fmt.Sprintf("- %s\n", t))
		}
	} else {
		sb.WriteString("- Security-critical code\n")
		sb.WriteString("- Architecture decisions\n")
		sb.WriteString("- Complex multi-step problems\n")
	}
	sb.WriteString("\n")

	// Behavioral guidance
	sb.WriteString("## How to Use This Information\n\n")
	sb.WriteString("1. **Know your limits**: If a task falls in your weak areas, say so upfront\n")
	sb.WriteString("2. **Be confident in strengths**: When tasks match your strengths, deliver with confidence\n")
	sb.WriteString("3. **Ask for clarification**: If requirements are unclear, ask before implementing\n")
	sb.WriteString("4. **Report blockers**: If you can't complete something, explain why\n")

	return sb.String()
}

// WriteJuniorReport writes the JUNIOR.md file.
func WriteJuniorReport(report *AssessmentReport, dir string) error {
	juniorMdPath := filepath.Join(dir, "JUNIOR.md")
	content := GenerateJuniorMarkdown(report)
	return os.WriteFile(juniorMdPath, []byte(content), 0644)
}

// GenerateSummary returns a short summary for console output.
func GenerateSummary(report *AssessmentReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n  Model: %s\n", report.ModelName))
	sb.WriteString(fmt.Sprintf("  Score: %d/%d (%.0f%%)\n", report.TotalScore, report.MaxScore, report.Percentage))

	if len(report.Strengths) > 0 {
		sb.WriteString("  Strengths: ")
		for i, s := range report.Strengths {
			if i > 0 {
				sb.WriteString(", ")
			}
			// Extract just the category name
			s = strings.TrimPrefix(s, "Strong ")
			s = strings.TrimSuffix(s, fmt.Sprintf(" (%.0f%%)", report.CategoryScores[CategoryOrder()[0]].Percentage))
			sb.WriteString(s)
			if i >= 2 {
				break
			}
		}
		sb.WriteString("\n")
	}

	if len(report.Weaknesses) > 0 {
		sb.WriteString("  Weaknesses: ")
		for i, w := range report.Weaknesses {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(w)
			if i >= 2 {
				break
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n  Files updated:\n")
	sb.WriteString("  - CLAUDE.md (Senior's reference)\n")
	sb.WriteString("  - JUNIOR.md (Junior's self-awareness prompt)\n")

	return sb.String()
}
