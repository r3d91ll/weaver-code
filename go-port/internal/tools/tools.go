// Package tools provides tool definitions and execution for the Junior model.
// These tools allow Junior to interact with the filesystem, execute commands,
// and communicate with Senior through shared memory.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/r3d91ll/weaver-code/internal/memory"
)

// Tool represents an available tool that Junior can use.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]ParameterDef
	Execute     func(ctx context.Context, args map[string]interface{}) (string, error)
}

// ParameterDef describes a tool parameter.
type ParameterDef struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ToolCall represents a tool invocation from the model.
type ToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function ToolCallFunc    `json:"function"`
}

// ToolCallFunc contains the function details.
type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolResult represents the result of executing a tool.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// Executor manages tool execution with safety boundaries.
type Executor struct {
	workspaceRoot string
	memory        *memory.SharedMemory
	tools         map[string]*Tool

	// Safety limits
	maxFileSize    int64
	commandTimeout time.Duration
	allowedCommands []string
}

// NewExecutor creates a new tool executor with safety boundaries.
func NewExecutor(workspaceRoot string, mem *memory.SharedMemory) *Executor {
	e := &Executor{
		workspaceRoot:   workspaceRoot,
		memory:          mem,
		maxFileSize:     1024 * 1024, // 1MB
		commandTimeout:  30 * time.Second,
		allowedCommands: []string{
			"ls", "cat", "head", "tail", "grep", "find", "wc",
			"go", "python", "python3", "node", "npm", "cargo", "make",
			"git", "pwd", "echo", "date", "which", "file", "diff",
		},
	}
	e.initTools()
	return e
}

// initTools sets up all available tools.
func (e *Executor) initTools() {
	e.tools = map[string]*Tool{
		"read_file": {
			Name:        "read_file",
			Description: "Read the contents of a file from the workspace",
			Parameters: map[string]ParameterDef{
				"path": {Type: "string", Description: "Relative path to the file", Required: true},
			},
			Execute: e.readFile,
		},
		"write_file": {
			Name:        "write_file",
			Description: "Write content to a file in the workspace. Creates parent directories if needed.",
			Parameters: map[string]ParameterDef{
				"path":    {Type: "string", Description: "Relative path to the file", Required: true},
				"content": {Type: "string", Description: "Content to write", Required: true},
			},
			Execute: e.writeFile,
		},
		"list_directory": {
			Name:        "list_directory",
			Description: "List files and directories in a path",
			Parameters: map[string]ParameterDef{
				"path": {Type: "string", Description: "Relative path to list (use '.' for current directory)", Required: true},
			},
			Execute: e.listDirectory,
		},
		"execute_command": {
			Name:        "execute_command",
			Description: "Execute a shell command in the workspace. Only allowed commands are permitted.",
			Parameters: map[string]ParameterDef{
				"command": {Type: "string", Description: "Command to execute (e.g., 'go build', 'python script.py')", Required: true},
			},
			Execute: e.executeCommand,
		},
		"search_files": {
			Name:        "search_files",
			Description: "Search for a pattern in files using grep",
			Parameters: map[string]ParameterDef{
				"pattern": {Type: "string", Description: "Pattern to search for", Required: true},
				"path":    {Type: "string", Description: "Path to search in (defaults to '.')", Required: false},
			},
			Execute: e.searchFiles,
		},
		"context_write": {
			Name:        "context_write",
			Description: "Write to shared context - a space visible to both you and Senior. Use this to share findings, ask questions, report status, or leave notes for Senior to review.",
			Parameters: map[string]ParameterDef{
				"content": {Type: "string", Description: "Content to add to shared context", Required: true},
				"tags":    {Type: "string", Description: "Comma-separated tags for organization (optional)", Required: false},
			},
			Execute: e.contextWrite,
		},
		"context_read": {
			Name:        "context_read",
			Description: "Read from shared context. Returns recent entries that you or Senior have written. Use this to see what Senior has communicated or to review your own notes.",
			Parameters: map[string]ParameterDef{
				"limit": {Type: "integer", Description: "Maximum entries to return (default 10)", Required: false},
				"tag":   {Type: "string", Description: "Filter by tag (optional)", Required: false},
			},
			Execute: e.contextRead,
		},
	}
}

// GetTools returns all available tools.
func (e *Executor) GetTools() map[string]*Tool {
	return e.tools
}

// GetToolSchemas returns OpenAI-compatible tool schemas.
func (e *Executor) GetToolSchemas() []map[string]interface{} {
	schemas := make([]map[string]interface{}, 0, len(e.tools))

	for _, tool := range e.tools {
		properties := make(map[string]interface{})
		required := []string{}

		for name, param := range tool.Parameters {
			properties[name] = map[string]interface{}{
				"type":        param.Type,
				"description": param.Description,
			}
			if param.Required {
				required = append(required, name)
			}
		}

		schema := map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters": map[string]interface{}{
					"type":       "object",
					"properties": properties,
					"required":   required,
				},
			},
		}
		schemas = append(schemas, schema)
	}

	return schemas
}

// Execute runs a tool by name with the given arguments.
func (e *Executor) Execute(ctx context.Context, name string, argsJSON string) (string, error) {
	tool, ok := e.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}

	var args map[string]interface{}
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("failed to parse arguments: %w", err)
		}
	} else {
		args = make(map[string]interface{})
	}

	return tool.Execute(ctx, args)
}

// safePath validates and resolves a path within the workspace.
func (e *Executor) safePath(relPath string) (string, error) {
	// Clean the path
	relPath = filepath.Clean(relPath)

	// Prevent absolute paths
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("absolute paths not allowed: %s", relPath)
	}

	// Resolve to absolute path
	absPath := filepath.Join(e.workspaceRoot, relPath)

	// Ensure it's within workspace (prevent directory traversal)
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", err
	}

	workspaceAbs, err := filepath.Abs(e.workspaceRoot)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(absPath, workspaceAbs) {
		return "", fmt.Errorf("path escapes workspace: %s", relPath)
	}

	return absPath, nil
}

// readFile reads a file from the workspace.
func (e *Executor) readFile(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required")
	}

	safePath, err := e.safePath(path)
	if err != nil {
		return "", err
	}

	// Check file size
	info, err := os.Stat(safePath)
	if err != nil {
		return "", fmt.Errorf("cannot access file: %w", err)
	}

	if info.Size() > e.maxFileSize {
		return "", fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), e.maxFileSize)
	}

	content, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}

// writeFile writes content to a file in the workspace.
func (e *Executor) writeFile(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required")
	}

	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}

	safePath, err := e.safePath(path)
	if err != nil {
		return "", err
	}

	// Create parent directories
	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directories: %w", err)
	}

	// Write file
	if err := os.WriteFile(safePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// listDirectory lists files in a directory.
func (e *Executor) listDirectory(ctx context.Context, args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	safePath, err := e.safePath(path)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(safePath)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	var sb strings.Builder
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		typeChar := "-"
		if entry.IsDir() {
			typeChar = "d"
		}

		sb.WriteString(fmt.Sprintf("%s %8d %s %s\n",
			typeChar,
			info.Size(),
			info.ModTime().Format("Jan 02 15:04"),
			entry.Name(),
		))
	}

	return sb.String(), nil
}

// executeCommand runs a shell command.
func (e *Executor) executeCommand(ctx context.Context, args map[string]interface{}) (string, error) {
	command, ok := args["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("command is required")
	}

	// Parse command to check if it's allowed
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	baseCmd := filepath.Base(parts[0])
	allowed := false
	for _, cmd := range e.allowedCommands {
		if baseCmd == cmd {
			allowed = true
			break
		}
	}

	if !allowed {
		return "", fmt.Errorf("command not allowed: %s (allowed: %v)", baseCmd, e.allowedCommands)
	}

	// Create command with timeout
	ctx, cancel := context.WithTimeout(ctx, e.commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = e.workspaceRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after %v", e.commandTimeout)
		}
		return string(output), fmt.Errorf("command failed: %w\nOutput: %s", err, output)
	}

	return string(output), nil
}

// searchFiles searches for a pattern in files.
func (e *Executor) searchFiles(ctx context.Context, args map[string]interface{}) (string, error) {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	safePath, err := e.safePath(path)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, e.commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "grep", "-rn", "--include=*.go", "--include=*.py", "--include=*.js", "--include=*.ts", "--include=*.md", "--include=*.yaml", "--include=*.json", pattern, safePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// grep returns exit code 1 if no matches found
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "No matches found", nil
		}
		return string(output), fmt.Errorf("search failed: %w", err)
	}

	// Truncate if too long
	result := string(output)
	if len(result) > 10000 {
		result = result[:10000] + "\n... (truncated)"
	}

	return result, nil
}

// contextWrite writes to shared context.
func (e *Executor) contextWrite(ctx context.Context, args map[string]interface{}) (string, error) {
	content, ok := args["content"].(string)
	if !ok || content == "" {
		return "", fmt.Errorf("content is required")
	}

	var tags []string
	if tagsStr, ok := args["tags"].(string); ok && tagsStr != "" {
		for _, tag := range strings.Split(tagsStr, ",") {
			tags = append(tags, strings.TrimSpace(tag))
		}
	}

	id := e.memory.Write("junior", content, tags)
	return fmt.Sprintf("Added to shared context (ID: %s). Senior will see this.", id), nil
}

// contextRead reads from shared context.
func (e *Executor) contextRead(ctx context.Context, args map[string]interface{}) (string, error) {
	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	var filterTags []string
	if tag, ok := args["tag"].(string); ok && tag != "" {
		filterTags = []string{tag}
	}

	notes := e.memory.List(limit, "", filterTags)

	if len(notes) == 0 {
		return "Shared context is empty", nil
	}

	var sb strings.Builder
	sb.WriteString("=== Shared Context ===\n\n")

	for _, note := range notes {
		author := "Senior"
		if note.Author == "junior" {
			author = "You (Junior)"
		}
		sb.WriteString(fmt.Sprintf("--- [%s] %s at %s ---\n",
			note.ID,
			author,
			note.CreatedAt.Format("2006-01-02 15:04:05"),
		))
		if len(note.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("Tags: %v\n", note.Tags))
		}
		sb.WriteString(note.Content)
		sb.WriteString("\n\n")
	}

	return sb.String(), nil
}
