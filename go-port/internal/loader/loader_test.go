package loader

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetector_DetectAll(t *testing.T) {
	d := NewDetector()

	// This test doesn't mock servers, so it tests against real local services
	// In a real environment, this will fail gracefully when services aren't running
	statuses := d.DetectAll()

	if len(statuses) != 4 {
		t.Errorf("expected 4 service statuses, got %d", len(statuses))
	}

	// Verify all expected service types are present
	types := make(map[ServiceType]bool)
	for _, s := range statuses {
		types[s.Type] = true
	}

	expectedTypes := []ServiceType{ServiceLMStudio, ServiceOllama, ServiceVLLM, ServiceLocalAI}
	for _, et := range expectedTypes {
		if !types[et] {
			t.Errorf("missing service type: %s", et)
		}
	}
}

func TestDetector_parseOpenAIModels(t *testing.T) {
	// Create mock server returning OpenAI-style model list
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "gpt-oss-20b-fp16", "object": "model"},
				{"id": "mistral-7b", "object": "model"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	d := NewDetector()
	// Override the service config to use our test server
	d.services = []ServiceConfig{
		{
			Type:       ServiceLMStudio,
			Name:       "Test LM Studio",
			DefaultURL: server.URL,
			HealthPath: "/v1/models",
			ModelsPath: "/v1/models",
		},
	}

	statuses := d.DetectAll()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}

	status := statuses[0]
	if !status.Available {
		t.Error("expected service to be available")
	}

	if len(status.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(status.Models))
	}

	if status.ActiveModel != "gpt-oss-20b-fp16" {
		t.Errorf("expected active model 'gpt-oss-20b-fp16', got '%s'", status.ActiveModel)
	}
}

func TestDetector_parseOllamaModels(t *testing.T) {
	// Create mock server returning Ollama-style model list
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"models": []map[string]interface{}{
				{
					"name": "llama3:8b",
					"size": 4661224676,
					"details": map[string]interface{}{
						"parameter_size":     "8B",
						"quantization_level": "Q4_K_M",
					},
				},
				{
					"name": "qwen2.5-coder:7b",
					"size": 3800000000,
					"details": map[string]interface{}{
						"parameter_size":     "7B",
						"quantization_level": "Q4_0",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	d := NewDetector()
	d.services = []ServiceConfig{
		{
			Type:       ServiceOllama,
			Name:       "Test Ollama",
			DefaultURL: server.URL,
			HealthPath: "/api/tags",
			ModelsPath: "/api/tags",
		},
	}

	statuses := d.DetectAll()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}

	status := statuses[0]
	if !status.Available {
		t.Error("expected service to be available")
	}

	if len(status.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(status.Models))
	}

	// Check model details
	model := status.Models[0]
	if model.Name != "llama3:8b" {
		t.Errorf("expected model name 'llama3:8b', got '%s'", model.Name)
	}
	if model.Size != "8B" {
		t.Errorf("expected size '8B', got '%s'", model.Size)
	}
	if model.Quantization != "Q4_K_M" {
		t.Errorf("expected quantization 'Q4_K_M', got '%s'", model.Quantization)
	}
}

func TestDetector_unavailableService(t *testing.T) {
	d := NewDetector()
	d.services = []ServiceConfig{
		{
			Type:       ServiceLMStudio,
			Name:       "Test LM Studio",
			DefaultURL: "http://localhost:99999", // Invalid port
			HealthPath: "/v1/models",
			ModelsPath: "/v1/models",
		},
	}

	statuses := d.DetectAll()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}

	status := statuses[0]
	if status.Available {
		t.Error("expected service to be unavailable")
	}

	if status.Error == "" {
		t.Error("expected error message for unavailable service")
	}
}

func TestLoader_DetectJunior(t *testing.T) {
	// Create mock LM Studio server with active model
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "test-model", "object": "model"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	l := NewLoader()
	// Override detector services
	l.detector.services = []ServiceConfig{
		{
			Type:       ServiceLMStudio,
			Name:       "Test LM Studio",
			DefaultURL: server.URL,
			HealthPath: "/v1/models",
			ModelsPath: "/v1/models",
		},
	}

	cfg, statuses := l.DetectJunior()

	if len(statuses) != 1 {
		t.Errorf("expected 1 status, got %d", len(statuses))
	}

	if cfg == nil {
		t.Fatal("expected JuniorConfig, got nil")
	}

	if cfg.ModelName != "test-model" {
		t.Errorf("expected model 'test-model', got '%s'", cfg.ModelName)
	}

	if cfg.ServiceName != "Test LM Studio" {
		t.Errorf("expected service 'Test LM Studio', got '%s'", cfg.ServiceName)
	}
}

func TestFormatStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   ServiceStatus
		contains string
	}{
		{
			name: "unavailable service",
			status: ServiceStatus{
				Name:      "LM Studio",
				URL:       "http://localhost:1234",
				Available: false,
				Error:     "Not running",
			},
			contains: "Not running",
		},
		{
			name: "available with active model",
			status: ServiceStatus{
				Name:        "LM Studio",
				URL:         "http://localhost:1234",
				Available:   true,
				ActiveModel: "gpt-oss-20b",
			},
			contains: "Active model: gpt-oss-20b",
		},
		{
			name: "available with models but none loaded",
			status: ServiceStatus{
				Name:      "Ollama",
				URL:       "http://localhost:11434",
				Available: true,
				Models:    []ModelInfo{{Name: "llama3:8b"}},
			},
			contains: "1 models available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatStatus(tt.status)
			if !containsString(result, tt.contains) {
				t.Errorf("FormatStatus() = %q, want to contain %q", result, tt.contains)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
