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

func TestTimelineDoneIncludesUsage(t *testing.T) {
	timeline, events := collectTimeline()

	timeline.Done(&channel.OutgoingMessage{
		Text: "final answer",
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
	if data["ok"] != true || data["text"] != "final answer" {
		t.Errorf("done data = %#v", data)
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
