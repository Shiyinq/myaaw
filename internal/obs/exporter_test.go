package obs

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func newRecordingExporter() (*Exporter, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	return &Exporter{
		provider: provider,
		tracer:   provider.Tracer("myaaw/obs-test"),
		enabled:  true,
		contexts: map[string]exportedSpanContext{},
		pending:  map[string][]pendingExport{},
	}, recorder
}

func spanWithKind(t *testing.T, spans []sdktrace.ReadOnlySpan, runID, kind string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		var gotRun, gotKind string
		for _, attr := range span.Attributes() {
			switch attr.Key {
			case "myaaw.run.id":
				gotRun = attr.Value.AsString()
			case "myaaw.span.kind":
				gotKind = attr.Value.AsString()
			}
		}
		if gotRun == runID && gotKind == kind {
			return span
		}
	}
	t.Fatalf("no %s span for run %s in %d exported spans", kind, runID, len(spans))
	return nil
}

// A subagent usually finishes before its parent turn, so its export has to wait
// for the parent span to exist and then attach to it.
func TestExporterNestsChildRunWhenItFinishesFirst(t *testing.T) {
	exporter, recorder := newRecordingExporter()
	base := time.Now().UnixMilli()

	parentRun := Run{ID: "run_parent", StartedAt: base}
	parentSpans := []Span{
		{ID: "run_parent", Kind: KindRun, StartedAt: base, EndedAt: base + 1000},
		{ID: "sp_tool", RunID: "run_parent", ParentID: "run_parent", Kind: KindTool, Name: "subagent", StartedAt: base + 100, EndedAt: base + 900},
	}
	childRun := Run{ID: "run_child", ParentRunID: "run_parent", ParentSpanID: "sp_tool", StartedAt: base + 200, EndedAt: base + 800}
	childSpans := []Span{
		{ID: "run_child", Kind: KindRun, ParentID: "sp_tool", StartedAt: base + 200, EndedAt: base + 800},
		{ID: "sp_llm", RunID: "run_child", ParentID: "run_child", Kind: KindLLM, Name: "deepseek-flash", StartedAt: base + 300, EndedAt: base + 700},
	}

	exporter.Export(childRun, childSpans)
	if got := len(recorder.Ended()); got != 0 {
		t.Fatalf("child exported %d spans before its parent; want it queued", got)
	}

	exporter.Export(parentRun, parentSpans)
	ended := recorder.Ended()
	if len(ended) != 4 {
		t.Fatalf("exported spans = %d, want parent and child", len(ended))
	}

	toolSpan := spanWithKind(t, ended, "run_parent", "tool")
	childRunSpan := spanWithKind(t, ended, "run_child", "run")
	childLLM := spanWithKind(t, ended, "run_child", "llm")

	if childRunSpan.Parent().SpanID() != toolSpan.SpanContext().SpanID() {
		t.Fatal("child run is not nested under the parent tool span")
	}
	if childRunSpan.SpanContext().TraceID() != toolSpan.SpanContext().TraceID() {
		t.Fatal("child run ended up in a different trace")
	}
	if childLLM.Parent().SpanID() != childRunSpan.SpanContext().SpanID() {
		t.Fatal("child llm span is not nested under its run")
	}
}

func TestExporterNestsChildRunWhenParentFinishesFirst(t *testing.T) {
	exporter, recorder := newRecordingExporter()
	base := time.Now().UnixMilli()

	parentRun := Run{ID: "run_parent", StartedAt: base}
	parentSpans := []Span{
		{ID: "run_parent", Kind: KindRun, StartedAt: base, EndedAt: base + 1000},
		{ID: "sp_tool", RunID: "run_parent", ParentID: "run_parent", Kind: KindTool, Name: "subagent", StartedAt: base + 100, EndedAt: base + 900},
	}
	exporter.Export(parentRun, parentSpans)
	exporter.Export(Run{ID: "run_child", ParentRunID: "run_parent", ParentSpanID: "sp_tool"}, []Span{
		{ID: "run_child", Kind: KindRun, ParentID: "sp_tool", StartedAt: base + 200, EndedAt: base + 800},
	})

	ended := recorder.Ended()
	toolSpan := spanWithKind(t, ended, "run_parent", "tool")
	childRunSpan := spanWithKind(t, ended, "run_child", "run")
	if childRunSpan.Parent().SpanID() != toolSpan.SpanContext().SpanID() {
		t.Fatal("child run is not nested under the parent tool span")
	}
}

func TestExporterIsDisabledWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	exporter, err := NewExporter(context.Background())
	if err != nil {
		t.Fatalf("new exporter: %v", err)
	}
	if exporter.Enabled() {
		t.Fatal("exporter should be disabled without an endpoint")
	}

	// A disabled exporter must accept work without doing anything.
	exporter.Export(Run{ID: "run_1", StartedAt: time.Now().UnixMilli()}, []Span{
		{ID: "sp_1", Kind: KindRun, StartedAt: time.Now().UnixMilli()},
	})
	if err := exporter.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestSpanAttributesUseGenAIConventions(t *testing.T) {
	run := Run{ID: "run_1", Provider: "deepseek", Agent: "default", Channel: "telegram"}
	attrs := spanAttributes(run, Span{
		ID: "sp_1", Kind: KindLLM, Name: "deepseek-flash",
		InputTokens: 100, OutputTokens: 20, CostUSD: 0.001, FirstTokenMs: 300,
	})

	values := map[string]any{}
	for _, attr := range attrs {
		values[string(attr.Key)] = attr.Value.AsInterface()
	}

	expected := map[string]any{
		"gen_ai.operation.name":      "chat",
		"gen_ai.system":              "deepseek",
		"gen_ai.request.model":       "deepseek-flash",
		"gen_ai.usage.input_tokens":  int64(100),
		"gen_ai.usage.output_tokens": int64(20),
		"myaaw.run.id":               "run_1",
		"myaaw.span.kind":            "llm",
		"myaaw.step.index":           nil,
		"myaaw.cost_usd":             0.001,
		"myaaw.ttft_ms":              int64(300),
	}
	for key, want := range expected {
		if want == nil {
			continue
		}
		if got := values[key]; got != want {
			t.Fatalf("%s = %v, want %v", key, got, want)
		}
	}

	toolAttrs := spanAttributes(run, Span{ID: "sp_2", Kind: KindTool, Name: "bash"})
	toolValues := map[string]any{}
	for _, attr := range toolAttrs {
		toolValues[string(attr.Key)] = attr.Value.AsInterface()
	}
	if toolValues["gen_ai.tool.name"] != "bash" {
		t.Fatalf("tool name attribute = %v", toolValues["gen_ai.tool.name"])
	}
	if toolValues["gen_ai.operation.name"] != "execute_tool" {
		t.Fatalf("tool operation = %v", toolValues["gen_ai.operation.name"])
	}
}
