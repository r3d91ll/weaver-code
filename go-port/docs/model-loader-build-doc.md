# Model Loader Feature - Build Document

## Overview

Add automatic Junior model detection and loading on startup. The CLI should discover available local model services (LM Studio, Ollama), inventory available models, detect if a model is currently running, and provide a menu to load/switch models.

## Purpose

- Zero-config startup when a model is already running
- Easy model switching without leaving the CLI
- Support multiple local inference backends
- No manual URL/model configuration needed

## Startup Flow

```
$ ./weaver

Detecting local model services...
  ✓ LM Studio (localhost:1234) - Running
    └─ Active model: gpt-oss-20b-fp16
  ✓ Ollama (localhost:11434) - Running
    └─ No model loaded
  ✗ vLLM (localhost:8000) - Not detected

Using Junior: gpt-oss-20b-fp16 via LM Studio

╔═══════════════════════════════════════════╗
║       Weaver Code - Go Edition            ║
║  Senior: Claude Code                      ║
║  Junior: gpt-oss-20b-fp16 (LM Studio)     ║
╚═══════════════════════════════════════════╝
```

## Service Detection

### Supported Services

| Service | Default URL | Health Check | Models Endpoint |
|---------|-------------|--------------|-----------------|
| LM Studio | localhost:1234 | GET /v1/models | GET /v1/models |
| Ollama | localhost:11434 | GET /api/tags | GET /api/tags |
| vLLM | localhost:8000 | GET /v1/models | GET /v1/models |
| LocalAI | localhost:8080 | GET /v1/models | GET /v1/models |

### Detection Logic

```go
type ServiceStatus struct {
    Name        string   // "lmstudio", "ollama", etc.
    URL         string   // Base URL
    Available   bool     // Service is running
    ActiveModel string   // Currently loaded model (if any)
    Models      []string // Available models to load
}

func DetectServices() []ServiceStatus {
    services := []ServiceStatus{
        {Name: "lmstudio", URL: "http://localhost:1234"},
        {Name: "ollama", URL: "http://localhost:11434"},
        {Name: "vllm", URL: "http://localhost:8000"},
    }

    for i := range services {
        services[i].Available = checkHealth(services[i])
        if services[i].Available {
            services[i].Models = listModels(services[i])
            services[i].ActiveModel = getActiveModel(services[i])
        }
    }
    return services
}
```

## Model Loading Commands

### `/models` - List Available Models

```
You> /models

Local Model Services:

LM Studio (localhost:1234) ✓ Running
  [ACTIVE] gpt-oss-20b-fp16
  [ ] mistral-7b-instruct
  [ ] codellama-13b

Ollama (localhost:11434) ✓ Running
  [ ] llama3:8b
  [ ] qwen2.5-coder:7b
  [ ] deepseek-coder:6.7b

Use /load <service> <model> to switch models
Example: /load ollama llama3:8b
```

### `/load <service> <model>` - Load a Model

```
You> /load ollama qwen2.5-coder:7b

Loading qwen2.5-coder:7b via Ollama...
✓ Model loaded successfully

Junior is now: qwen2.5-coder:7b (Ollama)

Note: Previous Junior context has been cleared.
```

### `/junior` - Show Current Junior Status

```
You> /junior

Junior Engineer Status:
  Service: LM Studio
  Model: gpt-oss-20b-fp16
  Endpoint: http://localhost:1234/v1
  Context: 32,768 tokens
  Status: ✓ Available

  Last Assessment: 2025-12-06 (Score: 87%)
  See CLAUDE.md for details
```

## Technical Design

### New Files

```
go-port/
├── internal/
│   └── loader/
│       ├── loader.go       # Main loader logic
│       ├── services.go     # Service definitions & detection
│       ├── lmstudio.go     # LM Studio specific API
│       ├── ollama.go       # Ollama specific API
│       └── models.go       # Model listing/loading
```

### Service Interface

```go
type ModelService interface {
    Name() string
    BaseURL() string
    IsAvailable() bool
    ListModels() ([]ModelInfo, error)
    GetActiveModel() (string, error)
    LoadModel(name string) error  // Some services support this
}

type ModelInfo struct {
    Name        string
    Size        string  // "7B", "13B", etc.
    Quantization string // "Q4_K_M", "fp16", etc.
    Loaded      bool    // Currently active
}
```

### Ollama-Specific

Ollama requires explicitly loading models:

```go
// GET /api/tags - List available models
// POST /api/generate with empty prompt - Load model into memory

func (o *OllamaService) LoadModel(name string) error {
    // Ollama loads on first request, or we can warm it up
    resp, err := http.Post(o.URL+"/api/generate", "application/json",
        strings.NewReader(`{"model":"`+name+`","prompt":""}`))
    return err
}
```

### LM Studio-Specific

LM Studio models are loaded via the GUI, but we can detect what's running:

```go
// GET /v1/models - Returns currently loaded model

func (l *LMStudioService) GetActiveModel() (string, error) {
    resp, _ := http.Get(l.URL + "/v1/models")
    // Parse response for model name
}

func (l *LMStudioService) LoadModel(name string) error {
    return errors.New("LM Studio requires manual model loading via GUI")
}
```

## Startup Behavior

### Auto-Detection Priority

1. Check for `--local-url` flag (user override, skip detection)
2. Poll LM Studio (1234) - if model active, use it
3. Poll Ollama (11434) - if model active, use it
4. Poll other services...
5. If no active model found, prompt user or start without Junior

### No Model Running

```
$ ./weaver

Detecting local model services...
  ✓ LM Studio (localhost:1234) - Running (no model loaded)
  ✓ Ollama (localhost:11434) - Running (no model loaded)

⚠ No Junior model is currently active.

Options:
  1. Load a model with /models and /load
  2. Start without Junior (Senior only)
  3. Quit and load a model manually

Choice [1/2/3]: 2

Starting in Senior-only mode. Use /models to load a Junior later.
```

## Implementation Steps

1. [ ] Create `internal/loader/` package
2. [ ] Implement service detection for LM Studio
3. [ ] Implement service detection for Ollama
4. [ ] Add `/models` command to CLI
5. [ ] Add `/load` command to CLI
6. [ ] Add `/junior` command to CLI
7. [ ] Integrate detection into startup flow
8. [ ] Update banner to show Junior model info
9. [ ] Handle "no model" gracefully
10. [ ] Add tests with mock servers
11. [ ] Update help text

## API Reference

### LM Studio

```bash
# Health check / List models
curl http://localhost:1234/v1/models
# Response: {"data":[{"id":"gpt-oss-20b-fp16","object":"model",...}]}
```

### Ollama

```bash
# List available models
curl http://localhost:11434/api/tags
# Response: {"models":[{"name":"llama3:8b","size":4661224676,...}]}

# Check what's loaded (make a dummy request)
curl http://localhost:11434/api/generate -d '{"model":"llama3:8b","prompt":"hi","stream":false}'
```

## Edge Cases

- **Multiple services with active models:** Use first detected, show warning
- **Service goes down mid-session:** Graceful error, suggest /models
- **Model swap mid-conversation:** Clear Junior context, warn user
- **Unknown service on custom port:** Allow manual URL override still works

## Future Enhancements (Not in Scope)

- [ ] Model download/pull via CLI
- [ ] Model performance benchmarking
- [ ] Favorite models list
- [ ] Auto-load last used model
- [ ] Remote model servers (not just localhost)
