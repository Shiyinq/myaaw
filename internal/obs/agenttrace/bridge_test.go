package agenttrace

import (
	"context"
	"path/filepath"
	"testing"

	"myaaw/internal/agent"
	"myaaw/internal/obs"
	"myaaw/internal/provider"
)

func newStore(t *testing.T) *obs.Store {
	t.Helper()
	store, err := obs.Open(filepath.Join(t.TempDir(), "observability.db"), obs.Options{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestFromEventMapsEngineEvents(t *testing.T) {
	t.Run("llm request start carries the transcript", func(t *testing.T) {
		event, ok := FromEvent(agent.AgentEvent{
			Type:      agent.EventLLMRequestStart,
			StepIndex: 2,
			Model:     "deepseek-flash",
			Messages:  []provider.Message{{Role: "user", Content: "hi"}},
		})
		if !ok {
			t.Fatal("event was dropped")
		}
		if event.Type != obs.EventLLMRequestStart || event.Model != "deepseek-flash" || event.StepIndex != 2 {
			t.Fatalf("event = %+v", event)
		}
		if len(event.Messages) != 1 {
			t.Fatalf("messages = %d", len(event.Messages))
		}
	})

	t.Run("message end carries usage and content", func(t *testing.T) {
		message := provider.Message{
			Role:       "assistant",
			Content:    "answer",
			Thought:    "thinking",
			StopReason: provider.StopNormal,
			Usage:      provider.Usage{PromptTokens: 10, CompletionTokens: 5},
		}
		event, ok := FromEvent(agent.AgentEvent{Type: agent.EventMessageEnd, Message: &message})
		if !ok {
			t.Fatal("event was dropped")
		}
		if event.Type != obs.EventLLMEnd || event.Output != "answer" || event.Thought != "thinking" {
			t.Fatalf("event = %+v", event)
		}
		if event.Usage.PromptTokens != 10 || event.StopReason != provider.StopNormal {
			t.Fatalf("usage/stop = %+v / %q", event.Usage, event.StopReason)
		}
	})

	t.Run("tool end carries result and error state", func(t *testing.T) {
		result := agent.ToolResult{
			Content:    []provider.ContentItem{{Type: "text", Text: "boom"}},
			DurationMs: 12,
			IsError:    true,
		}
		event, ok := FromEvent(agent.AgentEvent{
			Type:       agent.EventToolExecutionEnd,
			ToolCallID: "call_1",
			ToolName:   "bash",
			ToolResult: &result,
		})
		if !ok {
			t.Fatal("event was dropped")
		}
		if event.Type != obs.EventToolEnd || event.ToolOutput != "boom" || event.ToolDurationMs != 12 {
			t.Fatalf("event = %+v", event)
		}
		if !event.IsError || event.ErrorText == "" {
			t.Fatalf("error state = %+v", event)
		}
	})

	t.Run("first delta marks the first token", func(t *testing.T) {
		event, ok := FromEvent(agent.AgentEvent{Type: agent.EventMessageUpdate, Delta: "h"})
		if !ok || event.Type != obs.EventFirstToken {
			t.Fatalf("event = %+v ok = %v", event, ok)
		}
		if _, ok := FromEvent(agent.AgentEvent{Type: agent.EventMessageUpdate}); ok {
			t.Fatal("an empty update must not count as a token")
		}
	})

	t.Run("unrelated events are ignored", func(t *testing.T) {
		for _, eventType := range []agent.EventType{agent.EventTraceUpdate, agent.EventUsage, agent.EventAgentStart} {
			if _, ok := FromEvent(agent.AgentEvent{Type: eventType}); ok {
				t.Fatalf("event %d should be ignored", eventType)
			}
		}
	})
}

func TestEndEventMapsStatus(t *testing.T) {
	cancelled := EndEvent(agent.RunResult{StopReason: provider.StopAborted, Err: context.Canceled})
	if cancelled.Status != obs.StatusCancelled || cancelled.Err == "" {
		t.Fatalf("cancelled = %+v", cancelled)
	}

	failed := EndEvent(agent.RunResult{StopReason: provider.StopError, Err: context.DeadlineExceeded})
	if failed.Status != obs.StatusError {
		t.Fatalf("failed status = %q", failed.Status)
	}

	completed := EndEvent(agent.RunResult{StopReason: provider.StopNormal})
	if completed.Status != obs.StatusCompleted || completed.Err != "" {
		t.Fatalf("completed = %+v", completed)
	}
}

func TestSystemPrompt(t *testing.T) {
	messages := []provider.Message{
		{Role: "system", Content: "prompt"},
		{Role: "user", Content: "hi"},
	}
	if got := SystemPrompt(messages); got != "prompt" {
		t.Fatalf("system prompt = %q", got)
	}
	if got := SystemPrompt([]provider.Message{{Role: "user", Content: "hi"}}); got != "" {
		t.Fatalf("system prompt = %q, want empty", got)
	}
}

func TestTraceRoundTripsThroughContext(t *testing.T) {
	store := newStore(t)
	trace := Start(store, obs.NewPricer(nil), obs.Config{StoreContent: true}, obs.RunStart{Channel: "cli"})
	defer trace.Close()

	ctx := WithTrace(context.Background(), trace)
	if got := FromContext(ctx); got != trace {
		t.Fatal("trace did not round trip through the context")
	}
	if FromContext(context.Background()) != nil {
		t.Fatal("a bare context must not carry a trace")
	}
}

func TestStartChildNestsSubagentRun(t *testing.T) {
	store := newStore(t)
	parent := Start(store, obs.NewPricer(nil), obs.Config{StoreContent: true}, obs.RunStart{
		Channel: "telegram", Agent: "default", Model: "deepseek-flash",
		UserMessage: "research this",
	})
	defer parent.Close()

	// The parent's tool call is the anchor the child run hangs under.
	parent.Event(agent.AgentEvent{Type: agent.EventStepStart, StepIndex: 1})
	parent.Event(agent.AgentEvent{
		Type: agent.EventToolExecutionStart, StepIndex: 1,
		ToolCallID: "call_sub", ToolName: "subagent", ToolArguments: `{"task":"x"}`,
	})
	toolSpanID := parent.ToolSpanID("call_sub")
	if toolSpanID == "" {
		t.Fatal("tool span id was not recorded")
	}

	child := parent.StartChild(obs.RunStart{
		Channel: "subagent", Agent: "researcher", Model: "deepseek-flash",
		UserMessage: "x", ParentSpanID: toolSpanID,
	})
	if child == nil {
		t.Fatal("StartChild returned nil")
	}
	child.Event(agent.AgentEvent{Type: agent.EventStepStart, StepIndex: 1})
	child.Event(agent.AgentEvent{
		Type: agent.EventLLMRequestStart, StepIndex: 1, Model: "deepseek-flash",
		Messages: []provider.Message{{Role: "user", Content: "x"}},
	})
	child.Event(agent.AgentEvent{
		Type: agent.EventMessageEnd, StepIndex: 1,
		Message: &provider.Message{
			Role: "assistant", Content: "answer", StopReason: provider.StopNormal,
			Usage: provider.Usage{PromptTokens: 1000, CompletionTokens: 100, TotalTokens: 1100},
		},
	})
	child.Event(agent.AgentEvent{Type: agent.EventStepEnd, StepIndex: 1})
	child.End(agent.RunResult{
		StopReason: provider.StopNormal,
		Usage:      provider.Usage{PromptTokens: 1000, CompletionTokens: 100, TotalTokens: 1100},
	})
	if err := child.Close(); err != nil {
		t.Fatalf("close child: %v", err)
	}

	if err := parent.Flush(context.Background()); err != nil {
		t.Fatalf("flush parent: %v", err)
	}

	parentRun, spans, err := store.GetRun(parent.RunID())
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}
	if parentRun.Status != obs.StatusRunning {
		t.Fatalf("parent status = %q", parentRun.Status)
	}

	// A one-level-deep read must include the nested run's spans.
	var childRunSpan *obs.Span
	for i := range spans {
		if spans[i].Kind == obs.KindRun && spans[i].ID != parent.RunID() {
			childRunSpan = &spans[i]
		}
	}
	if childRunSpan == nil {
		t.Fatal("the nested run span is missing from the parent trace")
	}
	if childRunSpan.ParentID != toolSpanID {
		t.Fatalf("nested run parent = %q, want the tool span %q", childRunSpan.ParentID, toolSpanID)
	}

	childRun, childSpans, err := store.GetRun(childRunSpan.ID)
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	if childRun.ParentRunID != parent.RunID() || childRun.ParentSpanID != toolSpanID {
		t.Fatalf("child run links = %q / %q", childRun.ParentRunID, childRun.ParentSpanID)
	}
	if childRun.Status != obs.StatusCompleted {
		t.Fatalf("child status = %q", childRun.Status)
	}
	if len(childSpans) == 0 {
		t.Fatal("child run has no spans")
	}
}

func TestNilTraceIsSafe(t *testing.T) {
	var trace *Trace
	trace.Event(agent.AgentEvent{Type: agent.EventStepStart})
	trace.End(agent.RunResult{})
	if err := trace.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if trace.RunID() != "" || trace.ToolSpanID("x") != "" {
		t.Fatal("nil trace must report empty ids")
	}
	if trace.StartChild(obs.RunStart{}) != nil {
		t.Fatal("nil trace cannot start a child")
	}
}
