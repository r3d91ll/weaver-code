package assessment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TrainingExample represents a single training example for fine-tuning.
type TrainingExample struct {
	// Prompt is the input/instruction given to the model
	Prompt string `json:"prompt"`

	// Completion is the model's response
	Completion string `json:"completion"`

	// Category of the challenge
	Category string `json:"category"`

	// ChallengeName for reference
	ChallengeName string `json:"challenge_name"`

	// Score achieved (0-4)
	Score int `json:"score"`

	// MaxScore possible
	MaxScore int `json:"max_score"`

	// Quality label for filtering
	Quality string `json:"quality"` // "excellent", "good", "poor", "failed"

	// ToolCalls extracted from the response (for tool_use training)
	ToolCalls []ToolCallRecord `json:"tool_calls,omitempty"`

	// Duration of the response
	DurationSeconds float64 `json:"duration_seconds"`

	// Evaluation from Senior (useful context)
	Evaluation string `json:"evaluation,omitempty"`

	// Timestamp
	Timestamp string `json:"timestamp"`
}

// ToolCallRecord captures a tool invocation for training data.
type ToolCallRecord struct {
	ToolName  string `json:"tool_name"`
	Arguments string `json:"arguments"`
	Result    string `json:"result"`
}

// TrainingExporter exports assessment results as training data.
type TrainingExporter struct {
	outputDir string
	examples  []TrainingExample
}

// NewTrainingExporter creates a new exporter.
func NewTrainingExporter(outputDir string) *TrainingExporter {
	return &TrainingExporter{
		outputDir: outputDir,
		examples:  make([]TrainingExample, 0),
	}
}

// AddResult adds a challenge result as a training example.
func (e *TrainingExporter) AddResult(challenge Challenge, result ChallengeResult) {
	quality := classifyQuality(result.Score, result.MaxScore)

	example := TrainingExample{
		Prompt:          challenge.Prompt,
		Completion:      result.JuniorCode,
		Category:        challenge.Category,
		ChallengeName:   challenge.Name,
		Score:           result.Score,
		MaxScore:        result.MaxScore,
		Quality:         quality,
		DurationSeconds: result.Duration.Seconds(),
		Evaluation:      result.Evaluation,
		Timestamp:       time.Now().Format(time.RFC3339),
	}

	// Extract tool calls if present in completion
	if challenge.Category == "tool_use" {
		example.ToolCalls = extractToolCallRecords(result.JuniorCode)
	}

	e.examples = append(e.examples, example)
}

// classifyQuality converts score to quality label.
func classifyQuality(score, maxScore int) string {
	if maxScore == 0 {
		return "failed"
	}
	pct := float64(score) / float64(maxScore) * 100

	switch {
	case pct >= 90:
		return "excellent"
	case pct >= 70:
		return "good"
	case pct >= 40:
		return "poor"
	default:
		return "failed"
	}
}

// extractToolCallRecords parses tool call log from response.
func extractToolCallRecords(response string) []ToolCallRecord {
	records := make([]ToolCallRecord, 0)

	// Look for our tool execution log format
	lines := strings.Split(response, "\n")
	var currentTool *ToolCallRecord

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "[TOOL CALL]") {
			if currentTool != nil {
				records = append(records, *currentTool)
			}
			toolName := strings.TrimSpace(strings.TrimPrefix(line, "[TOOL CALL]"))
			currentTool = &ToolCallRecord{ToolName: toolName}
		} else if strings.HasPrefix(line, "Arguments:") && currentTool != nil {
			currentTool.Arguments = strings.TrimSpace(strings.TrimPrefix(line, "Arguments:"))
		} else if strings.HasPrefix(line, "Result:") && currentTool != nil {
			currentTool.Result = strings.TrimSpace(strings.TrimPrefix(line, "Result:"))
		}
	}

	if currentTool != nil {
		records = append(records, *currentTool)
	}

	return records
}

// Export writes all examples to JSONL files.
func (e *TrainingExporter) Export() error {
	if err := os.MkdirAll(e.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")

	// Export all examples
	allPath := filepath.Join(e.outputDir, fmt.Sprintf("training_all_%s.jsonl", timestamp))
	if err := e.writeJSONL(allPath, e.examples); err != nil {
		return err
	}

	// Export by quality for easy filtering
	byQuality := e.groupByQuality()
	for quality, examples := range byQuality {
		if len(examples) == 0 {
			continue
		}
		path := filepath.Join(e.outputDir, fmt.Sprintf("training_%s_%s.jsonl", quality, timestamp))
		if err := e.writeJSONL(path, examples); err != nil {
			return err
		}
	}

	// Export by category
	byCategory := e.groupByCategory()
	for category, examples := range byCategory {
		if len(examples) == 0 {
			continue
		}
		path := filepath.Join(e.outputDir, fmt.Sprintf("training_%s_%s.jsonl", category, timestamp))
		if err := e.writeJSONL(path, examples); err != nil {
			return err
		}
	}

	// Write summary
	summaryPath := filepath.Join(e.outputDir, fmt.Sprintf("training_summary_%s.json", timestamp))
	if err := e.writeSummary(summaryPath); err != nil {
		return err
	}

	return nil
}

func (e *TrainingExporter) writeJSONL(path string, examples []TrainingExample) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	for _, ex := range examples {
		if err := encoder.Encode(ex); err != nil {
			return fmt.Errorf("failed to encode example: %w", err)
		}
	}

	return nil
}

func (e *TrainingExporter) groupByQuality() map[string][]TrainingExample {
	result := make(map[string][]TrainingExample)
	for _, ex := range e.examples {
		result[ex.Quality] = append(result[ex.Quality], ex)
	}
	return result
}

func (e *TrainingExporter) groupByCategory() map[string][]TrainingExample {
	result := make(map[string][]TrainingExample)
	for _, ex := range e.examples {
		result[ex.Category] = append(result[ex.Category], ex)
	}
	return result
}

// TrainingSummary holds statistics about exported training data.
type TrainingSummary struct {
	Timestamp      string         `json:"timestamp"`
	TotalExamples  int            `json:"total_examples"`
	ByQuality      map[string]int `json:"by_quality"`
	ByCategory     map[string]int `json:"by_category"`
	AverageScore   float64        `json:"average_score"`
	ToolCallCount  int            `json:"tool_call_count"`
	OutputFiles    []string       `json:"output_files"`
}

func (e *TrainingExporter) writeSummary(path string) error {
	summary := TrainingSummary{
		Timestamp:     time.Now().Format(time.RFC3339),
		TotalExamples: len(e.examples),
		ByQuality:     make(map[string]int),
		ByCategory:    make(map[string]int),
	}

	var totalScore, totalMax int
	for _, ex := range e.examples {
		summary.ByQuality[ex.Quality]++
		summary.ByCategory[ex.Category]++
		totalScore += ex.Score
		totalMax += ex.MaxScore
		summary.ToolCallCount += len(ex.ToolCalls)
	}

	if totalMax > 0 {
		summary.AverageScore = float64(totalScore) / float64(totalMax) * 100
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(summary)
}

// Count returns the number of examples collected.
func (e *TrainingExporter) Count() int {
	return len(e.examples)
}

// AlpacaExample is the format used by Alpaca-style fine-tuning.
type AlpacaExample struct {
	Instruction string `json:"instruction"`
	Input       string `json:"input"`
	Output      string `json:"output"`
}

// ExportAlpacaFormat exports in Alpaca format (instruction, input, output).
func (e *TrainingExporter) ExportAlpacaFormat(minQuality string) error {
	timestamp := time.Now().Format("20060102_150405")
	path := filepath.Join(e.outputDir, fmt.Sprintf("alpaca_%s_%s.json", minQuality, timestamp))

	qualityOrder := map[string]int{"excellent": 4, "good": 3, "poor": 2, "failed": 1}
	minQualityLevel := qualityOrder[minQuality]

	var filtered []AlpacaExample
	for _, ex := range e.examples {
		if qualityOrder[ex.Quality] >= minQualityLevel {
			filtered = append(filtered, AlpacaExample{
				Instruction: ex.Prompt,
				Input:       "", // We include everything in instruction
				Output:      ex.Completion,
			})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(filtered)
}

// ShareGPTMessage is a message in ShareGPT format.
type ShareGPTMessage struct {
	From  string `json:"from"` // "human" or "gpt"
	Value string `json:"value"`
}

// ShareGPTExample is the format used by ShareGPT-style fine-tuning.
type ShareGPTExample struct {
	Conversations []ShareGPTMessage `json:"conversations"`
}

// ExportShareGPTFormat exports in ShareGPT conversation format.
func (e *TrainingExporter) ExportShareGPTFormat(minQuality string) error {
	timestamp := time.Now().Format("20060102_150405")
	path := filepath.Join(e.outputDir, fmt.Sprintf("sharegpt_%s_%s.json", minQuality, timestamp))

	qualityOrder := map[string]int{"excellent": 4, "good": 3, "poor": 2, "failed": 1}
	minQualityLevel := qualityOrder[minQuality]

	var filtered []ShareGPTExample
	for _, ex := range e.examples {
		if qualityOrder[ex.Quality] >= minQualityLevel {
			filtered = append(filtered, ShareGPTExample{
				Conversations: []ShareGPTMessage{
					{From: "human", Value: ex.Prompt},
					{From: "gpt", Value: ex.Completion},
				},
			})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(filtered)
}

// ExportDPOFormat exports paired good/bad examples for DPO training.
// This pairs each failed/poor response with an excellent/good response for the same category.
func (e *TrainingExporter) ExportDPOFormat() error {
	timestamp := time.Now().Format("20060102_150405")
	path := filepath.Join(e.outputDir, fmt.Sprintf("dpo_pairs_%s.jsonl", timestamp))

	// Group by category
	good := make(map[string][]TrainingExample)  // excellent or good
	bad := make(map[string][]TrainingExample)   // poor or failed

	for _, ex := range e.examples {
		if ex.Quality == "excellent" || ex.Quality == "good" {
			good[ex.Category] = append(good[ex.Category], ex)
		} else {
			bad[ex.Category] = append(bad[ex.Category], ex)
		}
	}

	type DPOPair struct {
		Prompt   string `json:"prompt"`
		Chosen   string `json:"chosen"`    // Good response
		Rejected string `json:"rejected"`  // Bad response
		Category string `json:"category"`
	}

	var pairs []DPOPair
	for category, badExamples := range bad {
		goodExamples := good[category]
		if len(goodExamples) == 0 {
			continue
		}

		for i, badEx := range badExamples {
			// Pair with a good example (cycling through available good examples)
			goodEx := goodExamples[i%len(goodExamples)]
			pairs = append(pairs, DPOPair{
				Prompt:   badEx.Prompt,
				Chosen:   goodEx.Completion,
				Rejected: badEx.Completion,
				Category: category,
			})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	for _, pair := range pairs {
		if err := encoder.Encode(pair); err != nil {
			return err
		}
	}

	return nil
}
