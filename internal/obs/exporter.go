package obs

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
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

	mu sync.Mutex
	// contexts remembers the OTel span context of recently exported spans so a
	// child run can nest under the parent span that started it.
	contexts map[string]exportedSpanContext
	// pending holds child runs whose parent span has not been exported yet. A
	// subagent usually finishes before the parent turn does.
	pending map[string][]pendingExport
}

const (
	exportedContextTTL = time.Hour
	exportedPendingTTL = 10 * time.Minute
	exportedContextCap = 50_000
)

type exportedSpanContext struct {
	context trace.SpanContext
	at      time.Time
}

type pendingExport struct {
	run   Run
	spans []Span
	at    time.Time
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
		contexts: map[string]exportedSpanContext{},
		pending:  map[string][]pendingExport{},
	}, nil
}

// Enabled reports whether an endpoint was configured.
func (e *Exporter) Enabled() bool {
	return e != nil && e.enabled
}

// Export sends one run and its spans. Spans are emitted parent-first so the
// collector sees a connected trace. A child run waits until the parent span
// that started it has been exported, which keeps a subagent inside the trace of
// the turn that invoked it.
func (e *Exporter) Export(run Run, spans []Span) {
	if !e.Enabled() {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[obs] otel export panicked: %v", recovered)
		}
	}()

	e.mu.Lock()
	defer e.mu.Unlock()

	e.flushStaleLocked()

	if run.ParentRunID != "" && run.ParentSpanID != "" {
		if _, ok := e.contexts[run.ParentSpanID]; !ok {
			e.pending[run.ParentRunID] = append(e.pending[run.ParentRunID], pendingExport{
				run: run, spans: spans, at: time.Now(),
			})
			return
		}
	}
	e.exportLocked(run, spans)
}

func (e *Exporter) exportLocked(run Run, spans []Span) {
	contexts := make(map[string]trace.SpanContext, len(spans))
	now := time.Now()
	for _, span := range spans {
		parentCtx := context.Background()
		if parent, ok := contexts[span.ParentID]; ok {
			parentCtx = trace.ContextWithSpanContext(parentCtx, parent)
		} else if parent, ok := e.contexts[span.ParentID]; ok {
			parentCtx = trace.ContextWithSpanContext(parentCtx, parent.context)
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
		spanContext := otelSpan.SpanContext()
		contexts[span.ID] = spanContext
		e.contexts[span.ID] = exportedSpanContext{context: spanContext, at: now}
	}

	// The parent is in the collector now, so any child that was waiting for it
	// can be attached to the real span context.
	children := e.pending[run.ID]
	delete(e.pending, run.ID)
	for _, child := range children {
		e.exportLocked(child.run, child.spans)
	}
}

// flushStaleLocked is the safety valve for a child whose parent never exports
// (the parent was killed, or the run ended without a trace). Such a child is
// exported as its own root rather than being dropped.
func (e *Exporter) flushStaleLocked() {
	now := time.Now()
	for parent, children := range e.pending {
		kept := children[:0]
		for _, child := range children {
			if now.Sub(child.at) < exportedPendingTTL {
				kept = append(kept, child)
				continue
			}
			log.Printf("[obs] exporting run %s without its parent %s; the parent span was never exported", child.run.ID, parent)
			e.exportLocked(child.run, child.spans)
		}
		if len(kept) == 0 {
			delete(e.pending, parent)
			continue
		}
		e.pending[parent] = kept
	}

	if len(e.contexts) > exportedContextCap {
		log.Printf("[obs] dropping %d cached span contexts", len(e.contexts))
		e.contexts = map[string]exportedSpanContext{}
		return
	}
	for id, entry := range e.contexts {
		if now.Sub(entry.at) > exportedContextTTL {
			delete(e.contexts, id)
		}
	}
}

// Shutdown flushes buffered spans and stops the provider. Runs still waiting
// for a parent are exported as roots so their data is not lost.
func (e *Exporter) Shutdown(ctx context.Context) error {
	if !e.Enabled() {
		return nil
	}
	e.mu.Lock()
	for parent, children := range e.pending {
		for _, child := range children {
			log.Printf("[obs] exporting run %s at shutdown without its parent %s", child.run.ID, parent)
			e.exportLocked(child.run, child.spans)
		}
	}
	e.pending = map[string][]pendingExport{}
	e.mu.Unlock()

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
