package obs

import (
	"context"
	"testing"
	"time"
)

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
