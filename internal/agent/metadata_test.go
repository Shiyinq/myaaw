package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"myaaw/internal/provider"
)

const continuationKey = "openai_chatgpt"

func continuationMetadata() map[string]json.RawMessage {
	return map[string]json.RawMessage{continuationKey: json.RawMessage(`{"output":[{"type":"reasoning","id":"rs_1","encrypted_content":"opaque-encrypted-state","summary":[]}]}`)}
}

type continuationProvider struct{ scriptedProvider }

func (*continuationProvider) ProviderName() string { return continuationKey }

func TestHistoryToolContinuationPreservesMetadata(t *testing.T) {
	registerFakeTool(t, "test_metadata", &fakeTool{run: func(string) string { return "result" }})
	old := provider.Message{Role: "assistant", Content: "old answer", ProviderMetadata: continuationMetadata(), Version: 2}
	call := provider.Message{Role: "assistant", ToolCalls: []provider.ToolCall{toolCall("call_1", "test_metadata", `{"x":"1"}`)}, ProviderMetadata: continuationMetadata()}
	answer := provider.Message{Role: "assistant", Content: "new answer", ProviderMetadata: continuationMetadata()}
	p := &continuationProvider{scriptedProvider: scriptedProvider{turns: []provider.Message{call, answer}}}
	history := []provider.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: strings.Repeat("discard ", 100)},
		{Role: "assistant", Content: "discard"},
		{Role: "user", Content: "previous"}, old,
		{Role: "user", Content: "continue"},
	}
	var checkpoints [][]provider.Message
	result, err := NewAgent(p).RunEvents(context.Background(), RunConfig{
		Model: "test-model", MaxIterations: 4, ContextMaxTokens: 200,
		TransformContext: func(_ context.Context, messages []provider.Message) []provider.Message { return messages },
		OnCheckpoint: func(_ context.Context, messages []provider.Message) {
			checkpoints = append(checkpoints, CloneMessages(messages))
			// Even an accidentally mutating persistence hook cannot corrupt the run.
			for i := range messages {
				for key, raw := range messages[i].ProviderMetadata {
					raw[0] = '!'
					delete(messages[i].ProviderMetadata, key)
				}
			}
		},
	}, history, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.requests) != 2 {
		t.Fatalf("requests = %d", len(p.requests))
	}
	if len(p.requests[0]) >= len(history) {
		t.Fatal("test did not exercise context truncation")
	}
	for _, request := range p.requests {
		for _, msg := range request {
			if msg.Role == "assistant" && (provider.MessageText(msg) == "old answer" || len(msg.ToolCalls) > 0) {
				if !reflect.DeepEqual(msg.ProviderMetadata, continuationMetadata()) {
					t.Fatal("history continuation lost")
				}
			}
		}
	}
	second := p.requests[1]
	if second[len(second)-1].Role != "tool" || len(second[len(second)-2].ProviderMetadata) == 0 {
		t.Fatal("tool continuation did not retain its paired assistant metadata")
	}
	for _, messages := range [][]provider.Message{result.Transcript, result.NewMessages} {
		for _, msg := range messages {
			if msg.Role == "assistant" && (len(msg.ToolCalls) > 0 || provider.MessageText(msg) == "new answer") && !reflect.DeepEqual(msg.ProviderMetadata, continuationMetadata()) {
				t.Fatal("result lost continuation")
			}
		}
	}
	if !reflect.DeepEqual(result.Final.ProviderMetadata, continuationMetadata()) {
		t.Fatal("final metadata lost")
	}
	sawCall := false
	for _, snapshot := range checkpoints {
		for _, msg := range snapshot {
			if len(msg.ToolCalls) > 0 {
				sawCall = true
				if !reflect.DeepEqual(msg.ProviderMetadata, continuationMetadata()) {
					t.Fatal("checkpoint lost continuation")
				}
			}
		}
	}
	if !sawCall {
		t.Fatal("no tool checkpoint")
	}
	if !reflect.DeepEqual(old.ProviderMetadata, continuationMetadata()) {
		t.Fatal("input history mutated")
	}
}

func TestTransformDropsMetadataOnlyForModifiedPairs(t *testing.T) {
	for _, change := range []string{"none", "content", "arguments", "call_id", "remove_calls", "role"} {
		t.Run(change, func(t *testing.T) {
			original := []provider.Message{{Role: "assistant", Content: "answer", ToolCalls: []provider.ToolCall{{ID: "c1", Function: provider.FunctionCall{Name: "tool", Arguments: map[string]interface{}{"x": "old"}}}}, ProviderMetadata: continuationMetadata()}}
			out := transformMessages(context.Background(), original, func(_ context.Context, messages []provider.Message) []provider.Message {
				switch change {
				case "content":
					messages[0].Content = "edited"
				case "arguments":
					messages[0].ToolCalls[0].Function.Arguments.(map[string]interface{})["x"] = "edited"
				case "call_id":
					messages[0].ToolCalls[0].ID = "edited"
				case "remove_calls":
					messages[0].ToolCalls = nil
				case "role":
					messages[0].Role = "user"
				}
				return messages
			})
			if got := len(out[0].ProviderMetadata) > 0; got != (change == "none") {
				t.Fatalf("metadata retained = %v", got)
			}
			if provider.MessageText(original[0]) != "answer" || original[0].ToolCalls[0].ID != "c1" || original[0].ToolCalls[0].Function.Arguments.(map[string]interface{})["x"] != "old" || len(original[0].ProviderMetadata) == 0 {
				t.Fatal("transform changed live history")
			}
		})
	}
}

func TestAgentDoesNotLeakMetadataToAnotherProvider(t *testing.T) {
	history := []provider.Message{{Role: "assistant", Content: "saved", Version: 2, ProviderMetadata: continuationMetadata()}, {Role: "user", Content: "next"}}
	p := &scriptedProvider{turns: []provider.Message{{Role: "assistant", Content: "done"}}}
	result, err := NewAgent(p).RunEvents(context.Background(), RunConfig{Model: "test", MaxIterations: 2}, history, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.requests[0][0].ProviderMetadata) != 0 {
		t.Fatal("foreign provider received continuation state")
	}
	if len(result.Transcript[0].ProviderMetadata) == 0 {
		t.Fatal("provider switch destroyed persisted continuation")
	}
}

func TestCloneMessagesOwnsMetadataAndPairedContent(t *testing.T) {
	original := []provider.Message{{Role: "assistant", Content: []provider.ContentItem{{Type: "image_url", ImageURL: &provider.ImageInfo{URL: "original"}}}, ProviderMetadata: continuationMetadata()}}
	copied := CloneMessages(original)
	copied[0].ProviderMetadata[continuationKey][0] = '!'
	copied[0].Content.([]provider.ContentItem)[0].ImageURL.URL = "edited"
	delete(copied[0].ProviderMetadata, continuationKey)
	if !reflect.DeepEqual(original[0].ProviderMetadata, continuationMetadata()) || original[0].Content.([]provider.ContentItem)[0].ImageURL.URL != "original" {
		t.Fatal("copy aliases original")
	}
}

func TestImageRewritesDropPairedMetadata(t *testing.T) {
	message := provider.Message{Role: "assistant", Content: []provider.ContentItem{{Type: "image_url", ImageURL: &provider.ImageInfo{URL: "data:image/png;base64,not base64"}}}, ProviderMetadata: continuationMetadata()}
	repaired := SanitizeImageDataURLs([]provider.Message{message})
	if len(repaired[0].ProviderMetadata) != 0 {
		t.Fatal("image repair retained stale metadata")
	}
	if len(message.ProviderMetadata) == 0 {
		t.Fatal("image repair modified original")
	}
	stripMessageImages(&message)
	if len(message.ProviderMetadata) != 0 {
		t.Fatal("image eviction retained stale metadata")
	}
}

func TestToolIDNormalizationDropsMetadata(t *testing.T) {
	registerFakeTool(t, "test_metadata_id", &fakeTool{run: func(string) string { return "result" }})
	p := &continuationProvider{scriptedProvider: scriptedProvider{turns: []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{toolCall("", "test_metadata_id", `{"x":"1"}`)}, ProviderMetadata: continuationMetadata()},
		{Role: "assistant", Content: "done"},
	}}}
	result, err := NewAgent(p).RunEvents(context.Background(), RunConfig{Model: "test", MaxIterations: 3}, []provider.Message{{Role: "user", Content: "go"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.NewMessages[0].ProviderMetadata) != 0 || len(p.requests[1][1].ProviderMetadata) != 0 {
		t.Fatal("rewritten tool ID retained stale continuation")
	}
}
