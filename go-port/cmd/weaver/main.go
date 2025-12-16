// Package main is the entry point for the Weaver CLI.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chzyer/readline"
	"github.com/r3d91ll/weaver-code/internal/assessment"
	"github.com/r3d91ll/weaver-code/internal/loader"
	"github.com/r3d91ll/weaver-code/internal/orchestrator"
	"github.com/r3d91ll/weaver-code/internal/senior"
	"github.com/r3d91ll/weaver-code/internal/telemetry"
)

// Version info (set via ldflags)
var (
	Version = "dev"
	Commit  = "unknown"
)

// Global loader for model management
var modelLoader *loader.Loader

// JuniorInfo tracks the current Junior configuration
type JuniorInfo struct {
	Service     loader.ServiceType
	ServiceName string
	ModelName   string
	URL         string
}

var currentJunior *JuniorInfo

func main() {
	// Parse flags
	localURL := flag.String("local-url", "", "Local model API URL (overrides auto-detection)")
	localModel := flag.String("local-model", "", "Local model name (overrides auto-detection)")
	localContext := flag.Int("local-context", 32000, "Local model context window size")
	provider := flag.String("provider", "claude_code", "Senior model provider (claude_code, anthropic_api)")
	message := flag.String("m", "", "Single message (non-interactive)")
	noDetect := flag.Bool("no-detect", false, "Skip auto-detection of local models")
	traceProject := flag.String("trace", "", "Enable tracing and send to Phoenix project (e.g., --trace weaver-dev)")
	traceEndpoint := flag.String("trace-endpoint", "localhost:6006", "Phoenix endpoint for traces")
	version := flag.Bool("version", false, "Show version")
	help := flag.Bool("help", false, "Show help")

	// Training data collection flags
	trainingRun := flag.Bool("training-run", false, "Run extended assessment for training data collection")
	numChallenges := flag.Int("num-challenges", 1000, "Number of challenges for training run")
	trainingOutput := flag.String("training-output", "./training_data", "Output directory for training data")

	flag.Parse()

	if *version {
		fmt.Printf("weaver %s (%s)\n", Version, Commit)
		return
	}

	if *help {
		printHelp()
		return
	}

	// Initialize telemetry if enabled
	if *traceProject != "" {
		cfg := telemetry.Config{
			Enabled:     true,
			Endpoint:    *traceEndpoint,
			ProjectName: *traceProject,
			ServiceName: "weaver",
		}
		if err := telemetry.Init(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to initialize telemetry: %v\n", err)
		} else {
			fmt.Printf(colorGreen+"✓ Tracing enabled"+colorReset+" (project: %s, endpoint: http://%s/v1/traces)\n", *traceProject, *traceEndpoint)
			defer func() {
				fmt.Print(colorGray + "Flushing traces..." + colorReset)
				if err := telemetry.Shutdown(context.Background()); err != nil {
					fmt.Printf(" %serror: %v%s\n", colorRed, err, colorReset)
				} else {
					fmt.Println(colorGreen + " done" + colorReset)
				}
			}()
		}
	} else {
		telemetry.Init(telemetry.DefaultConfig())
	}

	// Initialize model loader
	modelLoader = loader.NewLoader()

	// Create orchestrator config
	cfg := orchestrator.DefaultConfig()
	cfg.JuniorContextLimit = *localContext
	cfg.SeniorProvider = senior.Provider(*provider)

	// Handle Junior model configuration
	if *localURL != "" {
		// User explicitly specified URL - use it
		cfg.JuniorURL = *localURL
		if *localModel != "" {
			cfg.JuniorModel = *localModel
		} else {
			cfg.JuniorModel = "local-model"
		}
		currentJunior = &JuniorInfo{
			ServiceName: "Manual",
			ModelName:   cfg.JuniorModel,
			URL:         cfg.JuniorURL,
		}
	} else if !*noDetect {
		// Auto-detect local model services
		fmt.Println("\nDetecting local model services...")
		juniorCfg, statuses := modelLoader.DetectJunior()

		// Print detection results
		for _, status := range statuses {
			fmt.Println(loader.FormatStatus(status))
		}
		fmt.Println()

		if juniorCfg != nil && juniorCfg.ModelName != "" {
			cfg.JuniorURL = juniorCfg.URL
			cfg.JuniorModel = juniorCfg.ModelName
			currentJunior = &JuniorInfo{
				Service:     juniorCfg.Service,
				ServiceName: juniorCfg.ServiceName,
				ModelName:   juniorCfg.ModelName,
				URL:         juniorCfg.URL,
			}
			fmt.Printf("Using Junior: %s via %s\n\n", juniorCfg.ModelName, juniorCfg.ServiceName)
		} else if juniorCfg != nil {
			// Service available but no active model reported
			cfg.JuniorURL = juniorCfg.URL
			cfg.JuniorModel = "none"
			currentJunior = &JuniorInfo{
				Service:     juniorCfg.Service,
				ServiceName: juniorCfg.ServiceName,
				ModelName:   "",
				URL:         juniorCfg.URL,
			}
			fmt.Printf(colorYellow+"Warning: %s is running but no model is configured.\n"+colorReset, juniorCfg.ServiceName)
			fmt.Println("Use /load to select a model. Example: /load ollama gpt-oss:20b-weaver")
		} else {
			// No services available - default to Ollama endpoint
			cfg.JuniorURL = "http://localhost:11434/v1"
			cfg.JuniorModel = "none"
			currentJunior = nil
			fmt.Println(colorYellow + "Warning: No local model services detected." + colorReset)
			fmt.Println("Junior delegation will be unavailable until a service is started.")
		}
	} else {
		// --no-detect flag used with defaults - prefer Ollama
		cfg.JuniorURL = "http://localhost:11434/v1"
		cfg.JuniorModel = "gpt-oss:20b-weaver"
		currentJunior = &JuniorInfo{
			ServiceName: "Ollama",
			ModelName:   "gpt-oss:20b-weaver",
			URL:         cfg.JuniorURL,
		}
	}

	weaver, err := orchestrator.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Load JUNIOR.md if it exists (from previous assessment)
	loadJuniorMd(weaver)

	// Handle training data collection mode
	if *trainingRun {
		ctx := context.Background()
		runTrainingDataCollection(ctx, weaver, *numChallenges, *trainingOutput)
		return
	}

	// Handle single message mode
	if *message != "" {
		ctx := context.Background()
		response, err := weaver.Chat(ctx, *message)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)
		return
	}

	// Handle piped input
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Stdin is a pipe
		scanner := bufio.NewScanner(os.Stdin)
		var input strings.Builder
		for scanner.Scan() {
			input.WriteString(scanner.Text())
			input.WriteString("\n")
		}
		if input.Len() > 0 {
			ctx := context.Background()
			response, err := weaver.Chat(ctx, strings.TrimSpace(input.String()))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(response)
			return
		}
	}

	// Interactive mode
	runInteractive(weaver)
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorBlue   = "\033[34m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorGray   = "\033[90m"
)

func runInteractive(weaver *orchestrator.Weaver) {
	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nGoodbye!")
		cancel()
		os.Exit(0)
	}()

	printBanner(weaver)

	// Setup readline with history
	homeDir, _ := os.UserHomeDir()
	historyFile := filepath.Join(homeDir, ".weaver", "history")

	// Ensure .weaver directory exists
	os.MkdirAll(filepath.Dir(historyFile), 0755)

	// Command completer for slash commands
	completer := readline.NewPrefixCompleter(
		readline.PcItem("/help"),
		readline.PcItem("/quit"),
		readline.PcItem("/exit"),
		readline.PcItem("/clear"),
		readline.PcItem("/history"),
		readline.PcItem("/models"),
		readline.PcItem("/load",
			readline.PcItem("ollama"),
			readline.PcItem("lmstudio"),
		),
		readline.PcItem("/junior"),
		readline.PcItem("/junior-assessment"),
		readline.PcItem("/junior-assessment-long"),
		readline.PcItem("/local"),
		readline.PcItem("/memory"),
		readline.PcItem("/export"),
		readline.PcItem("/compact"),
	)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:            colorGreen + "You> " + colorReset,
		HistoryFile:       historyFile,
		HistoryLimit:      1000,
		AutoComplete:      completer,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing readline: %v\n", err)
		// Fallback to basic scanner
		runInteractiveBasic(weaver, ctx)
		return
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				fmt.Println("\nUse /quit to exit or Ctrl+D")
				continue
			}
			continue
		} else if err == io.EOF {
			fmt.Println("\nGoodbye!")
			break
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		// Handle commands
		if strings.HasPrefix(input, "/") {
			if handleCommand(input, weaver, ctx) {
				continue
			}
		}

		// Send to weaver with structured streaming
		fmt.Println()
		fmt.Print(colorGray + "Sending to Senior..." + colorReset)
		chunks, errs := weaver.ChatStreamStructured(ctx, input)

		// Clear the "Sending" message once we get first response
		firstChunk := true

		currentAgent := ""
		for chunk := range chunks {
			// Clear "Sending..." on first chunk
			if firstChunk {
				fmt.Print("\r" + strings.Repeat(" ", 30) + "\r") // Clear line
				firstChunk = false
			}

			// Handle agent transitions
			if chunk.Agent != currentAgent && chunk.Content != "" {
				if currentAgent != "" {
					fmt.Println() // End previous agent's output
				}
				currentAgent = chunk.Agent
				printAgentLabel(currentAgent)
			}

			// Print content
			if chunk.Content != "" {
				fmt.Print(chunk.Content)
			}

			// Handle agent completion
			if chunk.Done && currentAgent != "" {
				fmt.Println()
				currentAgent = ""
			}
		}

		if err := <-errs; err != nil {
			fmt.Printf(colorRed + "Error: %v" + colorReset + "\n", err)
		}
		fmt.Println()
	}
}

func printAgentLabel(agent string) {
	switch agent {
	case "senior":
		fmt.Print(colorBlue + "Senior> " + colorReset)
	case "junior":
		fmt.Print(colorYellow + "Junior> " + colorReset)
	case "system":
		fmt.Print(colorGray + "System> " + colorReset)
	}
}

// runInteractiveBasic is a fallback when readline isn't available.
func runInteractiveBasic(weaver *orchestrator.Weaver, ctx context.Context) {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print(colorGreen + "You> " + colorReset)

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle commands
		if strings.HasPrefix(input, "/") {
			if handleCommand(input, weaver, ctx) {
				continue
			}
		}

		// Send to weaver with structured streaming
		fmt.Println()
		fmt.Print(colorGray + "Sending to Senior..." + colorReset)
		chunks, errs := weaver.ChatStreamStructured(ctx, input)

		// Clear the "Sending" message once we get first response
		firstChunk := true

		currentAgent := ""
		for chunk := range chunks {
			// Clear "Sending..." on first chunk
			if firstChunk {
				fmt.Print("\r" + strings.Repeat(" ", 30) + "\r") // Clear line
				firstChunk = false
			}

			// Handle agent transitions
			if chunk.Agent != currentAgent && chunk.Content != "" {
				if currentAgent != "" {
					fmt.Println() // End previous agent's output
				}
				currentAgent = chunk.Agent
				printAgentLabel(currentAgent)
			}

			// Print content
			if chunk.Content != "" {
				fmt.Print(chunk.Content)
			}

			// Handle agent completion
			if chunk.Done && currentAgent != "" {
				fmt.Println()
				currentAgent = ""
			}
		}

		if err := <-errs; err != nil {
			fmt.Printf(colorRed+"Error: %v"+colorReset+"\n", err)
		}
		fmt.Println()
	}
}

func handleCommand(input string, weaver *orchestrator.Weaver, ctx context.Context) bool {
	parts := strings.SplitN(input, " ", 2)
	cmd := strings.ToLower(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}

	switch cmd {
	case "/help":
		printHelp()
		return true

	case "/quit", "/exit", "/q":
		fmt.Println("Goodbye!")
		os.Exit(0)

	case "/local":
		if arg == "" {
			fmt.Println(colorRed + "Usage: /local <message>" + colorReset)
			return true
		}
		fmt.Println()
		fmt.Print(colorGray + "Sending to Junior..." + colorReset)
		response, err := weaver.ChatJuniorDirect(ctx, arg)
		fmt.Print("\r" + strings.Repeat(" ", 30) + "\r") // Clear status
		if err != nil {
			fmt.Printf(colorRed+"Error: %v"+colorReset+"\n", err)
		} else {
			fmt.Print(colorYellow + "Junior> " + colorReset)
			fmt.Println(response)
		}
		fmt.Println()
		return true

	case "/agents":
		agents := weaver.ListAgents()
		fmt.Println("\nAgents:")
		for _, a := range agents {
			status := colorGreen + "available" + colorReset
			if !a.Available {
				status = colorRed + "unavailable" + colorReset
			}
			fmt.Printf("  [%s] %s (%s) - %s\n", a.Role, a.Name, a.Provider, status)
		}
		fmt.Println()
		return true

	case "/memory":
		if arg == "" {
			notes := weaver.Memory().List(10, "", nil)
			if len(notes) == 0 {
				fmt.Println("\nNo shared notes.")
			} else {
				fmt.Println("\nShared Notes:")
				for _, n := range notes {
					fmt.Printf("  [%s] %s: %s\n", n.ID, n.Author, truncate(n.Content, 60))
				}
				fmt.Println()
			}
		} else if arg == "clear" {
			weaver.Memory().Clear()
			fmt.Println("\nShared notes cleared.")
		}
		return true

	case "/clear":
		weaver.ClearContext()
		fmt.Println("\nConversation cleared (Senior + Junior).")
		return true

	case "/clear-senior":
		weaver.ClearSeniorContext()
		fmt.Println("\nSenior context cleared.")
		return true

	case "/clear-junior":
		weaver.ClearJuniorContext()
		fmt.Println("\nJunior context cleared.")
		return true

	case "/models":
		printModels()
		return true

	case "/load":
		if arg == "" {
			fmt.Println(colorRed + "Usage: /load <service> <model>" + colorReset)
			fmt.Println("Example: /load ollama llama3:8b")
			return true
		}
		loadModel(arg, weaver)
		return true

	case "/junior":
		printJuniorStatus()
		return true

	case "/junior-assessment":
		runJuniorAssessment(ctx, weaver)
		return true

	case "/junior-assessment-long":
		runJuniorAssessmentLong(ctx, weaver)
		return true

	default:
		// Unknown command, pass through to senior model
		return false
	}

	return true
}

// printModels lists all available models from detected services.
func printModels() {
	if modelLoader == nil {
		fmt.Println(colorRed + "\nModel loader not initialized." + colorReset)
		return
	}

	statuses := modelLoader.GetAllStatuses()

	fmt.Println("\n" + colorBlue + "Local Model Services:" + colorReset)

	hasModels := false
	for _, status := range statuses {
		if !status.Available {
			fmt.Printf("\n%s (%s) %s\n", status.Name, status.URL, colorRed+"Not running"+colorReset)
			continue
		}

		fmt.Printf("\n%s (%s) %s\n", status.Name, status.URL, colorGreen+"Running"+colorReset)

		if len(status.Models) == 0 {
			fmt.Println("  (no models available)")
			continue
		}

		hasModels = true
		for _, m := range status.Models {
			marker := "[ ]"
			if m.Loaded || m.Name == status.ActiveModel {
				marker = colorGreen + "[ACTIVE]" + colorReset
			}

			sizeInfo := ""
			if m.Size != "" {
				sizeInfo = fmt.Sprintf(" (%s", m.Size)
				if m.Quantization != "" {
					sizeInfo += " " + m.Quantization
				}
				sizeInfo += ")"
			}

			fmt.Printf("  %s %s%s\n", marker, m.Name, sizeInfo)
		}
	}

	fmt.Println()
	if hasModels {
		fmt.Println("Use /load <service> <model> to switch models")
		fmt.Println("Example: /load ollama llama3:8b")
	}
	fmt.Println()
}

// loadModel loads a model on the specified service.
func loadModel(arg string, weaver *orchestrator.Weaver) {
	parts := strings.SplitN(arg, " ", 2)
	if len(parts) != 2 {
		fmt.Println(colorRed + "Usage: /load <service> <model>" + colorReset)
		fmt.Println("Example: /load ollama llama3:8b")
		return
	}

	serviceName := strings.ToLower(parts[0])
	modelName := parts[1]

	// Map service name to type
	var svcType loader.ServiceType
	switch serviceName {
	case "lmstudio", "lm-studio", "lm_studio":
		svcType = loader.ServiceLMStudio
	case "ollama":
		svcType = loader.ServiceOllama
	case "vllm":
		svcType = loader.ServiceVLLM
	case "localai", "local-ai", "local_ai":
		svcType = loader.ServiceLocalAI
	default:
		fmt.Printf(colorRed+"Unknown service: %s\n"+colorReset, serviceName)
		fmt.Println("Available services: lmstudio, ollama, vllm, localai")
		return
	}

	// Check if service is available
	status := modelLoader.GetServiceStatus(svcType)
	if status == nil || !status.Available {
		fmt.Printf(colorRed+"%s is not running\n"+colorReset, serviceName)
		return
	}

	fmt.Printf("\nLoading %s via %s...\n", modelName, status.Name)

	// Attempt to load the model
	if err := modelLoader.LoadModel(svcType, modelName); err != nil {
		fmt.Printf(colorRed+"Error: %v\n"+colorReset, err)
		return
	}

	fmt.Println(colorGreen + "Model loaded successfully" + colorReset)

	// Update current Junior info
	url := status.URL
	if svcType == loader.ServiceOllama {
		url += "/v1"
	} else if svcType == loader.ServiceLMStudio || svcType == loader.ServiceVLLM || svcType == loader.ServiceLocalAI {
		url += "/v1"
	}

	currentJunior = &JuniorInfo{
		Service:     svcType,
		ServiceName: status.Name,
		ModelName:   modelName,
		URL:         url,
	}

	// Update weaver's junior model configuration
	// LM Studio handles context automatically, Ollama uses modelfile settings
	contextLimit := 32000 // Default conservative limit
	if svcType == loader.ServiceLMStudio {
		contextLimit = 32000 // LM Studio manages context per-model
	} else if svcType == loader.ServiceOllama {
		contextLimit = 131072 // Ollama models often have larger context via modelfiles
	}
	weaver.UpdateJuniorModel(url, modelName, contextLimit)

	// Clear Junior context since we switched models
	weaver.ClearJuniorContext()
	fmt.Println("\nJunior is now: " + colorYellow + modelName + colorReset + " (" + status.Name + ")")
	fmt.Println("Note: Junior context has been cleared.")
	fmt.Println()
}

// printJuniorStatus shows the current Junior model status.
func printJuniorStatus() {
	fmt.Println("\n" + colorBlue + "Junior Engineer Status:" + colorReset)

	if currentJunior == nil {
		fmt.Println("  Status: " + colorRed + "Not configured" + colorReset)
		fmt.Println("\n  No local model service detected.")
		fmt.Println("  Start LM Studio or Ollama and restart Weaver,")
		fmt.Println("  or use /models and /load to configure manually.")
		fmt.Println()
		return
	}

	if currentJunior.ModelName == "" {
		fmt.Printf("  Service: %s\n", currentJunior.ServiceName)
		fmt.Println("  Model: " + colorYellow + "None loaded" + colorReset)
		fmt.Printf("  Endpoint: %s\n", currentJunior.URL)
		fmt.Println("  Status: " + colorYellow + "Service available, no model" + colorReset)
		fmt.Println("\n  Use /models to see available models")
		fmt.Println("  Use /load <service> <model> to load one")
		fmt.Println()
		return
	}

	fmt.Printf("  Service: %s\n", currentJunior.ServiceName)
	fmt.Printf("  Model: %s\n", currentJunior.ModelName)
	fmt.Printf("  Endpoint: %s\n", currentJunior.URL)
	fmt.Println("  Status: " + colorGreen + "Available" + colorReset)
	fmt.Println()
}

// weaverJuniorClient adapts Weaver to both JuniorClient and JuniorToolClient interfaces.
type weaverJuniorClient struct {
	weaver *orchestrator.Weaver
}

func (w *weaverJuniorClient) Chat(ctx context.Context, message string) (string, error) {
	return w.weaver.ChatJuniorRaw(ctx, message)
}

func (w *weaverJuniorClient) Name() string {
	return w.weaver.JuniorModel()
}

func (w *weaverJuniorClient) IsAvailable() bool {
	return w.weaver.JuniorIsAvailable()
}

// ChatWithTools implements JuniorToolClient for tool-enabled assessment challenges.
// Runs full tool execution loop and extracts code from write_file tool calls.
func (w *weaverJuniorClient) ChatWithTools(ctx context.Context, message string) (string, error) {
	return w.weaver.ChatJuniorWithToolsRaw(ctx, message)
}

// ToolsEnabled implements JuniorToolClient.
func (w *weaverJuniorClient) ToolsEnabled() bool {
	return w.weaver.ToolsEnabled()
}

// weaverSeniorClient adapts Weaver to the assessment.SeniorClient interface.
type weaverSeniorClient struct {
	weaver *orchestrator.Weaver
}

func (w *weaverSeniorClient) Chat(ctx context.Context, message string) (string, error) {
	return w.weaver.ChatSeniorRaw(ctx, message)
}

// runJuniorAssessment runs the Junior model assessment.
func runJuniorAssessment(ctx context.Context, weaver *orchestrator.Weaver) {
	fmt.Println("\n" + colorBlue + "Starting Junior Assessment..." + colorReset)
	fmt.Println("This will run 15 coding challenges and have Senior evaluate the responses.")
	fmt.Println("Estimated time: 5-10 minutes depending on model speed.")
	fmt.Printf("DEBUG: Using endpoint: %s\n", weaver.JuniorURL())
	fmt.Printf("DEBUG: Using model: %s\n", weaver.JuniorModel())

	// Check if Junior is available
	if !weaver.JuniorIsAvailable() {
		fmt.Println(colorRed + "Error: Junior model is not available." + colorReset)
		fmt.Println("Please ensure a local model is running and try again.")
		return
	}

	// Create the assessor
	juniorClient := &weaverJuniorClient{weaver: weaver}
	seniorClient := &weaverSeniorClient{weaver: weaver}
	assessor := assessment.NewAssessor(juniorClient, seniorClient)

	// Set up progress callback
	assessor.SetProgressCallback(func(current, total int, name string) {
		fmt.Printf("\r  [%d/%d] %s...                    ", current, total, name)
	})

	// Run the assessment
	report, err := assessor.Run(ctx)
	if err != nil {
		fmt.Printf("\n"+colorRed+"Assessment failed: %v"+colorReset+"\n", err)
		return
	}

	// Clear the progress line
	fmt.Print("\r                                                            \r")

	// Set the endpoint in the report
	report.ModelEndpoint = weaver.JuniorURL()

	// Get working directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf(colorRed+"Warning: Could not get working directory: %v"+colorReset+"\n", err)
		cwd = "."
	}

	// Write to CLAUDE.md (for Senior to understand Junior's capabilities)
	if err := assessment.WriteReport(report, cwd); err != nil {
		fmt.Printf(colorRed+"Warning: Could not write CLAUDE.md: %v"+colorReset+"\n", err)
	} else {
		fmt.Println(colorGreen + "✓ CLAUDE.md updated with assessment results" + colorReset)
	}

	// Write to JUNIOR.md (for Junior's self-awareness)
	if err := assessment.WriteJuniorReport(report, cwd); err != nil {
		fmt.Printf(colorRed+"Warning: Could not write JUNIOR.md: %v"+colorReset+"\n", err)
	} else {
		fmt.Println(colorGreen + "✓ JUNIOR.md created with capability profile" + colorReset)
	}

	// Update Junior's system prompt with the assessment results
	// Use model-specific prompt (e.g., Devstral gets optimized prompt)
	juniorMdContent := assessment.GenerateJuniorMarkdown(report)
	modelName := weaver.JuniorModel()
	newPrompt := orchestrator.BuildJuniorPromptForModelWithAssessment(modelName, juniorMdContent)
	weaver.UpdateJuniorPrompt(newPrompt)
	fmt.Println(colorGreen + "✓ Junior's system prompt updated with assessment" + colorReset)

	// Print summary
	fmt.Println(assessment.GenerateSummary(report))
}

// runTrainingDataCollection runs an extended assessment for training data collection.
func runTrainingDataCollection(ctx context.Context, weaver *orchestrator.Weaver, numChallenges int, outputDir string) {
	fmt.Println("\n" + colorBlue + "╔═══════════════════════════════════════════╗" + colorReset)
	fmt.Println(colorBlue + "║" + colorReset + "     Training Data Collection Mode         " + colorBlue + "║" + colorReset)
	fmt.Println(colorBlue + "╚═══════════════════════════════════════════╝" + colorReset)
	fmt.Println()
	fmt.Printf("Model: %s\n", weaver.JuniorModel())
	fmt.Printf("Challenges: %d\n", numChallenges)
	fmt.Printf("Output: %s\n", outputDir)
	fmt.Println()
	fmt.Println("This will run " + colorYellow + "overnight" + colorReset + " - press Ctrl+C to stop early (progress will be saved).")
	fmt.Println()

	// Check if Junior is available
	if !weaver.JuniorIsAvailable() {
		fmt.Println(colorRed + "Error: Junior model is not available." + colorReset)
		fmt.Println("Please ensure a local model is running and try again.")
		return
	}

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n" + colorYellow + "Stopping... saving progress..." + colorReset)
		cancel()
	}()

	// Create config
	config := assessment.ExtendedAssessmentConfig{
		NumChallenges:       numChallenges,
		OutputDir:           outputDir,
		ContinueOnError:     true,
		SaveInterval:        100,
		ExportFormats:       []string{"jsonl", "alpaca", "sharegpt", "dpo"},
		MinQualityForExport: "good",
	}

	// Create assessor
	juniorClient := &weaverJuniorClient{weaver: weaver}
	seniorClient := &weaverSeniorClient{weaver: weaver}
	assessor := assessment.NewExtendedAssessor(juniorClient, seniorClient, config)

	// Set up progress callback
	assessor.SetProgressCallback(func(completed, total int, lastChallenge string, elapsed, remaining time.Duration) {
		pct := float64(completed) / float64(total) * 100
		fmt.Printf("\r[%d/%d] %.1f%% | %s | Elapsed: %s | Remaining: ~%s          ",
			completed, total, pct, lastChallenge,
			elapsed.Round(time.Second), remaining.Round(time.Second))
	})

	// Run the extended assessment
	fmt.Println("Starting assessment...")
	fmt.Println()

	report, err := assessor.Run(ctx)
	if err != nil && err != context.Canceled {
		fmt.Printf("\n"+colorRed+"Error: %v"+colorReset+"\n", err)
	}

	// Print summary
	if report != nil {
		fmt.Println(report.Summary())

		// Write report file
		if err := assessment.WriteExtendedReport(report, outputDir); err != nil {
			fmt.Printf(colorRed+"Warning: Could not write report: %v"+colorReset+"\n", err)
		}
	}

	fmt.Println(colorGreen + "Training data collection complete!" + colorReset)
	fmt.Printf("Files saved to: %s\n", outputDir)
}

// runJuniorAssessmentLong runs the extended assessment from within the interactive session.
func runJuniorAssessmentLong(ctx context.Context, weaver *orchestrator.Weaver) {
	fmt.Println("\n" + colorBlue + "Extended Junior Assessment (Training Data Collection)" + colorReset)
	fmt.Println("This runs many challenges and exports results for fine-tuning.")
	fmt.Println()

	// Get number of challenges
	fmt.Print("Number of challenges [1000]: ")
	var numStr string
	fmt.Scanln(&numStr)
	numChallenges := 1000
	if numStr != "" {
		if n, err := strconv.Atoi(numStr); err == nil && n > 0 {
			numChallenges = n
		}
	}

	// Get output directory
	fmt.Print("Output directory [./training_data]: ")
	var outputDir string
	fmt.Scanln(&outputDir)
	if outputDir == "" {
		outputDir = "./training_data"
	}

	fmt.Println()
	fmt.Printf("Will run %d challenges, output to %s\n", numChallenges, outputDir)
	fmt.Print("Press Enter to start (Ctrl+C to cancel)...")
	fmt.Scanln()

	// Run the training data collection
	runTrainingDataCollection(ctx, weaver, numChallenges, outputDir)
}

func printBanner(weaver *orchestrator.Weaver) {
	provider := weaver.SeniorProvider()
	providerName := "Claude Code"
	if provider == senior.ProviderAnthropicAPI {
		providerName = "Anthropic API"
	}

	// Format Junior info for banner
	juniorInfo := "Not configured"
	if currentJunior != nil {
		if currentJunior.ModelName != "" {
			juniorInfo = fmt.Sprintf("%s (%s)", currentJunior.ModelName, currentJunior.ServiceName)
		} else {
			juniorInfo = fmt.Sprintf("None loaded (%s)", currentJunior.ServiceName)
		}
	}

	// Truncate if too long
	if len(juniorInfo) > 30 {
		juniorInfo = juniorInfo[:27] + "..."
	}

	fmt.Println()
	fmt.Println(colorBlue + "╔═══════════════════════════════════════════╗" + colorReset)
	fmt.Println(colorBlue + "║" + colorReset + "       \033[1mWeaver Code\033[0m - Go Edition          " + colorBlue + "║" + colorReset)
	fmt.Printf(colorBlue+"║"+colorReset+"  Senior: %-32s"+colorBlue+"║"+colorReset+"\n", providerName)
	fmt.Printf(colorBlue+"║"+colorReset+"  Junior: %-32s"+colorBlue+"║"+colorReset+"\n", juniorInfo)
	fmt.Println(colorBlue + "╚═══════════════════════════════════════════╝" + colorReset)
	fmt.Println()
	fmt.Println("Commands: /help /models /junior /junior-assessment /junior-assessment-long /local /clear /quit")
	fmt.Println()
}

func printHelp() {
	fmt.Print(`Weaver Code - Universal AI Orchestration CLI

Usage:
  weaver                    Interactive mode (auto-detects local models)
  weaver -m "message"       Single query
  echo "msg" | weaver       Pipe mode

Options:
  --provider          Senior model provider: claude_code (default), anthropic_api
  --local-url         Junior model API URL (overrides auto-detection)
  --local-model       Junior model name (overrides auto-detection)
  --local-context     Junior model context size (default: 32000)
  --no-detect         Skip auto-detection of local models
  --trace <project>   Enable tracing to Phoenix project (e.g., --trace weaver-dev)
  --trace-endpoint    Phoenix endpoint (default: localhost:6006)
  --version           Show version
  --help              Show this help

Training Data Collection (for fine-tuning):
  --training-run      Run extended assessment for training data collection
  --num-challenges    Number of challenges to generate (default: 1000)
  --training-output   Output directory for training data (default: ./training_data)

  Example: weaver --training-run --num-challenges 1000 --training-output ./training_data

  This runs overnight and exports training data in multiple formats:
    - JSONL (raw examples with scores and metadata)
    - Alpaca (instruction/output pairs for SFT)
    - ShareGPT (conversation format for SFT)
    - DPO pairs (good/bad examples for preference learning)

Model Commands:
  /models                  List available local models (LM Studio, Ollama, etc.)
  /load <svc> <model>      Load a model (e.g., /load ollama llama3:8b)
  /junior                  Show current Junior model status
  /junior-assessment       Run coding challenges to evaluate Junior's abilities
  /junior-assessment-long  Extended assessment (1000+ challenges) for training data

Chat Commands:
  /local <msg>    Send message directly to Junior (bypasses Senior)
  /agents         List available agents
  /memory         Show shared notes
  /memory clear   Clear shared notes
  /clear          Clear all conversation history (Senior + Junior)
  /clear-senior   Clear only Senior's context
  /clear-junior   Clear only Junior's context
  /quit           Exit

Message Flow:
  You>     Your input (green)
  Senior>  Claude's response (blue)
  Junior>  Local model's response (yellow)

On startup, Weaver auto-detects running local model services:
  - LM Studio (localhost:1234) - Recommended, huge model selection, native tool support
  - Ollama (localhost:11434) - Requires modelfiles for custom context windows
  - vLLM (localhost:8000)
  - LocalAI (localhost:8080)

LM Studio Notes:
  - Install CLI for model loading: npx lmstudio install-cli
  - Native function/tool calling support (Qwen, Llama 3.1+, Ministral)
  - No modelfiles needed - context/GPU handled automatically

All messages go to Senior by default. Senior may delegate to Junior
internally. Use /local only for direct Junior access.
`)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// loadJuniorMd loads JUNIOR.md from the current directory if it exists
// and updates Junior's system prompt with the assessment results.
func loadJuniorMd(weaver *orchestrator.Weaver) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	juniorMdPath := cwd + "/JUNIOR.md"
	content, err := os.ReadFile(juniorMdPath)
	if err != nil {
		// File doesn't exist or can't be read - that's fine
		return
	}

	// Update Junior's system prompt with the assessment content
	// Use model-specific prompt (e.g., Devstral gets optimized prompt)
	modelName := weaver.JuniorModel()
	newPrompt := orchestrator.BuildJuniorPromptForModelWithAssessment(modelName, string(content))
	weaver.UpdateJuniorPrompt(newPrompt)

	fmt.Println(colorGray + "Loaded JUNIOR.md assessment profile" + colorReset)
}
