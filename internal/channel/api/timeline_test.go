package api

import (
	"testing"

	"myaaw/internal/channel"
	"myaaw/internal/provider"
)

func collectTimeline() (*Timeline, *[]SSEEvent) {
	var events []SSEEvent
	return NewTimeline(func(event SSEEvent) {
		events = append(events, event)
	}), &events
}

func eventNames(events []SSEEvent) []string {
	names := make([]string, 0, len(events))
	for _, event := range events {
		names = append(names, event.Name)
	}
	return names
}

func TestTimelineEmitsThoughtToolAndText(t *testing.T) {
	timeline, events := collectTimeline()

	timeline.HandleChunk(channel.StreamChunk{Kind: "thought", Thought: "checking files"})
	timeline.HandleChunk(channel.StreamChunk{
		Kind:          "tool",
		ToolCallID:    "call_1",
		ToolName:      "bash",
		ToolArguments: `{"command":"ls"}`,
	})
	timeline.HandleChunk(channel.StreamChunk{
		Kind:           "tool_result",
		ToolCallID:     "call_1",
		ToolName:       "bash",
		ToolResultText: "file1.txt",
	})
	timeline.HandleChunk(channel.StreamChunk{Kind: "text", Text: "done"})

	wantEvents := []string{"thought", "tool", "tool_result", "chunk"}
	gotEvents := eventNames(*events)
	if len(gotEvents) != len(wantEvents) {
		t.Fatalf("events = %v, want %v", gotEvents, wantEvents)
	}
	for i := range wantEvents {
		if gotEvents[i] != wantEvents[i] {
			t.Fatalf("events = %v, want %v", gotEvents, wantEvents)
		}
	}

	items := timeline.Items()
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3: %+v", len(items), items)
	}
	if items[0].Kind != "thought" || items[0].Text != "checking files" {
		t.Errorf("thought item = %+v", items[0])
	}
	if items[1].Kind != "tool" || items[1].ID != "call_1" || items[1].Name != "bash" {
		t.Errorf("tool item = %+v", items[1])
	}
	if items[1].Result != "file1.txt" || items[1].IsError {
		t.Errorf("tool result = %+v", items[1])
	}
	if items[2].Kind != "text" || items[2].Text != "done" {
		t.Errorf("text item = %+v", items[2])
	}
}

func TestTimelineCoalescesDeltasPerKind(t *testing.T) {
	timeline, _ := collectTimeline()

	timeline.HandleChunk(channel.StreamChunk{Kind: "thought", Thought: "a"})
	timeline.HandleChunk(channel.StreamChunk{Kind: "thought", Thought: "b"})
	timeline.HandleChunk(channel.StreamChunk{Kind: "text", Text: "c"})
	timeline.HandleChunk(channel.StreamChunk{Kind: "text", Text: "d"})

	items := timeline.Items()
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2: %+v", len(items), items)
	}
	if items[0].Kind != "thought" || items[0].Text != "ab" {
		t.Errorf("thought item = %+v", items[0])
	}
	if items[1].Kind != "text" || items[1].Text != "cd" {
		t.Errorf("text item = %+v", items[1])
	}
}

func TestTimelineAppendsUnmatchedToolResult(t *testing.T) {
	timeline, _ := collectTimeline()

	timeline.HandleChunk(channel.StreamChunk{
		Kind:           "tool_result",
		ToolCallID:     "call_2",
		ToolName:       "python",
		ToolResultText: "ok",
		IsError:        true,
	})

	items := timeline.Items()
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1: %+v", len(items), items)
	}
	if items[0].Kind != "tool_result" || items[0].Result != "ok" || !items[0].IsError {
		t.Errorf("tool_result item = %+v", items[0])
	}
}

func TestTimelineEmitsRetryAndCompaction(t *testing.T) {
	timeline, events := collectTimeline()

	timeline.HandleChunk(channel.StreamChunk{Kind: "retry", Attempt: 2})
	timeline.HandleChunk(channel.StreamChunk{
		Kind: "compaction", TokensBefore: 62400, TokensAfter: 38100, DroppedMessages: 14,
	})
	timeline.HandleChunk(channel.StreamChunk{Kind: "text", Text: "done"})

	names := eventNames(*events)
	if len(names) != 3 || names[0] != "retry" || names[1] != "compaction" || names[2] != "chunk" {
		t.Fatalf("events = %v", names)
	}

	items := timeline.Items()
	if len(items) != 3 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Kind != "retry" || items[0].Attempt != 2 {
		t.Fatalf("retry item = %+v", items[0])
	}
	if items[1].Kind != "compaction" || items[1].TokensBefore != 62400 || items[1].TokensAfter != 38100 || items[1].DroppedMessages != 14 {
		t.Fatalf("compaction item = %+v", items[1])
	}

	data, ok := (*events)[1].Data.(map[string]any)
	if !ok || data["tokens_before"] != 62400 || data["dropped_messages"] != 14 {
		t.Fatalf("compaction event = %#v", (*events)[1].Data)
	}
}

func TestTimelineDoneIncludesUsage(t *testing.T) {
	timeline, events := collectTimeline()

	timeline.Done(&channel.OutgoingMessage{
		Text:           "final answer",
		ConversationID: "conv-1",
		RunID:          "run_test",
		Usage: provider.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	})

	if len(*events) != 1 || (*events)[0].Name != "done" {
		t.Fatalf("events = %v, want one done event", eventNames(*events))
	}
	data, ok := (*events)[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("done data = %#v", (*events)[0].Data)
	}
	if data["ok"] != true || data["text"] != "final answer" || data["conversation_id"] != "conv-1" {
		t.Errorf("done data = %#v", data)
	}
	if data["run_id"] != "run_test" {
		t.Errorf("done run_id = %#v", data["run_id"])
	}
	usage, ok := data["usage"].(map[string]int)
	if !ok || usage["total_tokens"] != 15 {
		t.Errorf("usage = %#v", data["usage"])
	}
}

func TestTimelineDoneReplacesStreamedText(t *testing.T) {
	timeline, _ := collectTimeline()

	timeline.HandleChunk(channel.StreamChunk{Kind: "text", Text: "draft"})
	timeline.Done(&channel.OutgoingMessage{Text: "final answer"})

	items := timeline.Items()
	if len(items) != 1 || items[0].Kind != "text" || items[0].Text != "final answer" {
		t.Fatalf("items = %+v", items)
	}
}

func TestTimelineDoneKeepsIntermediateText(t *testing.T) {
	timeline, _ := collectTimeline()

	timeline.HandleChunk(channel.StreamChunk{Kind: "text", Text: "let me check"})
	timeline.HandleChunk(channel.StreamChunk{
		Kind:       "tool",
		ToolCallID: "call_1",
		ToolName:   "bash",
	})
	timeline.HandleChunk(channel.StreamChunk{
		Kind:       "tool_result",
		ToolCallID: "call_1",
		ToolName:   "bash",
	})
	timeline.Done(&channel.OutgoingMessage{Text: "final answer"})

	items := timeline.Items()
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3: %+v", len(items), items)
	}
	if items[0].Kind != "text" || items[0].Text != "let me check" {
		t.Errorf("intermediate text = %+v", items[0])
	}
	if items[1].Kind != "tool" {
		t.Errorf("tool = %+v", items[1])
	}
	if items[2].Kind != "text" || items[2].Text != "final answer" {
		t.Errorf("final text = %+v", items[2])
	}
}

func TestTimelineWithoutEmitterKeepsItems(t *testing.T) {
	timeline := NewTimeline(nil)

	timeline.HandleChunk(channel.StreamChunk{Kind: "text", Text: "hello"})
	timeline.Done(&channel.OutgoingMessage{Text: "hello"})

	items := timeline.Items()
	if len(items) != 1 || items[0].Text != "hello" {
		t.Fatalf("items = %+v", items)
	}
}

func TestTimelineEmitsApprovalEvents(t *testing.T) {
	timeline, events := collectTimeline()

	timeline.HandleChunk(channel.StreamChunk{
		Kind:        "approval_required",
		ApprovalID:  "appr-1",
		ToolName:    "bash",
		ToolAction:  "",
		ArgsSummary: `{"command":"ls"}`,
	})
	timeline.HandleChunk(channel.StreamChunk{
		Kind:       "approval_resolved",
		ApprovalID: "appr-1",
		Decision:   "allow_once",
	})

	names := eventNames(*events)
	if len(names) != 2 || names[0] != "approval_required" || names[1] != "approval_resolved" {
		t.Fatalf("events = %v", names)
	}

	items := timeline.Items()
	if len(items) != 1 || items[0].Kind != "approval" || items[0].ID != "appr-1" {
		t.Fatalf("items = %+v", items)
	}
}
