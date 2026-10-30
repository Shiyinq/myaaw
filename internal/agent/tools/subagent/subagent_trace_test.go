package subagent

import (
	"context"
	"path/filepath"
	"testing"

	"myaaw/internal/agent"
	"myaaw/internal/obs"
	"myaaw/internal/obs/agenttrace"
)

func TestStartTraceNestsUnderTheParentRun(t *testing.T) {
	store, err := obs.Open(filepath.Join(t.TempDir(), "observability.db"), obs.Options{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	parent := agenttrace.Start(store, obs.NewPricer(nil), obs.Config{StoreContent: true}, obs.RunStart{
		Channel: "telegram", Agent: "default", UserMessage: "delegate this",
	})
	defer parent.Close()

	parent.Event(agent.AgentEvent{Type: agent.EventStepStart, StepIndex: 1})
	parentSpan := parent.CurrentSpanID()
	if parentSpan == "" {
		t.Fatal("parent has no current span")
	}

	ctx := agenttrace.WithTrace(context.Background(), parent)
	tool := NewSubAgentTool()
	child := tool.startTrace(ctx, SubAgentTask{Name: "researcher", Instruction: "find it"}, "you are a sub-agent")
	if child == nil {
		t.Fatal("startTrace returned nil for a traced run")
	}
	defer child.Close()

	if child.RunID() == "" || child.RunID() == parent.RunID() {
		t.Fatalf("child run id = %q", child.RunID())
	}

	child.End(agent.RunResult{})
	if err := child.Flush(context.Background()); err != nil {
		t.Fatalf("flush child: %v", err)
	}

	childRun, _, err := store.GetRun(child.RunID())
	if err != nil {
		t.Fatalf("get child run: %v", err)
	}
	if childRun.ParentRunID != parent.RunID() {
		t.Fatalf("parent run id = %q, want %q", childRun.ParentRunID, parent.RunID())
	}
	if childRun.ParentSpanID != parentSpan {
		t.Fatalf("parent span id = %q, want %q", childRun.ParentSpanID, parentSpan)
	}
	if childRun.Channel != "subagent" || childRun.Agent != "researcher" {
		t.Fatalf("child run = %+v", childRun)
	}
}

func TestStartTraceWithoutParentIsNil(t *testing.T) {
	tool := NewSubAgentTool()
	if trace := tool.startTrace(context.Background(), SubAgentTask{Name: "x"}, ""); trace != nil {
		t.Fatal("an untraced run must not create a child trace")
	}
}
