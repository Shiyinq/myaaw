package agent

import (
	"context"
	"encoding/json"
	"reflect"

	"myaaw/internal/provider"
)

// CloneMessages makes independently mutable transcript copies, including opaque
// provider state. In particular, a request transform or persistence hook must
// not mutate the content/continuation pair in the live transcript.
func CloneMessages(messages []provider.Message) []provider.Message {
	if messages == nil {
		return nil
	}
	out := make([]provider.Message, len(messages))
	for i, msg := range messages {
		msg.Content = cloneMessageValue(msg.Content)
		msg.Images = append([]string(nil), msg.Images...)
		msg.Uploads = append([]provider.Upload(nil), msg.Uploads...)
		msg.Attachments = append([]provider.MessageAttachment(nil), msg.Attachments...)
		msg.Trace = append([]provider.ReactStep(nil), msg.Trace...)
		msg.Usage.Details = append([]provider.UsageDetail(nil), msg.Usage.Details...)
		if msg.CreatedAt != nil {
			created := *msg.CreatedAt
			msg.CreatedAt = &created
		}
		if msg.ToolCalls != nil {
			msg.ToolCalls = append([]provider.ToolCall{}, msg.ToolCalls...)
			for j := range msg.ToolCalls {
				call := &msg.ToolCalls[j]
				call.Function.Arguments = cloneMessageValue(call.Function.Arguments)
				if call.Index != nil {
					index := *call.Index
					call.Index = &index
				}
			}
		}
		if msg.ProviderMetadata != nil {
			metadata := make(map[string]json.RawMessage, len(msg.ProviderMetadata))
			for key, raw := range msg.ProviderMetadata {
				metadata[key] = append(json.RawMessage(nil), raw...)
			}
			msg.ProviderMetadata = metadata
		}
		out[i] = msg
	}
	return out
}

func cloneMessageValue(value interface{}) interface{} {
	switch value := value.(type) {
	case []provider.ContentItem:
		items := append([]provider.ContentItem{}, value...)
		for i := range items {
			if items[i].ImageURL != nil {
				image := *items[i].ImageURL
				items[i].ImageURL = &image
			}
		}
		return items
	case []interface{}:
		items := make([]interface{}, len(value))
		for i, item := range value {
			items[i] = cloneMessageValue(item)
		}
		return items
	case map[string]interface{}:
		items := make(map[string]interface{}, len(value))
		for key, item := range value {
			items[key] = cloneMessageValue(item)
		}
		return items
	case json.RawMessage:
		return append(json.RawMessage(nil), value...)
	default:
		return value
	}
}

func transformMessages(ctx context.Context, messages []provider.Message, transform func(context.Context, []provider.Message) []provider.Message) []provider.Message {
	out := transform(ctx, CloneMessages(messages))
	for i := range out {
		if len(out[i].ProviderMetadata) == 0 {
			continue
		}
		// Transforms may reorder or remove messages, and older assistant turns
		// have no stable ID. Match the original opaque state and its paired
		// payload, rather than relying on a slice index or a display-only ID.
		unchanged := false
		for _, original := range messages {
			if original.Role == out[i].Role &&
				reflect.DeepEqual(original.ProviderMetadata, out[i].ProviderMetadata) &&
				reflect.DeepEqual(original.Content, out[i].Content) &&
				reflect.DeepEqual(original.ToolCalls, out[i].ToolCalls) {
				unchanged = true
				break
			}
		}
		if !unchanged {
			out[i].ProviderMetadata = nil
		}
	}
	return out
}

// Provider continuation state is private to its originating provider. Filter
// only request copies: persisted history can still be resumed after switching
// back, without exposing encrypted state to a different provider.
func messagesForProvider(messages []provider.Message, name string) []provider.Message {
	out := CloneMessages(messages)
	for i := range out {
		raw := out[i].ProviderMetadata[name]
		out[i].ProviderMetadata = nil
		if out[i].Role == "assistant" && len(raw) > 0 {
			out[i].ProviderMetadata = map[string]json.RawMessage{name: raw}
		}
	}
	return out
}
