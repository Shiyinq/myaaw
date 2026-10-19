package api

import (
	"myaaw/internal/channel"
)

// SSEEvent is one Server-Sent Event emitted while the agent runs.
type SSEEvent struct {
	Name string
	Data any
}

// TimelineItem is one entry of the agent timeline. Tool results are attached to
// their tool item so clients can render the same thought/tool/answer sequence as
// the TUI and Telegram timelines.
type TimelineItem struct {
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Result    string `json:"result,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Timeline converts agent stream chunks into timeline items and, when emit is
// set, into Server-Sent Events. Pass nil emit for non-streaming responses.
type Timeline struct {
	items []TimelineItem
	emit  func(SSEEvent)
}

func NewTimeline(emit func(SSEEvent)) *Timeline {
	return &Timeline{emit: emit}
}

// Items returns the accumulated timeline.
func (t *Timeline) Items() []TimelineItem {
	return t.items
}

// HandleChunk is the onChunk callback for BotStreamContext.
func (t *Timeline) HandleChunk(chunk channel.StreamChunk) {
	switch chunk.Kind {
	case "thought", "thinking":
		t.appendText("thought", chunk.Thought)
	case "text":
		t.appendText("text", chunk.Text)
	case "tool":
		t.startTool(chunk)
	case "tool_result":
		t.finishTool(chunk)
	case "final", "usage":
		// Delivered by Done.
	case "trace":
		// The timeline already exposes thought and tool lifecycle events.
	default:
		// Backwards compatible with providers that only populate fields.
		if chunk.Thought != "" {
			t.appendText("thought", chunk.Thought)
		} else if chunk.Text != "" {
			t.appendText("text", chunk.Text)
		} else {
			t.startTool(chunk)
		}
	}
}

// Done finalizes the timeline with the authoritative answer and emits the
// closing event. Matching the TUI and Telegram timelines, the final answer is
// the last text item rather than a separate element.
func (t *Timeline) Done(out *channel.OutgoingMessage) {
	if out == nil {
		return
	}

	t.setFinalText(out.Text)

	if t.emit == nil {
		return
	}

	data := map[string]any{
		"ok":   true,
		"text": out.Text,
	}
	if out.Usage.TotalTokens > 0 {
		data["usage"] = map[string]int{
			"prompt_tokens":     out.Usage.PromptTokens,
			"completion_tokens": out.Usage.CompletionTokens,
			"total_tokens":      out.Usage.TotalTokens,
		}
	}
	t.emit(SSEEvent{Name: "done", Data: data})
}

func (t *Timeline) appendText(kind, delta string) {
	if delta == "" {
		return
	}

	event := "chunk"
	if kind == "thought" {
		event = "thought"
	}

	if n := len(t.items); n > 0 && t.items[n-1].Kind == kind {
		t.items[n-1].Text += delta
		t.emitEvent(event, map[string]string{"text": delta})
		return
	}

	t.items = append(t.items, TimelineItem{Kind: kind, Text: delta})
	t.emitEvent(event, map[string]string{"text": delta})
}

func (t *Timeline) startTool(chunk channel.StreamChunk) {
	name, id := chunk.ToolName, chunk.ToolCallID
	if len(chunk.ToolCalls) > 0 {
		if name == "" {
			name = chunk.ToolCalls[0].Function.Name
		}
		if id == "" {
			id = chunk.ToolCalls[0].ID
		}
	}
	if name == "" && id == "" {
		return
	}

	t.items = append(t.items, TimelineItem{
		Kind:      "tool",
		ID:        id,
		Name:      name,
		Arguments: chunk.ToolArguments,
	})
	t.emitEvent("tool", map[string]string{
		"id":        id,
		"name":      name,
		"arguments": chunk.ToolArguments,
	})
}

func (t *Timeline) finishTool(chunk channel.StreamChunk) {
	attached := false
	for i := len(t.items) - 1; i >= 0; i-- {
		item := &t.items[i]
		if item.Kind != "tool" || item.ID != chunk.ToolCallID {
			continue
		}
		item.Result = chunk.ToolResultText
		item.IsError = chunk.IsError
		attached = true
		break
	}

	if !attached {
		t.items = append(t.items, TimelineItem{
			Kind:    "tool_result",
			ID:      chunk.ToolCallID,
			Name:    chunk.ToolName,
			Result:  chunk.ToolResultText,
			IsError: chunk.IsError,
		})
	}

	t.emitEvent("tool_result", map[string]any{
		"id":       chunk.ToolCallID,
		"name":     chunk.ToolName,
		"result":   chunk.ToolResultText,
		"is_error": chunk.IsError,
	})
}

func (t *Timeline) emitEvent(name string, data any) {
	if t.emit == nil {
		return
	}
	t.emit(SSEEvent{Name: name, Data: data})
}

// setFinalText replaces the most recent text item with the authoritative final
// answer, or appends one if the answer was not streamed as text deltas. Text
// before the last tool call is intermediate and is kept.
func (t *Timeline) setFinalText(finalText string) {
	if finalText == "" {
		return
	}

	lastText, lastTool := -1, -1
	for i := len(t.items) - 1; i >= 0; i-- {
		if lastText == -1 && t.items[i].Kind == "text" {
			lastText = i
		}
		if lastTool == -1 && t.items[i].Kind == "tool" {
			lastTool = i
		}
		if lastText != -1 && lastTool != -1 {
			break
		}
	}

	if lastText != -1 && lastText > lastTool {
		t.items[lastText].Text = finalText
		return
	}
	t.items = append(t.items, TimelineItem{Kind: "text", Text: finalText})
}
