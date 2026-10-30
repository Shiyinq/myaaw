package agent

import (
	"context"
	"testing"

	"myaaw/internal/provider"
)

func TestCheckpointsIncludeUserMessageAndToolResults(t *testing.T) {
	registerFakeTool(t, "test_checkpoint", &fakeTool{
		run: func(args string) string { return "checkpoint-result" },
	})

	prov := &scriptedProvider{turns: []provider.Message{
		{
			Role:      "assistant",
			ToolCalls: []provider.ToolCall{toolCall("c1", "test_checkpoint", `{"x":"1"}`)},
		},
		{
			Role:    "assistant",
			Content: "final answer",
		},
	}}

	var snapshots [][]provider.Message
	a := NewAgent(prov)
	_, err := a.RunEvents(context.Background(), RunConfig{
		Model:         "test-model",
		MaxIterations: 5,
		OnCheckpoint: func(_ context.Context, transcript []provider.Message) {
			snapshot := make([]provider.Message, len(transcript))
			copy(snapshot, transcript)
			snapshots = append(snapshots, snapshot)
		},
	}, []provider.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("RunEvents: %v", err)
	}

	if len(snapshots) < 3 {
		t.Fatalf("checkpoints = %d, want at least 3 (start, tool round, turn end)", len(snapshots))
	}

	first := snapshots[0]
	if len(first) != 1 || first[0].Role != "user" || first[0].Content != "hi" {
		t.Fatalf("first checkpoint = %+v, want just the user message", first)
	}

	var sawToolResult bool
	for _, snapshot := range snapshots {
		for _, message := range snapshot {
			if message.Role == "tool" && message.Content == "checkpoint-result" {
				sawToolResult = true
			}
		}
	}
	if !sawToolResult {
		t.Fatal("no checkpoint contained the tool result")
	}
}

func TestCheckpointPanicDoesNotBreakTheRun(t *testing.T) {
	prov := &scriptedProvider{turns: []provider.Message{
		{Role: "assistant", Content: "done"},
	}}

	a := NewAgent(prov)
	result, err := a.RunEvents(context.Background(), RunConfig{
		Model:         "test-model",
		MaxIterations: 3,
		OnCheckpoint: func(context.Context, []provider.Message) {
			panic("checkpoint exploded")
		},
	}, []provider.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("RunEvents: %v", err)
	}
	if provider.MessageText(result.Final) != "done" {
		t.Fatalf("final = %q", provider.MessageText(result.Final))
	}
}
