package loader

import (
	"fmt"
)

// Loader provides a unified interface for model loading across services.
type Loader struct {
	detector *Detector
	ollama   *OllamaLoader
	lmstudio *LMStudioLoader
}

// NewLoader creates a new model loader.
func NewLoader() *Loader {
	return &Loader{
		detector: NewDetector(),
		ollama:   NewOllamaLoader(""),
		lmstudio: NewLMStudioLoader(""),
	}
}

// JuniorConfig represents the configuration for a Junior model.
type JuniorConfig struct {
	Service     ServiceType
	ServiceName string // Human-readable name
	ModelName   string
	URL         string
	Available   bool
}

// DetectJunior detects available local model services and returns config for the first available.
// Priority: LM Studio > Ollama > vLLM > LocalAI (LM Studio has native tool support)
func (l *Loader) DetectJunior() (*JuniorConfig, []ServiceStatus) {
	statuses := l.detector.DetectAll()

	// Build a map for quick lookup
	statusMap := make(map[ServiceType]*ServiceStatus)
	for i := range statuses {
		statusMap[statuses[i].Type] = &statuses[i]
	}

	// Priority order: LM Studio first (has native tool support), then others
	priorityOrder := []ServiceType{ServiceLMStudio, ServiceOllama, ServiceVLLM, ServiceLocalAI}

	// First pass: find service with active model, respecting priority
	for _, svcType := range priorityOrder {
		status, ok := statusMap[svcType]
		if !ok || !status.Available {
			continue
		}

		if status.ActiveModel != "" {
			return &JuniorConfig{
				Service:     status.Type,
				ServiceName: status.Name,
				ModelName:   status.ActiveModel,
				URL:         l.getAPIURL(*status),
				Available:   true,
			}, statuses
		}
	}

	// Second pass: find available service with models, respecting priority
	for _, svcType := range priorityOrder {
		status, ok := statusMap[svcType]
		if !ok || !status.Available {
			continue
		}

		if len(status.Models) > 0 {
			return &JuniorConfig{
				Service:     status.Type,
				ServiceName: status.Name,
				ModelName:   "", // No model loaded
				URL:         l.getAPIURL(*status),
				Available:   true,
			}, statuses
		}
	}

	return nil, statuses
}

// getAPIURL returns the API-compatible URL for a service.
func (l *Loader) getAPIURL(status ServiceStatus) string {
	switch status.Type {
	case ServiceLMStudio, ServiceVLLM, ServiceLocalAI:
		return status.URL + "/v1"
	case ServiceOllama:
		return status.URL + "/v1" // Ollama also supports OpenAI-compatible endpoint
	default:
		return status.URL
	}
}

// LoadModel loads a model on the specified service.
func (l *Loader) LoadModel(svcType ServiceType, modelName string) error {
	switch svcType {
	case ServiceOllama:
		return l.ollama.LoadModel(modelName)
	case ServiceLMStudio:
		return l.lmstudio.LoadModel(modelName)
	default:
		return fmt.Errorf("model loading not supported for %s", svcType)
	}
}

// GetServiceStatus returns the status of a specific service.
func (l *Loader) GetServiceStatus(svcType ServiceType) *ServiceStatus {
	return l.detector.DetectService(svcType)
}

// GetAllStatuses returns the status of all known services.
func (l *Loader) GetAllStatuses() []ServiceStatus {
	return l.detector.DetectAll()
}

// FormatStatus formats a ServiceStatus for display.
func FormatStatus(status ServiceStatus) string {
	if !status.Available {
		return fmt.Sprintf("  \u2717 %s (%s) - %s", status.Name, status.URL, status.Error)
	}

	if status.ActiveModel != "" {
		return fmt.Sprintf("  \u2713 %s (%s) - Running\n    \u2514\u2500 Active model: %s",
			status.Name, status.URL, status.ActiveModel)
	}

	modelCount := len(status.Models)
	if modelCount > 0 {
		return fmt.Sprintf("  \u2713 %s (%s) - Running (%d models available, none loaded)",
			status.Name, status.URL, modelCount)
	}

	return fmt.Sprintf("  \u2713 %s (%s) - Running (no models)", status.Name, status.URL)
}
