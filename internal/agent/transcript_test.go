package agent

import (
	"strings"
	"testing"

	"myaaw/internal/provider"
)

func TestMigrateLegacyTranscriptExpandsTraceOnce(t *testing.T) {
	legacy := []provider.Message{
		{Role: "user", Content: "hello"},
		{
			Role:    "assistant",
			Content: "final answer",
			Trace: []provider.ReactStep{
				{Thought: "think", Action: "first", ActionInput: `{"x":"1"}`, Observation: "one", StepGroup: 1},
				{Thought: "think", Action: "second", ActionInput: `{"x":"2"}`, Observation: "two", StepGroup: 1},
			},
		},
	}

	migrated := MigrateLegacyTranscript(legacy)
	if len(migrated) != 5 {
		t.Fatalf("migrated length = %d, want 5", len(migrated))
	}
	if migrated[0].Role != "user" {
		t.Fatalf("migrated[0] role = %q", migrated[0].Role)
	}
	if migrated[1].Role != "assistant" || len(migrated[1].ToolCalls) != 2 {
		t.Fatalf("migrated[1] is not an assistant with 2 tool calls: %+v", migrated[1])
	}
	if migrated[2].Role != "tool" || migrated[2].ToolCallID != migrated[1].ToolCalls[0].ID {
		t.Fatalf("first tool result does not match first tool call")
	}
	if migrated[3].Role != "tool" || migrated[3].ToolCallID != migrated[1].ToolCalls[1].ID {
		t.Fatalf("second tool result does not match second tool call")
	}
	if migrated[1].ToolCalls[0].ID == migrated[1].ToolCalls[1].ID {
		t.Fatal("synthetic tool call IDs are not unique")
	}
	if migrated[4].Role != "assistant" || migrated[4].Content != "final answer" {
		t.Fatalf("final assistant answer was not preserved")
	}
	for i, msg := range migrated {
		if msg.Version != transcriptVersion {
			t.Fatalf("migrated[%d] version = %d", i, msg.Version)
		}
	}

	again := MigrateLegacyTranscript(migrated)
	if len(again) != len(migrated) {
		t.Fatalf("migration is not idempotent: %d -> %d", len(migrated), len(again))
	}
}

func TestCompactTranscriptKeepsSystemAndLatestTurn(t *testing.T) {
	big := strings.Repeat("x", 2000)
	messages := []provider.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "old question " + big},
		{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "old-1", Type: "function", Function: provider.FunctionCall{Name: "bash", Arguments: `{"x":"1"}`}}}},
		{Role: "tool", ToolCallID: "old-1", Name: "bash", Content: "old output " + big},
		{Role: "assistant", Content: "old answer " + big},
		{Role: "user", Content: "new question"},
		{Role: "assistant", Content: "new answer"},
	}

	compacted := CompactTranscript(messages, 50)
	if len(compacted) != 3 {
		t.Fatalf("compacted length = %d, want 3: %+v", len(compacted), compacted)
	}
	if compacted[0].Role != "system" {
		t.Fatalf("system message was dropped")
	}
	if compacted[1].Role != "user" || compacted[1].Content != "new question" {
		t.Fatalf("latest user turn was not kept")
	}
	if compacted[2].Role != "assistant" || compacted[2].Content != "new answer" {
		t.Fatalf("latest assistant turn was not kept")
	}
}

func TestCompactTranscriptNeverSplitsToolPairs(t *testing.T) {
	big := strings.Repeat("x", 200)
	messages := []provider.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "question " + big},
		{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "c1", Type: "function", Function: provider.FunctionCall{Name: "bash", Arguments: `{"x":"1"}`}}}},
		{Role: "tool", ToolCallID: "c1", Name: "bash", Content: "result " + big},
		{Role: "assistant", Content: "answer"},
		{Role: "user", Content: "latest"},
	}

	// Budget that fits the first turn but not both; compaction walks backward
	// and must keep the assistant/tool pair together when it includes the turn.
	compacted := CompactTranscript(messages, 120)
	seenTool := false
	seenCall := false
	for _, msg := range compacted {
		if msg.Role == "tool" {
			seenTool = true
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			seenCall = true
		}
	}
	if seenTool != seenCall {
		t.Fatalf("compaction split a tool call/result pair: %+v", compacted)
	}
}
