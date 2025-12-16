// Package orchestrator coordinates senior and junior models.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	ctxmgr "github.com/r3d91ll/weaver-code/internal/context"
	"github.com/r3d91ll/weaver-code/internal/junior"
	"github.com/r3d91ll/weaver-code/internal/memory"
	"github.com/r3d91ll/weaver-code/internal/senior"
	"github.com/r3d91ll/weaver-code/internal/tools"
)

// Weaver orchestrates communication between senior and junior models.
type Weaver struct {
	senior        senior.Adapter
	junior        *junior.Model
	seniorCtx     *ctxmgr.Manager
	juniorCtx     *ctxmgr.Manager
	memory        *memory.SharedMemory
	toolExecutor  *tools.Executor
	currentAgent  string // "senior" or "junior"
	toolsEnabled  bool
}

// Config holds Weaver configuration.
type Config struct {
	// Senior model config
	SeniorProvider senior.Provider
	SeniorPrompt   string

	// Junior model config
	JuniorURL          string
	JuniorModel        string
	JuniorContextLimit int
	JuniorPrompt       string

	// Tools config
	EnableTools    bool   // Enable Junior tools
	WorkspaceRoot  string // Root directory for file operations (defaults to cwd)
}

// DefaultConfig returns sensible defaults.
// Uses Ollama (port 11434) with gpt-oss:20b-weaver model by default.
func DefaultConfig() Config {
	cwd, _ := os.Getwd()
	defaultModel := "gpt-oss:20b-weaver"
	return Config{
		SeniorProvider:     senior.ProviderClaudeCode,
		SeniorPrompt:       SeniorEngineerPrompt,
		JuniorURL:          "http://localhost:11434/v1",
		JuniorModel:        defaultModel,
		JuniorContextLimit: 131072,
		JuniorPrompt:       GetJuniorPromptForModel(defaultModel),
		EnableTools:        true,
		WorkspaceRoot:      cwd,
	}
}

// getTemperatureForModel returns the recommended temperature for a model.
// Devstral models work best with low temperature (0.15) for deterministic code.
func getTemperatureForModel(modelName string) float64 {
	modelLower := strings.ToLower(modelName)
	if strings.Contains(modelLower, "devstral") {
		return 0.15 // Mistral's recommendation for Devstral
	}
	return 0.7 // Default for other models
}

// getMaxTokensForModel returns the recommended max tokens for a model.
// Devstral models tend to generate verbose output with full docstrings.
func getMaxTokensForModel(modelName string) int {
	modelLower := strings.ToLower(modelName)
	if strings.Contains(modelLower, "devstral") {
		return 16384 // Devstral generates verbose code with docstrings
	}
	return 4096 // Default for other models
}

// getTimeoutForModel returns the recommended HTTP timeout for a model.
// Devstral models use tool calling which adds significant generation time.
func getTimeoutForModel(modelName string) time.Duration {
	modelLower := strings.ToLower(modelName)
	if strings.Contains(modelLower, "devstral") {
		return 5 * time.Minute // Devstral needs longer for tool call generation
	}
	return 2 * time.Minute // Default for other models
}

// New creates a new Weaver orchestrator.
func New(cfg Config) (*Weaver, error) {
	mem, err := memory.NewSharedMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize shared memory: %w", err)
	}

	// Create senior adapter based on provider
	var seniorAdapter senior.Adapter
	switch cfg.SeniorProvider {
	case senior.ProviderClaudeCode:
		seniorAdapter = senior.NewClaudeCode(cfg.SeniorPrompt)
	// Future: case senior.ProviderAnthropicAPI:
	default:
		seniorAdapter = senior.NewClaudeCode(cfg.SeniorPrompt)
	}

	// Get model-specific prompt if not explicitly set
	juniorPrompt := cfg.JuniorPrompt
	if juniorPrompt == "" || juniorPrompt == JuniorEngineerPrompt {
		juniorPrompt = GetJuniorPromptForModel(cfg.JuniorModel)
	}

	// Create junior model with model-specific settings
	juniorModel := junior.New(junior.Config{
		Name:         cfg.JuniorModel,
		BaseURL:      cfg.JuniorURL,
		Model:        cfg.JuniorModel,
		SystemPrompt: juniorPrompt,
		ContextLimit: cfg.JuniorContextLimit,
		MaxTokens:    getMaxTokensForModel(cfg.JuniorModel),
		Temperature:  getTemperatureForModel(cfg.JuniorModel),
		Timeout:      getTimeoutForModel(cfg.JuniorModel),
	})

	w := &Weaver{
		senior:       seniorAdapter,
		junior:       juniorModel,
		seniorCtx:    ctxmgr.NewManager(ctxmgr.DefaultClaudeConfig()),
		juniorCtx:    ctxmgr.NewManager(ctxmgr.DefaultLocalConfig(cfg.JuniorContextLimit)),
		memory:       mem,
		toolsEnabled: cfg.EnableTools,
	}

	// Initialize tool executor if enabled
	if cfg.EnableTools {
		workspaceRoot := cfg.WorkspaceRoot
		if workspaceRoot == "" {
			workspaceRoot, _ = os.Getwd()
		}
		w.toolExecutor = tools.NewExecutor(workspaceRoot, mem)

		// Set tools on junior model
		juniorModel.SetTools(w.toolExecutor.GetToolSchemas())
	}

	return w, nil
}

// Chat sends a message and returns the response.
// All user messages go to senior. Senior may delegate to junior via /local command.
func (w *Weaver) Chat(ctx context.Context, message string) (string, error) {
	// Check if context needs compaction for senior
	if w.seniorCtx.ShouldCompact() {
		if err := w.compactSeniorContext(ctx); err != nil {
			// Log but don't fail - try to continue
			fmt.Printf("Warning: context compaction failed: %v\n", err)
		}
	}

	// Add shared memory context
	memoryContext := w.memory.FormatForPrompt(10)
	enhancedMessage := message
	if memoryContext != "" {
		enhancedMessage = memoryContext + "\n---\n\n" + message
	}

	// Add user message to context
	w.seniorCtx.Add(ctxmgr.Message{
		Role:      "user",
		Content:   message,
		Timestamp: time.Now(),
	})

	// Send to senior
	response, err := w.senior.Chat(ctx, enhancedMessage, toSeniorMessages(w.seniorCtx.Messages()))
	if err != nil {
		return "", fmt.Errorf("senior error: %w", err)
	}

	// Add response to context
	w.seniorCtx.Add(ctxmgr.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	})
	w.currentAgent = "senior"

	// Handle delegation loop - Senior may delegate multiple times
	// Each review may contain another /local, so we loop until no more delegations
	var result strings.Builder
	result.WriteString(response)

	const maxDelegations = 50 // Safety limit to prevent infinite loops
	for i := 0; i < maxDelegations; i++ {
		task := extractLocalTask(response)
		if task == "" {
			break // No more delegations
		}

		// Delegate to Junior
		juniorResponse, err := w.delegateToJunior(ctx, task)
		if err != nil {
			result.WriteString("\n\n[Junior unavailable: " + err.Error() + "]")
			break
		}

		result.WriteString("\n\n---\n**Junior:**\n" + juniorResponse)

		// Send Junior's response back to senior for review
		review, err := w.getSeniorReview(ctx, juniorResponse)
		if err != nil {
			break
		}

		result.WriteString("\n\n---\n**Senior (review):**\n" + review)

		// Check if review contains another delegation
		response = review
	}

	return result.String(), nil
}

// ChatStream sends a message and streams the response (legacy string-based).
func (w *Weaver) ChatStream(ctx context.Context, message string) (<-chan string, <-chan error) {
	chunks := make(chan string, 100)
	errs := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errs)

		outChunks, outErrs := w.ChatStreamStructured(ctx, message)
		for chunk := range outChunks {
			chunks <- chunk.Content
		}
		if err := <-outErrs; err != nil {
			errs <- err
		}
	}()

	return chunks, errs
}

// ChatStreamStructured sends a message and streams structured output with agent attribution.
func (w *Weaver) ChatStreamStructured(ctx context.Context, message string) (<-chan OutputChunk, <-chan error) {
	chunks := make(chan OutputChunk, 100)
	errs := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errs)

		// Check for compaction
		if w.seniorCtx.ShouldCompact() {
			w.compactSeniorContext(ctx)
		}

		// Add memory context
		memoryContext := w.memory.FormatForPrompt(10)
		enhancedMessage := message
		if memoryContext != "" {
			enhancedMessage = memoryContext + "\n---\n\n" + message
		}

		w.seniorCtx.Add(ctxmgr.Message{
			Role:      "user",
			Content:   message,
			Timestamp: time.Now(),
		})

		seniorChunks, seniorErrs := w.senior.ChatStream(ctx, enhancedMessage, toSeniorMessages(w.seniorCtx.Messages()))

		var fullResponse strings.Builder

		// Forward senior chunks
		for chunk := range seniorChunks {
			fullResponse.WriteString(chunk)
			chunks <- OutputChunk{Agent: "senior", Content: chunk}
		}

		// Check for errors
		if err := <-seniorErrs; err != nil {
			errs <- err
			return
		}

		response := fullResponse.String()
		w.seniorCtx.Add(ctxmgr.Message{
			Role:      "assistant",
			Content:   response,
			Timestamp: time.Now(),
		})
		w.currentAgent = "senior"

		// Signal senior is done (before potential delegation)
		chunks <- OutputChunk{Agent: "senior", Done: true}

		// Handle delegation loop - Senior may delegate multiple times
		// Each review may contain another /local, so we loop until no more delegations
		const maxDelegations = 50 // Safety limit to prevent infinite loops
		for i := 0; i < maxDelegations; i++ {
			task := extractLocalTask(response)
			if task == "" {
				break // No more delegations
			}

			juniorResponse, err := w.delegateToJunior(ctx, task)
			if err != nil {
				chunks <- OutputChunk{Agent: "system", Content: "[Junior unavailable: " + err.Error() + "]"}
				return
			}

			// Send junior's response
			chunks <- OutputChunk{Agent: "junior", Content: juniorResponse}
			chunks <- OutputChunk{Agent: "junior", Done: true}

			// Get senior's review
			review, err := w.getSeniorReview(ctx, juniorResponse)
			if err != nil {
				// If review fails, we've already shown junior's work
				return
			}

			// Send senior's review
			chunks <- OutputChunk{Agent: "senior", Content: review}
			chunks <- OutputChunk{Agent: "senior", Done: true}

			// Check if review contains another delegation
			response = review
		}
	}()

	return chunks, errs
}

// delegateToJunior sends a task to the local model.
// If tools are enabled, it handles the tool execution loop.
func (w *Weaver) delegateToJunior(ctx context.Context, task string) (string, error) {
	if !w.junior.IsAvailable() {
		return "", fmt.Errorf("junior model not available")
	}

	// Check/truncate junior context
	if w.juniorCtx.ShouldTruncate() {
		w.juniorCtx.Truncate()
	} else if w.juniorCtx.ShouldCompact() {
		w.compactJuniorContext(ctx)
	}

	// Add shared context info
	sharedContext := w.memory.FormatForPrompt(5)
	enhancedTask := task
	if sharedContext != "" {
		enhancedTask = "=== Shared Context ===\n" + sharedContext + "\n---\n\nTask from Senior Engineer: " + task
	} else {
		enhancedTask = "Task from Senior Engineer: " + task
	}

	w.juniorCtx.Add(ctxmgr.Message{
		Role:      "user",
		Content:   task,
		Timestamp: time.Now(),
	})

	// If tools are not enabled, use simple chat
	if !w.toolsEnabled || w.toolExecutor == nil {
		response, err := w.junior.Chat(ctx, enhancedTask, toJuniorMessages(w.juniorCtx.Messages()))
		if err != nil {
			return "", err
		}
		w.juniorCtx.Add(ctxmgr.Message{
			Role:      "assistant",
			Content:   response,
			Timestamp: time.Now(),
		})
		w.currentAgent = "junior"
		return response, nil
	}

	// Tool-enabled chat with execution loop
	return w.delegateWithTools(ctx, enhancedTask)
}

// delegateWithTools handles the tool execution loop for Junior.
func (w *Weaver) delegateWithTools(ctx context.Context, task string) (string, error) {
	// Build initial messages
	messages := w.junior.BuildMessages(task, toJuniorMessages(w.juniorCtx.Messages()))

	const maxToolIterations = 10
	var finalResponse strings.Builder

	for i := 0; i < maxToolIterations; i++ {
		// Send to Junior
		resp, err := w.junior.ChatWithTools(ctx, messages)
		if err != nil {
			return finalResponse.String(), err
		}

		// Accumulate any content
		if resp.Content != "" {
			finalResponse.WriteString(resp.Content)
		}

		// If no tool calls, we're done
		if len(resp.ToolCalls) == 0 {
			break
		}

		// Add assistant message with tool calls to conversation
		messages = append(messages, junior.CreateAssistantMessage(resp.Content, resp.ToolCalls))

		// Execute each tool call
		for _, tc := range resp.ToolCalls {
			result, err := w.toolExecutor.Execute(ctx, tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
			}

			// Add tool result to messages
			messages = append(messages, junior.CreateToolResultMessage(tc.ID, result))
		}
	}

	response := finalResponse.String()
	w.juniorCtx.Add(ctxmgr.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	})
	w.currentAgent = "junior"

	return response, nil
}

// getSeniorReview sends Junior's response to senior for review.
func (w *Weaver) getSeniorReview(ctx context.Context, juniorResponse string) (string, error) {
	reviewPrompt := fmt.Sprintf(`Junior Engineer completed the delegated task.

## Junior's Response:
%s

## Your Task:
Review this work and provide the final response to the user. If changes are needed, either make them yourself or delegate again with /local.`, juniorResponse)

	w.seniorCtx.Add(ctxmgr.Message{
		Role:      "user",
		Content:   reviewPrompt,
		Timestamp: time.Now(),
	})

	response, err := w.senior.Chat(ctx, reviewPrompt, toSeniorMessages(w.seniorCtx.Messages()))
	if err != nil {
		return "", err
	}

	w.seniorCtx.Add(ctxmgr.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	})

	return response, nil
}

// compactSeniorContext asks the senior model to summarize and resets context.
func (w *Weaver) compactSeniorContext(ctx context.Context) error {
	prompt := w.seniorCtx.CompactionPrompt()
	summary, err := w.senior.Chat(ctx, prompt, toSeniorMessages(w.seniorCtx.Messages()))
	if err != nil {
		return err
	}
	w.seniorCtx.ResetWithSummary(summary)
	return nil
}

// compactJuniorContext asks the junior model to summarize and resets context.
func (w *Weaver) compactJuniorContext(ctx context.Context) error {
	prompt := w.juniorCtx.CompactionPrompt()
	summary, err := w.junior.Chat(ctx, prompt, toJuniorMessages(w.juniorCtx.Messages()))
	if err != nil {
		return err
	}
	w.juniorCtx.ResetWithSummary(summary)
	return nil
}

// ListAgents returns status of all models.
func (w *Weaver) ListAgents() []AgentStatus {
	return []AgentStatus{
		{
			Role:      "senior",
			Provider:  string(w.senior.Provider()),
			Name:      w.senior.Name(),
			Available: w.senior.IsAvailable(),
		},
		{
			Role:      "junior",
			Provider:  "local",
			Name:      w.junior.Name(),
			Available: w.junior.IsAvailable(),
		},
	}
}

// AgentStatus represents a model's current status.
type AgentStatus struct {
	Role      string // "senior" or "junior"
	Provider  string // "claude_code", "anthropic_api", "local"
	Name      string
	Available bool
}

// OutputChunk represents a piece of output with agent attribution.
type OutputChunk struct {
	Agent   string // "senior", "junior", or "system"
	Content string
	Done    bool // true if this agent is done speaking
}

// Memory returns the shared memory instance.
func (w *Weaver) Memory() *memory.SharedMemory {
	return w.memory
}

// CurrentAgent returns which model handled the last message.
func (w *Weaver) CurrentAgent() string {
	return w.currentAgent
}

// ClearContext clears conversation history for both models.
func (w *Weaver) ClearContext() {
	w.seniorCtx.Clear()
	w.juniorCtx.Clear()
}

// ClearSeniorContext clears only Senior's conversation history.
func (w *Weaver) ClearSeniorContext() {
	w.seniorCtx.Clear()
}

// ClearJuniorContext clears only Junior's conversation history.
func (w *Weaver) ClearJuniorContext() {
	w.juniorCtx.Clear()
}

// ChatJuniorDirect sends a message directly to Junior (bypassing Senior).
// Used when user explicitly requests /local.
func (w *Weaver) ChatJuniorDirect(ctx context.Context, message string) (string, error) {
	if !w.junior.IsAvailable() {
		return "", fmt.Errorf("junior model not available")
	}

	// Check/truncate junior context
	if w.juniorCtx.ShouldTruncate() {
		w.juniorCtx.Truncate()
	} else if w.juniorCtx.ShouldCompact() {
		w.compactJuniorContext(ctx)
	}

	// Add memory context
	memoryContext := w.memory.FormatForPrompt(5)
	enhancedMessage := message
	if memoryContext != "" {
		enhancedMessage = memoryContext + "\n---\n\n" + message
	}

	w.juniorCtx.Add(ctxmgr.Message{
		Role:      "user",
		Content:   message,
		Timestamp: time.Now(),
	})

	response, err := w.junior.Chat(ctx, enhancedMessage, toJuniorMessages(w.juniorCtx.Messages()))
	if err != nil {
		return "", err
	}

	w.juniorCtx.Add(ctxmgr.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	})
	w.currentAgent = "junior"

	return response, nil
}

// SeniorProvider returns the current senior model provider.
func (w *Weaver) SeniorProvider() senior.Provider {
	return w.senior.Provider()
}

// JuniorModel returns the junior model name.
func (w *Weaver) JuniorModel() string {
	return w.junior.Name()
}

// JuniorURL returns the junior model endpoint URL.
func (w *Weaver) JuniorURL() string {
	return w.junior.BaseURL()
}

// JuniorIsAvailable checks if Junior is available.
func (w *Weaver) JuniorIsAvailable() bool {
	return w.junior.IsAvailable()
}

// UpdateJuniorPrompt updates Junior's system prompt.
// This is called after assessment to inject JUNIOR.md content.
func (w *Weaver) UpdateJuniorPrompt(prompt string) {
	w.junior.SetSystemPrompt(prompt)
}

// UpdateJuniorModel switches to a different Junior model.
// Called by /load command to update the active model.
func (w *Weaver) UpdateJuniorModel(url, model string, contextLimit int) {
	// Use model-specific prompt if available
	systemPrompt := w.junior.SystemPrompt()
	if modelPrompt := GetJuniorPromptForModel(model); modelPrompt != JuniorEngineerPrompt {
		systemPrompt = modelPrompt
	}

	w.junior = junior.New(junior.Config{
		Name:         model,
		BaseURL:      url,
		Model:        model,
		SystemPrompt: systemPrompt,
		ContextLimit: contextLimit,
		MaxTokens:    getMaxTokensForModel(model),
		Temperature:  getTemperatureForModel(model),
		Timeout:      getTimeoutForModel(model),
	})
	// Re-enable tools on new model
	if w.toolsEnabled && w.toolExecutor != nil {
		w.junior.SetTools(w.toolExecutor.GetToolSchemas())
	}
}

// ChatJuniorRaw sends a message directly to Junior without context management.
// Used for assessment to avoid polluting conversation history.
func (w *Weaver) ChatJuniorRaw(ctx context.Context, message string) (string, error) {
	return w.junior.Chat(ctx, message, nil)
}

// ChatJuniorWithToolsRaw runs a tool-enabled chat with Junior for assessment.
// Executes tools and returns the final response including tool execution log.
// The tool execution log is appended so evaluators can see evidence of tool usage.
func (w *Weaver) ChatJuniorWithToolsRaw(ctx context.Context, message string) (string, error) {
	if !w.toolsEnabled || w.toolExecutor == nil {
		return w.junior.Chat(ctx, message, nil)
	}

	// Build initial messages
	messages := w.junior.BuildMessages(message, nil)

	const maxToolIterations = 10
	var finalResponse strings.Builder
	var codeFromTools strings.Builder // Track code from tool calls
	var toolLog strings.Builder       // Track tool execution for evaluator visibility

	for i := 0; i < maxToolIterations; i++ {
		// Send to Junior
		resp, err := w.junior.ChatWithTools(ctx, messages)
		if err != nil {
			// Return what we have so far with tool log
			return buildToolResponse(finalResponse.String(), codeFromTools.String(), toolLog.String()), err
		}

		// Accumulate any content
		if resp.Content != "" {
			finalResponse.WriteString(resp.Content)
		}

		// If no tool calls, we're done
		if len(resp.ToolCalls) == 0 {
			break
		}

		// Add assistant message with tool calls to conversation
		messages = append(messages, junior.CreateAssistantMessage(resp.Content, resp.ToolCalls))

		// Execute each tool call and log for evaluator visibility
		for _, tc := range resp.ToolCalls {
			// Log the tool invocation
			toolLog.WriteString(fmt.Sprintf("\n[TOOL CALL] %s\n", tc.Function.Name))
			toolLog.WriteString(fmt.Sprintf("Arguments: %s\n", tc.Function.Arguments))

			// Extract code from write_file tool calls for assessment
			if tc.Function.Name == "write_file" {
				code := extractCodeFromWriteFile(tc.Function.Arguments)
				if code != "" {
					if codeFromTools.Len() > 0 {
						codeFromTools.WriteString("\n\n")
					}
					codeFromTools.WriteString(code)
				}
			}

			result, err := w.toolExecutor.Execute(ctx, tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
			}

			// Log the result
			toolLog.WriteString(fmt.Sprintf("Result: %s\n", truncateResult(result, 500)))

			// Add tool result to messages
			messages = append(messages, junior.CreateToolResultMessage(tc.ID, result))
		}
	}

	return buildToolResponse(finalResponse.String(), codeFromTools.String(), toolLog.String()), nil
}

// buildToolResponse combines response content with tool execution log for evaluator.
func buildToolResponse(content, codeFromTools, toolLog string) string {
	var result strings.Builder

	// Primary content (what Junior said)
	if content != "" {
		result.WriteString(content)
	} else if codeFromTools != "" {
		// Fall back to code extracted from write_file calls
		result.WriteString(codeFromTools)
	}

	// Append tool execution log if any tools were called
	if toolLog != "" {
		if result.Len() > 0 {
			result.WriteString("\n\n")
		}
		result.WriteString("--- TOOL EXECUTION LOG ---")
		result.WriteString(toolLog)
		result.WriteString("\n--- END TOOL LOG ---")
	}

	return result.String()
}

// truncateResult truncates a tool result to maxLen for logging.
func truncateResult(result string, maxLen int) string {
	if len(result) <= maxLen {
		return result
	}
	return result[:maxLen] + "... (truncated)"
}

// extractCodeFromWriteFile extracts code content from write_file tool arguments.
func extractCodeFromWriteFile(argsJSON string) string {
	// Simple extraction - look for "content": "..."
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	if content, ok := args["content"].(string); ok {
		return content
	}
	return ""
}

// ToolsEnabled returns whether Junior tools are enabled.
func (w *Weaver) ToolsEnabled() bool {
	return w.toolsEnabled && w.toolExecutor != nil
}

// ChatSeniorRaw sends a message directly to Senior without context management.
// Used for assessment evaluation.
func (w *Weaver) ChatSeniorRaw(ctx context.Context, message string) (string, error) {
	return w.senior.Chat(ctx, message, nil)
}

// extractLocalTask finds /local commands in a response.
// Captures everything after /local until end of message.
// Senior should put /local at the end of their response when delegating.
var localPattern = regexp.MustCompile(`(?is)/local\s+(.+)$`)

func extractLocalTask(response string) string {
	matches := localPattern.FindStringSubmatch(response)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// toSeniorMessages converts context messages to senior format.
func toSeniorMessages(msgs []ctxmgr.Message) []senior.Message {
	result := make([]senior.Message, len(msgs))
	for i, m := range msgs {
		result[i] = senior.Message{
			Role:      m.Role,
			Content:   m.Content,
			Timestamp: m.Timestamp,
		}
	}
	return result
}

// toJuniorMessages converts context messages to junior format.
func toJuniorMessages(msgs []ctxmgr.Message) []junior.Message {
	result := make([]junior.Message, len(msgs))
	for i, m := range msgs {
		result[i] = junior.Message{
			Role:    m.Role,
			Content: m.Content,
		}
	}
	return result
}
