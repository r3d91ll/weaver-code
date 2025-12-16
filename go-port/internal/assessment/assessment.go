package assessment

import (
	"context"
	"fmt"
	"time"
)

// JuniorClient is the interface for communicating with Junior.
type JuniorClient interface {
	Chat(ctx context.Context, message string) (string, error)
	Name() string
	IsAvailable() bool
}

// JuniorToolClient extends JuniorClient with tool-enabled chat.
type JuniorToolClient interface {
	JuniorClient
	ChatWithTools(ctx context.Context, message string) (string, error)
	ToolsEnabled() bool
}

// SeniorClient is the interface for having Senior evaluate responses.
type SeniorClient interface {
	Chat(ctx context.Context, message string) (string, error)
}

// ChallengeResult holds the result of a single challenge.
type ChallengeResult struct {
	ChallengeID string
	Category    string
	Name        string
	Score       int
	MaxScore    int
	JuniorCode  string // Junior's response
	Evaluation  string // Senior's evaluation notes
	Duration    time.Duration
}

// AssessmentReport holds the complete assessment results.
type AssessmentReport struct {
	Timestamp       time.Time
	ModelName       string
	ModelEndpoint   string
	TotalScore      int
	MaxScore        int
	Percentage      float64
	CategoryScores  map[string]CategoryScore
	Strengths       []string
	Weaknesses      []string
	RecommendTasks  []string
	AvoidTasks      []string
	ChallengeResults []ChallengeResult
}

// CategoryScore holds scores for a category.
type CategoryScore struct {
	Name       string
	Score      int
	MaxScore   int
	Percentage float64
}

// ProgressCallback is called to report assessment progress.
type ProgressCallback func(current, total int, challengeName string)

// Assessor runs the Junior assessment.
type Assessor struct {
	junior   JuniorClient
	senior   SeniorClient
	progress ProgressCallback
}

// NewAssessor creates a new assessor.
func NewAssessor(junior JuniorClient, senior SeniorClient) *Assessor {
	return &Assessor{
		junior: junior,
		senior: senior,
	}
}

// SetProgressCallback sets the progress callback.
func (a *Assessor) SetProgressCallback(cb ProgressCallback) {
	a.progress = cb
}

// Run executes the full assessment.
func (a *Assessor) Run(ctx context.Context) (*AssessmentReport, error) {
	if !a.junior.IsAvailable() {
		return nil, fmt.Errorf("junior model is not available")
	}

	challenges := ChallengeSet()
	report := &AssessmentReport{
		Timestamp:       time.Now(),
		ModelName:       a.junior.Name(),
		CategoryScores:  make(map[string]CategoryScore),
		ChallengeResults: make([]ChallengeResult, 0, len(challenges)),
	}

	// Run each challenge
	for i, challenge := range challenges {
		if a.progress != nil {
			a.progress(i+1, len(challenges), challenge.Name)
		}

		result, err := a.runChallenge(ctx, challenge)
		if err != nil {
			// Don't fail the whole assessment on one challenge error
			result = &ChallengeResult{
				ChallengeID: challenge.ID,
				Category:    challenge.Category,
				Name:        challenge.Name,
				Score:       0,
				MaxScore:    challenge.MaxScore,
				Evaluation:  fmt.Sprintf("Error: %v", err),
			}
		}

		report.ChallengeResults = append(report.ChallengeResults, *result)
		report.TotalScore += result.Score
		report.MaxScore += result.MaxScore
	}

	// Calculate category scores
	a.calculateCategoryScores(report)

	// Calculate overall percentage
	if report.MaxScore > 0 {
		report.Percentage = float64(report.TotalScore) / float64(report.MaxScore) * 100
	}

	// Generate strengths/weaknesses analysis
	a.analyzeResults(report)

	return report, nil
}

// runChallenge executes a single challenge and evaluates it.
func (a *Assessor) runChallenge(ctx context.Context, challenge Challenge) (*ChallengeResult, error) {
	result := &ChallengeResult{
		ChallengeID: challenge.ID,
		Category:    challenge.Category,
		Name:        challenge.Name,
		MaxScore:    challenge.MaxScore,
	}

	// Get timeout
	timeout := time.Duration(challenge.TimeLimit) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	// Create context with timeout
	challengeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Send challenge to Junior - use tools for tool_use challenges
	start := time.Now()
	var juniorResponse string
	var err error

	if challenge.Category == "tool_use" {
		// Try to use tool-enabled chat for tool_use challenges
		if toolClient, ok := a.junior.(JuniorToolClient); ok && toolClient.ToolsEnabled() {
			// Warm-up: Prime the tool calling behavior with a simple tool call
			// This helps models that need context to "wake up" their tool use
			warmupCtx, warmupCancel := context.WithTimeout(ctx, 30*time.Second)
			_, _ = toolClient.ChatWithTools(warmupCtx, "List the current directory with list_directory.")
			warmupCancel()

			// Enhance prompt with assessment mode and context_write guidance
			enhancedPrompt := buildToolUsePrompt(challenge.Prompt)
			juniorResponse, err = toolClient.ChatWithTools(challengeCtx, enhancedPrompt)
		} else {
			// Fallback to regular chat - Junior won't be able to use tools
			juniorResponse, err = a.junior.Chat(challengeCtx, challenge.Prompt)
		}
	} else {
		juniorResponse, err = a.junior.Chat(challengeCtx, challenge.Prompt)
	}
	result.Duration = time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("junior failed to respond: %w", err)
	}

	result.JuniorCode = juniorResponse

	// Have Senior evaluate the response
	score, evaluation, err := a.evaluateResponse(ctx, challenge, juniorResponse)
	if err != nil {
		// If Senior evaluation fails, give a default score based on response presence
		if juniorResponse != "" {
			result.Score = 1 // At least they tried
		}
		result.Evaluation = fmt.Sprintf("Evaluation error: %v", err)
		return result, nil
	}

	result.Score = score
	result.Evaluation = evaluation

	return result, nil
}

// evaluateResponse has Senior evaluate Junior's response.
func (a *Assessor) evaluateResponse(ctx context.Context, challenge Challenge, response string) (int, string, error) {
	var evalPrompt string

	if challenge.Category == "tool_use" {
		// Special rubric for tool_use challenges - evaluates tool usage
		evalPrompt = fmt.Sprintf(`You are evaluating a Junior Engineer's performance on a TOOL USE challenge. Score it 0-4:

**Scoring Rubric for Tool Use:**
- 0: Did not attempt to use tools, or completely wrong approach
- 1: Attempted tools but used them incorrectly or incompletely
- 2: Used tools correctly but missed some steps or had minor issues
- 3: Completed all required steps using appropriate tools
- 4: Excellent tool usage, efficient workflow, clear reporting of results

**Challenge:** %s

**Challenge Prompt:**
%s

**Junior's Response (including tool calls and results):**
%s

**Your Task:**
1. Check if Junior used the appropriate tools (read_file, write_file, list_directory, execute_command, search_files)
2. Verify they completed the required steps
3. Assess if files were created/modified as requested
4. Evaluate the quality of their final report

Respond in this exact format:
SCORE: [0-4]
EVALUATION: [Your brief evaluation - 2-3 sentences max]`, challenge.Name, challenge.Prompt, response)
	} else {
		evalPrompt = fmt.Sprintf(`You are evaluating a Junior Engineer's code submission. Score it 0-4:

**Scoring Rubric:**
- 0: Failed, incorrect, or no code provided
- 1: Works but poor quality (e.g., works but inefficient, hard to read)
- 2: Correct with minor issues (e.g., missing edge cases, style issues)
- 3: Correct and clean (meets requirements well)
- 4: Excellent (exceeds requirements, elegant solution)

**Challenge:** %s

**Challenge Prompt:**
%s

**Junior's Response:**
%s

**Your Task:**
1. Evaluate the code against the requirements
2. Note any bugs, issues, or quality problems
3. Provide a score (0-4) and brief explanation

Respond in this exact format:
SCORE: [0-4]
EVALUATION: [Your brief evaluation - 2-3 sentences max]`, challenge.Name, challenge.Prompt, response)
	}

	evalResponse, err := a.senior.Chat(ctx, evalPrompt)
	if err != nil {
		return 0, "", err
	}

	// Parse the response
	score, evaluation := parseEvaluation(evalResponse)
	return score, evaluation, nil
}

// parseEvaluation extracts score and evaluation text from Senior's response.
func parseEvaluation(response string) (int, string) {
	score := 0
	evaluation := response

	// Try to find "SCORE: X" pattern
	var foundScore bool
	for i := 0; i < len(response)-7; i++ {
		if response[i:i+6] == "SCORE:" {
			// Look for the number
			for j := i + 6; j < len(response) && j < i+10; j++ {
				if response[j] >= '0' && response[j] <= '4' {
					score = int(response[j] - '0')
					foundScore = true
					break
				}
			}
			break
		}
	}

	// Try to find "EVALUATION:" and extract text after it
	for i := 0; i < len(response)-11; i++ {
		if response[i:i+11] == "EVALUATION:" {
			evaluation = response[i+11:]
			// Trim leading whitespace
			for len(evaluation) > 0 && (evaluation[0] == ' ' || evaluation[0] == '\n') {
				evaluation = evaluation[1:]
			}
			break
		}
	}

	// If no structured format found, try to infer score from response
	if !foundScore {
		// Look for common score patterns in natural language
		if containsAny(response, []string{"excellent", "perfect", "outstanding"}) {
			score = 4
		} else if containsAny(response, []string{"correct", "good", "works well"}) {
			score = 3
		} else if containsAny(response, []string{"minor issues", "mostly correct", "some problems"}) {
			score = 2
		} else if containsAny(response, []string{"poor", "needs work", "barely"}) {
			score = 1
		}
	}

	return score, evaluation
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if containsIgnoreCase(s, sub) {
			return true
		}
	}
	return false
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	// Simple lowercase comparison
	sl := toLower(s)
	subl := toLower(substr)
	for i := 0; i <= len(sl)-len(subl); i++ {
		if sl[i:i+len(subl)] == subl {
			return true
		}
	}
	return false
}

// toLower converts string to lowercase (ASCII only).
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

// buildToolUsePrompt enhances a tool_use challenge prompt with assessment guidance.
// This helps models that benefit from explicit instructions about tool invocation
// and state tracking during multi-step workflows.
func buildToolUsePrompt(originalPrompt string) string {
	return `**ASSESSMENT MODE - Tool Use Challenge**

You are being assessed on your ability to use tools effectively.
IMPORTANT: You must INVOKE tools, not describe what you would do.

**Guidelines:**
1. Use context_write to track your progress (e.g., "Step 1 complete: listed directory")
2. Actually call tools - don't just say "I would use read_file..."
3. Report results clearly after each tool operation
4. If a step fails, explain what happened

**Your Task:**
` + originalPrompt + `

Remember: INVOKE the tools. The system will execute them and return results.`
}

// calculateCategoryScores calculates per-category scores.
func (a *Assessor) calculateCategoryScores(report *AssessmentReport) {
	categoryTotals := make(map[string]int)
	categoryMax := make(map[string]int)

	for _, result := range report.ChallengeResults {
		categoryTotals[result.Category] += result.Score
		categoryMax[result.Category] += result.MaxScore
	}

	names := CategoryNames()
	for cat, total := range categoryTotals {
		max := categoryMax[cat]
		pct := float64(0)
		if max > 0 {
			pct = float64(total) / float64(max) * 100
		}
		report.CategoryScores[cat] = CategoryScore{
			Name:       names[cat],
			Score:      total,
			MaxScore:   max,
			Percentage: pct,
		}
	}
}

// analyzeResults generates strengths, weaknesses, and recommendations.
func (a *Assessor) analyzeResults(report *AssessmentReport) {
	// Analyze category performance
	for _, cat := range CategoryOrder() {
		score, ok := report.CategoryScores[cat]
		if !ok {
			continue
		}

		if score.Percentage >= 80 {
			report.Strengths = append(report.Strengths,
				fmt.Sprintf("Strong %s skills (%.0f%%)", score.Name, score.Percentage))
		} else if score.Percentage < 50 {
			report.Weaknesses = append(report.Weaknesses,
				fmt.Sprintf("Needs improvement in %s (%.0f%%)", score.Name, score.Percentage))
		}
	}

	// Generate task recommendations based on performance
	if report.Percentage >= 70 {
		report.RecommendTasks = append(report.RecommendTasks,
			"Boilerplate code generation",
			"Unit test writing",
			"Documentation and docstrings",
			"Simple refactoring tasks",
		)
	}

	if cs, ok := report.CategoryScores["data_structures"]; ok && cs.Percentage >= 70 {
		report.RecommendTasks = append(report.RecommendTasks,
			"Data structure implementations",
			"Algorithm implementations",
		)
	}

	if cs, ok := report.CategoryScores["code_quality"]; ok && cs.Percentage >= 80 {
		report.RecommendTasks = append(report.RecommendTasks,
			"Code review assistance",
			"Adding type hints and documentation",
		)
	}

	// Tasks to avoid
	if report.Percentage < 60 {
		report.AvoidTasks = append(report.AvoidTasks,
			"Complex architecture decisions",
			"Security-critical code",
		)
	}

	if cs, ok := report.CategoryScores["problem_solving"]; ok && cs.Percentage < 60 {
		report.AvoidTasks = append(report.AvoidTasks,
			"Debugging complex issues",
			"Performance optimization",
		)
	}

	if cs, ok := report.CategoryScores["real_world"]; ok && cs.Percentage < 60 {
		report.AvoidTasks = append(report.AvoidTasks,
			"Production error handling",
			"External API integration",
		)
	}

	// Tool use recommendations
	if cs, ok := report.CategoryScores["tool_use"]; ok {
		if cs.Percentage >= 70 {
			report.RecommendTasks = append(report.RecommendTasks,
				"File manipulation tasks",
				"Script creation and testing",
				"Codebase exploration",
			)
		} else if cs.Percentage < 50 {
			report.Weaknesses = append(report.Weaknesses,
				fmt.Sprintf("Limited tool proficiency (%.0f%%)", cs.Percentage))
			report.AvoidTasks = append(report.AvoidTasks,
				"Multi-step file operations",
				"Complex automated workflows",
			)
		}
	}

	// Always avoid
	report.AvoidTasks = append(report.AvoidTasks,
		"Security-critical authentication/authorization",
		"Complex concurrent/async patterns",
	)
}
