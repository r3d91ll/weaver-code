package loader

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// LMStudioLoader handles LM Studio-specific operations.
// Supports both CLI (lms) and GUI-based model management.
type LMStudioLoader struct {
	client   *http.Client
	baseURL  string
	hasCLI   bool
	cliPath  string
}

// NewLMStudioLoader creates a new LM Studio loader.
func NewLMStudioLoader(baseURL string) *LMStudioLoader {
	if baseURL == "" {
		baseURL = "http://localhost:1234"
	}

	loader := &LMStudioLoader{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		baseURL: baseURL,
	}

	// Check if lms CLI is available
	if path, err := exec.LookPath("lms"); err == nil {
		loader.hasCLI = true
		loader.cliPath = path
	}

	return loader
}

// HasCLI returns whether the lms CLI is available.
func (l *LMStudioLoader) HasCLI() bool {
	return l.hasCLI
}

// GetActiveModel returns the currently loaded model in LM Studio.
func (l *LMStudioLoader) GetActiveModel() (string, error) {
	resp, err := l.client.Get(l.baseURL + "/v1/models")
	if err != nil {
		return "", fmt.Errorf("failed to get models: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Data) > 0 {
		return result.Data[0].ID, nil
	}

	return "", nil
}

// ListModels returns all models available in LM Studio (on disk).
func (l *LMStudioLoader) ListModels() ([]string, error) {
	if !l.hasCLI {
		// Fall back to API - returns only loaded models
		resp, err := l.client.Get(l.baseURL + "/v1/models")
		if err != nil {
			return nil, fmt.Errorf("failed to get models: %w", err)
		}
		defer resp.Body.Close()

		var result struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		var models []string
		for _, m := range result.Data {
			models = append(models, m.ID)
		}
		return models, nil
	}

	// Use lms ls to get all available models
	cmd := exec.Command(l.cliPath, "ls")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("lms ls failed: %w", err)
	}

	var models []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "Available") && !strings.HasPrefix(line, "---") {
			// Parse model name from lms ls output
			// Format varies, but typically model identifier is the first column
			parts := strings.Fields(line)
			if len(parts) > 0 {
				models = append(models, parts[0])
			}
		}
	}

	return models, nil
}

// LoadModel loads a model using the lms CLI or returns instructions for GUI.
func (l *LMStudioLoader) LoadModel(name string) error {
	if !l.hasCLI {
		return fmt.Errorf("LM Studio CLI (lms) not found. Either:\n  1. Install CLI: npx lmstudio install-cli\n  2. Load '%s' manually in LM Studio GUI", name)
	}

	// Unload any currently loaded model first
	unloadCmd := exec.Command(l.cliPath, "unload", "--all")
	unloadCmd.Run() // Ignore errors - might not have anything loaded

	// Load the requested model
	// Use --gpu max to use all available GPU memory
	cmd := exec.Command(l.cliPath, "load", name, "--gpu", "max", "--yes")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to load model: %s\n%s", err, string(output))
	}

	return nil
}

// UnloadModel unloads the current model.
func (l *LMStudioLoader) UnloadModel() error {
	if !l.hasCLI {
		return fmt.Errorf("LM Studio CLI (lms) not found - cannot unload programmatically")
	}

	cmd := exec.Command(l.cliPath, "unload", "--all")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to unload model: %s\n%s", err, string(output))
	}

	return nil
}

// IsAvailable checks if LM Studio is running.
func (l *LMStudioLoader) IsAvailable() bool {
	resp, err := l.client.Get(l.baseURL + "/v1/models")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
