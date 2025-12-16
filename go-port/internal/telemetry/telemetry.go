// Package telemetry provides OpenTelemetry tracing for Weaver.
package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds telemetry configuration.
type Config struct {
	Enabled     bool
	Endpoint    string // e.g., "localhost:6006" for Phoenix
	ProjectName string // Phoenix project name
	ServiceName string
}

// DefaultConfig returns default telemetry config.
func DefaultConfig() Config {
	return Config{
		Enabled:     false,
		Endpoint:    "localhost:6006",
		ProjectName: "",
		ServiceName: "weaver",
	}
}

var (
	tracer   trace.Tracer
	provider *sdktrace.TracerProvider
	enabled  bool
)

// Init initializes the telemetry system.
func Init(cfg Config) error {
	if !cfg.Enabled {
		enabled = false
		tracer = otel.Tracer("weaver-noop")
		return nil
	}

	ctx := context.Background()

	// Build exporter options
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Endpoint),
		otlptracehttp.WithInsecure(), // Phoenix typically runs without TLS locally
		otlptracehttp.WithURLPath("/v1/traces"),
	}

	// Create OTLP HTTP exporter for Phoenix
	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return err
	}

	// Create resource with service info
	// Note: We don't use resource.Default() to avoid schema URL conflicts
	// Phoenix uses "openinference.project.name" resource attribute for project routing
	res := resource.NewWithAttributes(
		"",
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion("dev"),
		attribute.String("llm.system", "weaver"),
		attribute.String("openinference.project.name", cfg.ProjectName),
	)

	// Create trace provider
	provider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(provider)
	tracer = provider.Tracer("weaver")
	enabled = true

	return nil
}

// Shutdown cleanly shuts down the telemetry system.
// This flushes any pending traces before returning.
func Shutdown(ctx context.Context) error {
	if provider != nil {
		// Use a longer timeout to ensure traces are flushed
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return provider.Shutdown(shutdownCtx)
	}
	return nil
}

// Enabled returns whether telemetry is enabled.
func Enabled() bool {
	return enabled
}

// Flush forces any pending traces to be exported.
func Flush(ctx context.Context) error {
	if provider != nil {
		return provider.ForceFlush(ctx)
	}
	return nil
}

// Tracer returns the global tracer.
func Tracer() trace.Tracer {
	if tracer == nil {
		return otel.Tracer("weaver-noop")
	}
	return tracer
}

// LLMSpan represents an LLM call span with attributes.
type LLMSpan struct {
	span      trace.Span
	startTime time.Time
}

// StartLLMSpan starts a new span for an LLM call.
func StartLLMSpan(ctx context.Context, name string, model string, role string) (context.Context, *LLMSpan) {
	ctx, span := Tracer().Start(ctx, name,
		trace.WithAttributes(
			attribute.String("llm.vendor", role), // "senior" or "junior"
			attribute.String("llm.request.model", model),
			attribute.String("llm.system", "weaver"),
		),
	)

	return ctx, &LLMSpan{
		span:      span,
		startTime: time.Now(),
	}
}

// SetInput records the input prompt.
func (s *LLMSpan) SetInput(prompt string) {
	s.span.SetAttributes(
		attribute.String("llm.prompts.0.content", prompt),
		attribute.String("llm.prompts.0.role", "user"),
	)
}

// SetOutput records the output completion.
func (s *LLMSpan) SetOutput(response string) {
	s.span.SetAttributes(
		attribute.String("llm.completions.0.content", response),
		attribute.String("llm.completions.0.role", "assistant"),
	)
}

// SetTokens records token counts if available.
func (s *LLMSpan) SetTokens(promptTokens, completionTokens int) {
	if promptTokens > 0 {
		s.span.SetAttributes(attribute.Int("llm.token_count.prompt", promptTokens))
	}
	if completionTokens > 0 {
		s.span.SetAttributes(attribute.Int("llm.token_count.completion", completionTokens))
	}
	if promptTokens > 0 || completionTokens > 0 {
		s.span.SetAttributes(attribute.Int("llm.token_count.total", promptTokens+completionTokens))
	}
}

// SetError records an error on the span.
func (s *LLMSpan) SetError(err error) {
	s.span.RecordError(err)
	s.span.SetAttributes(attribute.Bool("error", true))
}

// End completes the span and flushes traces.
func (s *LLMSpan) End() {
	duration := time.Since(s.startTime)
	s.span.SetAttributes(attribute.Float64("llm.latency_ms", float64(duration.Milliseconds())))
	s.span.End()

	// Force flush to ensure trace is sent immediately
	if enabled {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		Flush(ctx)
	}
}

// EstimateTokens provides a rough token estimate (4 chars per token).
func EstimateTokens(text string) int {
	return len(text) / 4
}
