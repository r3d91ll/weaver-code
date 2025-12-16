package assessment

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ExtendedAssessmentConfig configures an extended assessment run.
type ExtendedAssessmentConfig struct {
	// NumChallenges is the total number of challenges to run (e.g., 1000)
	NumChallenges int

	// OutputDir is where training data will be exported
	OutputDir string

	// ContinueOnError keeps running even if individual challenges fail
	ContinueOnError bool

	// SaveInterval saves progress every N challenges (0 = only at end)
	SaveInterval int

	// ExportFormats specifies which formats to export ("jsonl", "alpaca", "sharegpt", "dpo")
	ExportFormats []string

	// MinQualityForExport is the minimum quality to include in filtered exports
	MinQualityForExport string // "excellent", "good", "poor", "failed"
}

// DefaultExtendedConfig returns sensible defaults for overnight runs.
func DefaultExtendedConfig() ExtendedAssessmentConfig {
	return ExtendedAssessmentConfig{
		NumChallenges:       1000,
		OutputDir:           "./training_data",
		ContinueOnError:     true,
		SaveInterval:        100, // Save every 100 challenges
		ExportFormats:       []string{"jsonl", "alpaca", "sharegpt", "dpo"},
		MinQualityForExport: "good", // Include good and excellent in filtered exports
	}
}

// ExtendedProgressCallback reports progress during extended assessment.
type ExtendedProgressCallback func(completed, total int, lastChallenge string, elapsed time.Duration, estimatedRemaining time.Duration)

// ExtendedAssessor runs large-scale assessments for training data collection.
type ExtendedAssessor struct {
	junior    JuniorClient
	senior    SeniorClient
	generator *ChallengeGenerator
	exporter  *TrainingExporter
	config    ExtendedAssessmentConfig
	progress  ExtendedProgressCallback
}

// NewExtendedAssessor creates an assessor for large-scale training data collection.
func NewExtendedAssessor(junior JuniorClient, senior SeniorClient, config ExtendedAssessmentConfig) *ExtendedAssessor {
	return &ExtendedAssessor{
		junior:    junior,
		senior:    senior,
		generator: NewChallengeGenerator(),
		exporter:  NewTrainingExporter(config.OutputDir),
		config:    config,
	}
}

// SetProgressCallback sets the progress callback.
func (a *ExtendedAssessor) SetProgressCallback(cb ExtendedProgressCallback) {
	a.progress = cb
}

// Run executes the extended assessment.
func (a *ExtendedAssessor) Run(ctx context.Context) (*ExtendedAssessmentReport, error) {
	if !a.junior.IsAvailable() {
		return nil, fmt.Errorf("junior model is not available")
	}

	// Create output directory
	if err := os.MkdirAll(a.config.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate challenges
	challenges := a.generator.GenerateExtendedSet(a.config.NumChallenges)

	report := &ExtendedAssessmentReport{
		StartTime:      time.Now(),
		ModelName:      a.junior.Name(),
		TotalChallenges: len(challenges),
		Config:         a.config,
		CategoryStats:  make(map[string]*CategoryStats),
	}

	// Initialize category stats
	for _, cat := range CategoryOrder() {
		report.CategoryStats[cat] = &CategoryStats{}
	}

	startTime := time.Now()
	var lastSaveCount int

	// Run each challenge
	for i, challenge := range challenges {
		// Check for cancellation
		select {
		case <-ctx.Done():
			report.EndTime = time.Now()
			report.WasCancelled = true
			a.saveProgress(report)
			return report, ctx.Err()
		default:
		}

		// Report progress
		if a.progress != nil {
			elapsed := time.Since(startTime)
			avgPerChallenge := elapsed / time.Duration(i+1)
			remaining := avgPerChallenge * time.Duration(len(challenges)-i-1)
			a.progress(i+1, len(challenges), challenge.Name, elapsed, remaining)
		}

		// Run the challenge
		result, err := a.runSingleChallenge(ctx, challenge)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("Challenge %s: %v", challenge.ID, err))
			if !a.config.ContinueOnError {
				report.EndTime = time.Now()
				return report, err
			}
			// Create a failed result
			result = &ChallengeResult{
				ChallengeID: challenge.ID,
				Category:    challenge.Category,
				Name:        challenge.Name,
				Score:       0,
				MaxScore:    challenge.MaxScore,
				Evaluation:  fmt.Sprintf("Error: %v", err),
			}
		}

		// Add to exporter
		a.exporter.AddResult(challenge, *result)

		// Update stats
		report.CompletedChallenges++
		report.TotalScore += result.Score
		report.MaxPossibleScore += result.MaxScore

		if stats, ok := report.CategoryStats[challenge.Category]; ok {
			stats.Completed++
			stats.TotalScore += result.Score
			stats.MaxScore += result.MaxScore
		}

		// Periodic save
		if a.config.SaveInterval > 0 && (i+1-lastSaveCount) >= a.config.SaveInterval {
			if err := a.saveProgress(report); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("Save error: %v", err))
			}
			lastSaveCount = i + 1
		}
	}

	report.EndTime = time.Now()

	// Final export
	if err := a.exportAll(); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("Export error: %v", err))
	}

	return report, nil
}

func (a *ExtendedAssessor) runSingleChallenge(ctx context.Context, challenge Challenge) (*ChallengeResult, error) {
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

	// Send challenge to Junior
	start := time.Now()
	var juniorResponse string
	var err error

	if challenge.Category == "tool_use" {
		if toolClient, ok := a.junior.(JuniorToolClient); ok && toolClient.ToolsEnabled() {
			// Warm-up for tool challenges
			warmupCtx, warmupCancel := context.WithTimeout(ctx, 30*time.Second)
			_, _ = toolClient.ChatWithTools(warmupCtx, "List the current directory with list_directory.")
			warmupCancel()

			enhancedPrompt := buildToolUsePrompt(challenge.Prompt)
			juniorResponse, err = toolClient.ChatWithTools(challengeCtx, enhancedPrompt)
		} else {
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

	// Evaluate - use shorter timeout for evaluation to avoid long waits
	evalCtx, evalCancel := context.WithTimeout(ctx, 60*time.Second)
	defer evalCancel()

	score, evaluation, err := a.evaluateResponse(evalCtx, challenge, juniorResponse)
	if err != nil {
		// Default score if evaluation fails
		if juniorResponse != "" {
			result.Score = 1
		}
		result.Evaluation = fmt.Sprintf("Evaluation error: %v", err)
		return result, nil
	}

	result.Score = score
	result.Evaluation = evaluation

	return result, nil
}

func (a *ExtendedAssessor) evaluateResponse(ctx context.Context, challenge Challenge, response string) (int, string, error) {
	// Use simplified evaluation for extended runs to save time
	evalPrompt := fmt.Sprintf(`Rate this code response 0-4:
0=Failed/no code, 1=Poor, 2=Minor issues, 3=Good, 4=Excellent

Challenge: %s
Response: %s

Reply with just: SCORE: [0-4]
EVALUATION: [one sentence]`, challenge.Name, truncateForEval(response, 2000))

	evalResponse, err := a.senior.Chat(ctx, evalPrompt)
	if err != nil {
		return 0, "", err
	}

	score, evaluation := parseEvaluation(evalResponse)
	return score, evaluation, nil
}

func truncateForEval(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... [truncated]"
}

func (a *ExtendedAssessor) saveProgress(report *ExtendedAssessmentReport) error {
	// Export current state
	return a.exporter.Export()
}

func (a *ExtendedAssessor) exportAll() error {
	// Export all formats
	if err := a.exporter.Export(); err != nil {
		return err
	}

	for _, format := range a.config.ExportFormats {
		switch format {
		case "alpaca":
			if err := a.exporter.ExportAlpacaFormat(a.config.MinQualityForExport); err != nil {
				return err
			}
		case "sharegpt":
			if err := a.exporter.ExportShareGPTFormat(a.config.MinQualityForExport); err != nil {
				return err
			}
		case "dpo":
			if err := a.exporter.ExportDPOFormat(); err != nil {
				return err
			}
		}
	}

	return nil
}

// ExtendedAssessmentReport holds results from an extended assessment run.
type ExtendedAssessmentReport struct {
	StartTime           time.Time
	EndTime             time.Time
	ModelName           string
	TotalChallenges     int
	CompletedChallenges int
	TotalScore          int
	MaxPossibleScore    int
	CategoryStats       map[string]*CategoryStats
	Errors              []string
	WasCancelled        bool
	Config              ExtendedAssessmentConfig
}

// CategoryStats holds per-category statistics.
type CategoryStats struct {
	Completed  int
	TotalScore int
	MaxScore   int
}

// Duration returns the total duration of the assessment.
func (r *ExtendedAssessmentReport) Duration() time.Duration {
	return r.EndTime.Sub(r.StartTime)
}

// OverallPercentage returns the overall score percentage.
func (r *ExtendedAssessmentReport) OverallPercentage() float64 {
	if r.MaxPossibleScore == 0 {
		return 0
	}
	return float64(r.TotalScore) / float64(r.MaxPossibleScore) * 100
}

// Summary returns a text summary of the extended assessment.
func (r *ExtendedAssessmentReport) Summary() string {
	var sb strings.Builder

	sb.WriteString("\n=== Extended Assessment Complete ===\n\n")
	sb.WriteString(fmt.Sprintf("Model: %s\n", r.ModelName))
	sb.WriteString(fmt.Sprintf("Duration: %s\n", r.Duration().Round(time.Second)))
	sb.WriteString(fmt.Sprintf("Challenges: %d/%d completed\n", r.CompletedChallenges, r.TotalChallenges))
	sb.WriteString(fmt.Sprintf("Overall Score: %d/%d (%.1f%%)\n\n", r.TotalScore, r.MaxPossibleScore, r.OverallPercentage()))

	sb.WriteString("Category Breakdown:\n")
	for _, cat := range CategoryOrder() {
		if stats, ok := r.CategoryStats[cat]; ok && stats.Completed > 0 {
			pct := float64(0)
			if stats.MaxScore > 0 {
				pct = float64(stats.TotalScore) / float64(stats.MaxScore) * 100
			}
			name := CategoryNames()[cat]
			sb.WriteString(fmt.Sprintf("  %s: %d/%d (%.1f%%)\n", name, stats.TotalScore, stats.MaxScore, pct))
		}
	}

	if len(r.Errors) > 0 {
		sb.WriteString(fmt.Sprintf("\nErrors: %d\n", len(r.Errors)))
		for i, err := range r.Errors {
			if i >= 5 {
				sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(r.Errors)-5))
				break
			}
			sb.WriteString(fmt.Sprintf("  - %s\n", err))
		}
	}

	sb.WriteString(fmt.Sprintf("\nTraining data exported to: %s\n", r.Config.OutputDir))

	if r.WasCancelled {
		sb.WriteString("\n[Assessment was cancelled - partial results saved]\n")
	}

	return sb.String()
}

// WriteExtendedReport writes the extended report to a file.
func WriteExtendedReport(report *ExtendedAssessmentReport, outputDir string) error {
	path := filepath.Join(outputDir, "assessment_report.txt")
	return os.WriteFile(path, []byte(report.Summary()), 0644)
}
