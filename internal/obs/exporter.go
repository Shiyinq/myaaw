package obs

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
)

// Exporter forwards finished runs to an OTLP collector so tools such as
// Langfuse or Phoenix can render the same trace. It stays inert unless
// OTEL_EXPORTER_OTLP_ENDPOINT is set, so the default install pays nothing.
type Exporter struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	enabled  bool
}

// NewExporter builds an OTLP exporter from the environment. A missing endpoint
// returns a disabled exporter and no error.
func NewExporter(ctx context.Context) (*Exporter, error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		return &Exporter{}, nil
	}

	var (
		spanExporter sdktrace.SpanExporter
		err          error
	)
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		spanExporter, err = otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	} else {
		options := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
		if envBoolDefault("OTEL_EXPORTER_OTLP_INSECURE", true) {
			options = append(options, otlptracegrpc.WithInsecure())
		}
		spanExporter, err = otlptracegrpc.New(ctx, options...)
	}
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName()),
	))
	if err != nil {
		return nil, fmt.Errorf("build otlp resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter),
		sdktrace.WithResource(res),
	)
	return &Exporter{
		provider: provider,
		tracer:   provider.Tracer("myaaw/obs"),
		enabled:  true,
	}, nil
}

// Enabled reports whether an endpoint was configured.
func (e *Exporter) Enabled() bool {
	return e != nil && e.enabled
}

// Export sends one run and its spans. Spans are emitted parent-first so the
// collector sees a connected trace.
func (e *Exporter) Export(run Run, spans []Span) {
	if !e.Enabled() {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[obs] otel export panicked: %v", recovered)
		}
	}()

	contexts := make(map[string]trace.SpanContext, len(spans))
	for _, span := range spans {
		parentCtx := context.Background()
		if parent, ok := contexts[span.ParentID]; ok {
			parentCtx = trace.ContextWithSpanContext(parentCtx, parent)
		}

		started := time.UnixMilli(span.StartedAt)
		options := []trace.SpanStartOption{
			trace.WithTimestamp(started),
			trace.WithAttributes(spanAttributes(run, span)...),
		}
		_, otelSpan := e.tracer.Start(parentCtx, spanName(span), options...)

		if span.Status == StatusError {
			otelSpan.SetStatus(codes.Error, span.Error)
		}
		if span.Status == StatusCancelled {
			otelSpan.SetStatus(codes.Error, "cancelled")
		}

		ended := started
		if span.EndedAt > 0 {
			ended = time.UnixMilli(span.EndedAt)
		}
		otelSpan.End(trace.WithTimestamp(ended))
		contexts[span.ID] = otelSpan.SpanContext()
	}
}

// Shutdown flushes buffered spans and stops the provider.
func (e *Exporter) Shutdown(ctx context.Context) error {
	if !e.Enabled() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return e.provider.Shutdown(ctx)
}

func spanName(span Span) string {
	if span.Name == "" {
		return string(span.Kind)
	}
	return fmt.Sprintf("%s %s", span.Kind, span.Name)
}

func spanAttributes(run Run, span Span) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("myaaw.run.id", run.ID),
		attribute.String("myaaw.span.kind", string(span.Kind)),
	}
	if run.Agent != "" {
		attrs = append(attrs, attribute.String("myaaw.agent", run.Agent))
	}
	if run.Channel != "" {
		attrs = append(attrs, attribute.String("myaaw.channel", run.Channel))
	}
	if span.StepIndex > 0 {
		attrs = append(attrs, attribute.Int("myaaw.step.index", span.StepIndex))
	}
	if span.RequestIndex > 0 {
		attrs = append(attrs, attribute.Int("myaaw.request.index", span.RequestIndex))
	}
	if span.Status != "" {
		attrs = append(attrs, attribute.String("myaaw.status", string(span.Status)))
	}
	if span.CostUSD > 0 {
		attrs = append(attrs, attribute.Float64("myaaw.cost_usd", span.CostUSD))
	}
	if span.FirstTokenMs > 0 {
		attrs = append(attrs, attribute.Int64("myaaw.ttft_ms", span.FirstTokenMs))
	}
	if span.Error != "" {
		attrs = append(attrs, attribute.String("myaaw.error", span.Error))
	}

	switch span.Kind {
	case KindRun:
		attrs = append(attrs, attribute.String("gen_ai.operation.name", "invoke_agent"))
	case KindLLM:
		system := run.Provider
		if system == "" {
			system = "unknown"
		}
		attrs = append(attrs,
			attribute.String("gen_ai.operation.name", "chat"),
			attribute.String("gen_ai.system", system),
			attribute.String("gen_ai.request.model", span.Name),
			attribute.Int("gen_ai.usage.input_tokens", span.InputTokens),
			attribute.Int("gen_ai.usage.output_tokens", span.OutputTokens),
		)
		if span.CacheHitTokens > 0 {
			attrs = append(attrs, attribute.Int("myaaw.usage.cached_input_tokens", span.CacheHitTokens))
		}
	case KindTool:
		attrs = append(attrs,
			attribute.String("gen_ai.operation.name", "execute_tool"),
			attribute.String("gen_ai.tool.name", span.Name),
		)
	}

	return attrs
}

func serviceName() string {
	if name := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME")); name != "" {
		return name
	}
	return "myaaw"
}

func envBoolDefault(name string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}
