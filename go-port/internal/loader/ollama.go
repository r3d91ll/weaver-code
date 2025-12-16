package loader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OllamaLoader handles Ollama-specific model loading.
type OllamaLoader struct {
	client  *http.Client
	baseURL string
}

// NewOllamaLoader creates a new Ollama loader.
func NewOllamaLoader(baseURL string) *OllamaLoader {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaLoader{
		client: &http.Client{
			Timeout: 60 * time.Second, // Model loading can take time
		},
		baseURL: baseURL,
	}
}

// LoadModel loads a model into Ollama's memory.
// Ollama loads models on first request, so we send a minimal generate request.
func (o *OllamaLoader) LoadModel(name string) error {
	reqBody := map[string]interface{}{
		"model":  name,
		"prompt": "", // Empty prompt just loads the model
		"stream": false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := o.client.Post(o.baseURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to load model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	return nil
}

// GetRunningModels checks what models are currently loaded in Ollama.
func (o *OllamaLoader) GetRunningModels() ([]string, error) {
	resp, err := o.client.Get(o.baseURL + "/api/ps")
	if err != nil {
		return nil, fmt.Errorf("failed to get running models: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	names := make([]string, len(result.Models))
	for i, m := range result.Models {
		names[i] = m.Name
	}

	return names, nil
}

// UnloadModel unloads a model from Ollama's memory.
func (o *OllamaLoader) UnloadModel(name string) error {
	reqBody := map[string]interface{}{
		"model":     name,
		"keep_alive": 0, // Setting keep_alive to 0 unloads the model
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := o.client.Post(o.baseURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to unload model: %w", err)
	}
	defer resp.Body.Close()

	return nil
}
