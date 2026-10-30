package provider

import (
	"context"
	"strings"
)

// StreamEventType identifies a delta or lifecycle event produced while an
// assistant response is being streamed.
type StreamEventType int

const (
	// StreamTextDelta carries a chunk of user-visible response text.
	StreamTextDelta StreamEventType = iota
	// StreamThinkingDelta carries a chunk of model reasoning.
	StreamThinkingDelta
	// StreamUsage carries token usage for the current provider request.
	StreamUsage
	// StreamDone is always the final event. It carries the fully assembled
	// assistant message and the stop reason for the turn.
	StreamDone
	// StreamAttempt is emitted before a retry, after the previous attempt failed
	// without delivering any output. It lets observers record each request.
	StreamAttempt
)

// StreamEvent is the provider-level event consumed by the agent loop.
//
// Providers must emit exactly one StreamDone event per request. Text and
// thinking deltas are advisory (used for UI); the authoritative message is
// carried by StreamDone.Message so callers never have to re-accumulate
// provider-specific chunks.
type StreamEvent struct {
	Type StreamEventType

	Text    string // StreamTextDelta
	Thought string // StreamThinkingDelta

	// Attempt is the 1-based request number on StreamAttempt (1 is the first
	// try, 2 the first retry). Err carries why the previous attempt failed.
	Attempt int

	Usage Usage // StreamUsage / StreamDone

	Message    *Message   // StreamDone
	StopReason StopReason // StreamDone
	Err        error      // StreamDone (StopError/StopAborted)
}

// chatStreamFromEvents adapts the rich StreamEvent API to the legacy
// callback-based ChatStream signature.
func chatStreamFromEvents(
	ctx context.Context,
	p LLMProvider,
	modelName string,
	messages []Message,
	tools []map[string]interface{},
	callback func(Message) error,
) error {
	sawText := false
	sawThought := false

	err := p.ChatStreamEvents(ctx, modelName, messages, tools, func(ev StreamEvent) error {
		switch ev.Type {
		case StreamTextDelta:
			if ev.Text == "" {
				return nil
			}
			sawText = true
			return callback(Message{Role: "assistant", Content: ev.Text})
		case StreamThinkingDelta:
			if ev.Thought == "" {
				return nil
			}
			sawThought = true
			return callback(Message{Role: "assistant", Thought: ev.Thought})
		case StreamUsage:
			if ev.Usage.TotalTokens == 0 {
				return nil
			}
			return callback(Message{Usage: ev.Usage})
		case StreamDone:
			if ev.Message != nil {
				final := *ev.Message
				if !sawText && MessageText(final) != "" {
					if err := callback(Message{Role: "assistant", Content: MessageText(final)}); err != nil {
						return err
					}
				}
				if !sawThought && final.Thought != "" {
					if err := callback(Message{Role: "assistant", Thought: final.Thought}); err != nil {
						return err
					}
				}
				if len(final.ToolCalls) > 0 {
					if err := callback(final); err != nil {
						return err
					}
				}
				if ev.Usage.TotalTokens == 0 {
					ev.Usage = final.Usage
				}
			}
			if ev.Usage.TotalTokens > 0 {
				if err := callback(Message{Usage: ev.Usage}); err != nil {
					return err
				}
			}
			if ev.Err != nil {
				return ev.Err
			}
		}
		return nil
	})

	return err
}

// MessageText extracts plain text from a message. It supports the two content
// shapes used by the codebase: a plain string and a list of ContentItem.
func MessageText(m Message) string {
	switch content := m.Content.(type) {
	case nil:
		return ""
	case string:
		return content
	case []ContentItem:
		var sb strings.Builder
		for _, item := range content {
			if item.Type == "text" {
				sb.WriteString(item.Text)
			}
		}
		return sb.String()
	case []interface{}:
		var sb strings.Builder
		for _, raw := range content {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if t, _ := item["type"].(string); t != "text" {
				continue
			}
			if text, _ := item["text"].(string); text != "" {
				sb.WriteString(text)
			}
		}
		return sb.String()
	default:
		return ""
	}
}

// MessageContentItems normalizes message content to ContentItem blocks.
func MessageContentItems(m Message) []ContentItem {
	switch content := m.Content.(type) {
	case nil:
		return nil
	case string:
		if content == "" {
			return nil
		}
		return []ContentItem{{Type: "text", Text: content}}
	case []ContentItem:
		return content
	case []interface{}:
		items := make([]ContentItem, 0, len(content))
		for _, raw := range content {
			block, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			item := ContentItem{}
			item.Type, _ = block["type"].(string)
			item.Text, _ = block["text"].(string)
			if url, ok := block["image_url"].(map[string]interface{}); ok {
				item.ImageURL = &ImageInfo{URL: stringValue(url["url"])}
			}
			items = append(items, item)
		}
		return items
	default:
		return nil
	}
}

func stringValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
