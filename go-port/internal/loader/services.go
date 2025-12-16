// Package loader provides automatic detection and loading of local model services.
package loader

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ServiceType identifies a local model service.
type ServiceType string

const (
	ServiceLMStudio ServiceType = "lmstudio"
	ServiceOllama   ServiceType = "ollama"
	ServiceVLLM     ServiceType = "vllm"
	ServiceLocalAI  ServiceType = "localai"
)

// ServiceStatus represents the status of a local model service.
type ServiceStatus struct {
	Type        ServiceType // "lmstudio", "ollama", etc.
	Name        string      // Human-readable name
	URL         string      // Base URL
	Available   bool        // Service is running
	ActiveModel string      // Currently loaded model (if any)
	Models      []ModelInfo // Available models to load
	Error       string      // Error message if detection failed
}

// ModelInfo represents information about an available model.
type ModelInfo struct {
	Name         string // Model identifier
	Size         string // Human-readable size ("7B", "13B", etc.)
	Quantization string // "Q4_K_M", "fp16", etc.
	Loaded       bool   // Currently active/loaded
}

// ServiceConfig defines how to detect and interact with a service.
type ServiceConfig struct {
	Type         ServiceType
	Name         string
	DefaultURL   string
	HealthPath   string
	ModelsPath   string
	SupportsLoad bool // Can we load models via API?
}

// DefaultServices returns the default service configurations.
func DefaultServices() []ServiceConfig {
	return []ServiceConfig{
		{
			Type:         ServiceLMStudio,
			Name:         "LM Studio",
			DefaultURL:   "http://localhost:1234",
			HealthPath:   "/v1/models",
			ModelsPath:   "/v1/models",
			SupportsLoad: true, // LM Studio supports loading via lms CLI (if installed)
		},
		{
			Type:         ServiceOllama,
			Name:         "Ollama",
			DefaultURL:   "http://localhost:11434",
			HealthPath:   "/api/tags",
			ModelsPath:   "/api/tags",
			SupportsLoad: true,
		},
		{
			Type:         ServiceVLLM,
			Name:         "vLLM",
			DefaultURL:   "http://localhost:8000",
			HealthPath:   "/v1/models",
			ModelsPath:   "/v1/models",
			SupportsLoad: false,
		},
		{
			Type:         ServiceLocalAI,
			Name:         "LocalAI",
			DefaultURL:   "http://localhost:8080",
			HealthPath:   "/v1/models",
			ModelsPath:   "/v1/models",
			SupportsLoad: false,
		},
	}
}

// Detector handles service detection and model discovery.
type Detector struct {
	client   *http.Client
	services []ServiceConfig
}

// NewDetector creates a new service detector.
func NewDetector() *Detector {
	return &Detector{
		client: &http.Client{
			Timeout: 2 * time.Second, // Quick timeout for polling
		},
		services: DefaultServices(),
	}
}

// DetectAll polls all known services and returns their status.
func (d *Detector) DetectAll() []ServiceStatus {
	results := make([]ServiceStatus, len(d.services))

	for i, svc := range d.services {
		results[i] = d.detectService(svc)
	}

	return results
}

// DetectService checks a specific service type.
func (d *Detector) DetectService(svcType ServiceType) *ServiceStatus {
	for _, svc := range d.services {
		if svc.Type == svcType {
			status := d.detectService(svc)
			return &status
		}
	}
	return nil
}

// detectService checks if a service is available and gets its models.
func (d *Detector) detectService(svc ServiceConfig) ServiceStatus {
	status := ServiceStatus{
		Type:   svc.Type,
		Name:   svc.Name,
		URL:    svc.DefaultURL,
		Models: []ModelInfo{},
	}

	// Check health/availability
	resp, err := d.client.Get(svc.DefaultURL + svc.HealthPath)
	if err != nil {
		status.Available = false
		status.Error = "Not running"
		return status
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		status.Available = false
		status.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return status
	}

	status.Available = true

	// Parse models based on service type
	switch svc.Type {
	case ServiceLMStudio, ServiceVLLM, ServiceLocalAI:
		d.parseOpenAIModels(&status, resp)
	case ServiceOllama:
		d.parseOllamaModels(&status, resp)
	}

	return status
}

// parseOpenAIModels parses the OpenAI-compatible /v1/models response.
func (d *Detector) parseOpenAIModels(status *ServiceStatus, resp *http.Response) {
	var result struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		status.Error = "Failed to parse models"
		return
	}

	for _, m := range result.Data {
		info := ModelInfo{
			Name:   m.ID,
			Loaded: true, // OpenAI-compatible APIs typically show loaded models
		}
		status.Models = append(status.Models, info)

		// First model is typically the active one
		if status.ActiveModel == "" {
			status.ActiveModel = m.ID
		}
	}
}

// parseOllamaModels parses the Ollama /api/tags response.
func (d *Detector) parseOllamaModels(status *ServiceStatus, resp *http.Response) {
	var result struct {
		Models []struct {
			Name       string `json:"name"`
			Size       int64  `json:"size"`
			ModifiedAt string `json:"modified_at"`
			Details    struct {
				ParameterSize     string `json:"parameter_size"`
				QuantizationLevel string `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		status.Error = "Failed to parse models"
		return
	}

	for _, m := range result.Models {
		info := ModelInfo{
			Name:         m.Name,
			Size:         m.Details.ParameterSize,
			Quantization: m.Details.QuantizationLevel,
			Loaded:       false, // Ollama models need to be loaded explicitly
		}
		status.Models = append(status.Models, info)
	}

	// Ollama doesn't have a "currently loaded" concept via /api/tags
	// We'd need to check /api/ps for running models
}

// GetFirstAvailableModel returns the first service with an active model.
func (d *Detector) GetFirstAvailableModel() (*ServiceStatus, *ModelInfo) {
	statuses := d.DetectAll()

	for _, status := range statuses {
		if !status.Available {
			continue
		}

		// Check for active model
		if status.ActiveModel != "" {
			for _, m := range status.Models {
				if m.Name == status.ActiveModel {
					return &status, &m
				}
			}
			// Active model set but not in list, return basic info
			return &status, &ModelInfo{Name: status.ActiveModel, Loaded: true}
		}
	}

	// No active model found, return first available service
	for _, status := range statuses {
		if status.Available {
			return &status, nil
		}
	}

	return nil, nil
}
