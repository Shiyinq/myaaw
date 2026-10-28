package agent

import (
	"context"
	"testing"

	"myaaw/internal/agent/tools"
	"myaaw/internal/provider"
)

type contextProbeTool struct {
	callID string
	marker string
}

func (p *contextProbeTool) CallTool(arguments string, tctx *tools.ToolsContext) string {
	return p.CallToolContext(context.Background(), arguments, tctx)
}

func (p *contextProbeTool) CallToolContext(ctx context.Context, _ string, _ *tools.ToolsContext) string {
	p.callID = ToolCallIDFromContext(ctx)
	p.marker, _ = ctx.Value(probeMarkerKey{}).(string)
	return "probe ok"
}

func (p *contextProbeTool) ToolDefinition() []byte {
	return []byte(`{"type":"function","function":{"name":"context_probe","parameters":{"type":"object","properties":{"x":{"type":"string"}}}}}`)
}

type probeMarkerKey struct{}

func TestToolExecutionExposesTheCallIDInContext(t *testing.T) {
	probe := &contextProbeTool{}
	tools.Register("context_probe", probe)
	t.Cleanup(func() { tools.ClearRegistryKey("context_probe") })

	prov := &scriptedProvider{turns: []provider.Message{
		{
			Role:      "assistant",
			ToolCalls: []provider.ToolCall{toolCall("call_ctx", "context_probe", `{"x":"1"}`)},
		},
		{Role: "assistant", Content: "done"},
	}}

	a := NewAgent(prov)
	ctx := context.WithValue(context.Background(), probeMarkerKey{}, "kept")
	if _, err := a.RunEvents(ctx, RunConfig{Model: "test-model", MaxIterations: 3},
		[]provider.Message{{Role: "user", Content: "go"}}, nil); err != nil {
		t.Fatalf("run: %v", err)
	}

	if probe.callID != "call_ctx" {
		t.Fatalf("tool saw call id %q, want call_ctx", probe.callID)
	}
	if probe.marker != "kept" {
		t.Fatalf("tool ctx lost the caller's values: %q", probe.marker)
	}
}

func TestToolCallIDFromContextIsEmptyWithoutACall(t *testing.T) {
	if got := ToolCallIDFromContext(context.Background()); got != "" {
		t.Fatalf("call id = %q, want empty", got)
	}
	if got := ToolCallIDFromContext(nil); got != "" {
		t.Fatalf("call id = %q, want empty", got)
	}
}
