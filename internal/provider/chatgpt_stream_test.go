package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"myaaw/internal/auth/chatgpt"
)

const chatGPTPartialText = "data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"message\",\"id\":\"msg-1\",\"role\":\"assistant\",\"content\":[]}}\n\ndata: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"item_id\":\"msg-1\",\"delta\":\"partial\"}\n\n"

func TestChatGPTStreamFailuresHaveOneDoneAndNoRetry(t *testing.T) {
	for _, tc := range []struct{ name, ending, contains string }{
		{"EOF", "", "without response.completed"},
		{"done sentinel", "data: [DONE]\n\n", "without response.completed"},
		{"malformed", "data: {not JSON}\n\n", "invalid ChatGPT SSE"},
		{"failed", "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_error\",\"message\":\"failed now\"}}}\n\n", "failed now"},
		{"quota after text", "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"subscription_sharing_usage_limit_exceeded\",\"message\":\"quota\"}}}\n\n", "https://chatgpt.com/settings/usage"},
		{"usage check after text", "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"subscription_sharing_usage_unavailable\"}}}\n\n", "Keep your credential"},
		{"incomplete", "data: {\"type\":\"response.incomplete\",\"response\":{\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n", "incomplete: max_output_tokens"},
		{"explicit error", "event: error\ndata: {\"code\":\"server_error\",\"message\":\"explicit failure\"}\n\n", "explicit failure"},
		{"nested error", "data: {\"type\":\"error\",\"error\":{\"code\":\"server_error\",\"message\":\"nested failure\"}}\n\n", "nested failure"},
		{"truncated completed frame", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}", "without response.completed"},
		{"invalid completed status", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"failed\"}}\n\n", "non-completed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("x-request-id", "stream-req")
				fmt.Fprint(w, chatGPTPartialText+tc.ending)
			})
			var events []StreamEvent
			err := p.ChatStreamEvents(context.Background(), "", nil, nil, func(ev StreamEvent) error { events = append(events, ev); return nil })
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("wrong failure: %v", err)
			}
			if requests != 1 {
				t.Fatalf("retried partial stream %d times", requests)
			}
			if len(events) != 2 || events[0].Type != StreamTextDelta || events[0].Text != "partial" || events[1].Type != StreamDone {
				t.Fatalf("wrong lifecycle: %+v", events)
			}
			done := events[1]
			if done.Err == nil || done.StopReason != StopError || done.Message == nil || !done.Message.IsError || MessageText(*done.Message) != "partial" || len(done.Message.ToolCalls) != 0 {
				t.Fatalf("partial output or error lost: %+v", done)
			}
		})
	}
}

func TestChatGPTMultilineSSEAndCompletedUsage(t *testing.T) {
	p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": heartbeat\r\n\r\nevent: response.output_item.added\r\ndata: {\"output_index\": 0,\r\ndata: \"item\": {\"type\":\"message\",\"role\":\"assistant\",\"id\":\"msg-1\",\"content\":[]}}\r\n\r\n")
		chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_text.delta", "output_index": 0, "item_id": "msg-1", "delta": "Hello"})
		chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_text.delta", "output_index": 0, "item_id": "msg-1", "delta": ", world"})
		chatGPTTestSSE(w, map[string]interface{}{"type": "response.reasoning_summary_text.delta", "delta": "A summary"})
		chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_text.done", "output_index": 0, "item_id": "msg-1", "text": "Hello, world"})
		chatGPTTestCompleted(w, chatGPTTestOutput("Hello, world"))
	})
	var events []StreamEvent
	err := p.ChatStreamEvents(context.Background(), "", nil, nil, func(ev StreamEvent) error { events = append(events, ev); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 {
		t.Fatalf("wrong event count: %+v", events)
	}
	done := events[len(events)-1]
	if done.Type != StreamDone || done.StopReason != StopNormal || done.Err != nil || MessageText(*done.Message) != "Hello, world" || done.Message.Thought != "A summary" || done.Usage.TotalTokens != 15 {
		t.Fatalf("bad completed message: %+v", done)
	}
}

func TestChatGPTToolArgumentAccumulationAndNamespace(t *testing.T) {
	p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_item.added", "output_index": 0, "item": map[string]interface{}{"type": "function_call", "id": "fc-1", "call_id": "call-1", "name": "myaaw.lookup", "arguments": ""}})
		chatGPTTestSSE(w, map[string]interface{}{"type": "response.function_call_arguments.delta", "output_index": 0, "item_id": "fc-1", "delta": `{"a":`})
		chatGPTTestSSE(w, map[string]interface{}{"type": "response.function_call_arguments.delta", "output_index": 0, "item_id": "fc-1", "delta": `1}`})
		// Omitted final output exercises accumulated items, not just final snapshots.
		chatGPTTestSSE(w, map[string]interface{}{"type": "response.completed", "response": map[string]interface{}{"status": "completed"}})
	})
	message, err := p.Chat("", nil, []map[string]interface{}{{"type": "function", "function": map[string]interface{}{"name": "lookup"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(message.ToolCalls) != 1 || message.ToolCalls[0].Function.Name != "lookup" || ToolCallArgumentsString(message.ToolCalls[0].Function.Arguments) != `{"a":1}` {
		t.Fatalf("tool assembly failed: %+v", message)
	}
}

func TestChatGPTExactlyOneDoneOnLocalAndCallbackErrors(t *testing.T) {
	p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, chatGPTPartialText)
		chatGPTTestCompleted(w, chatGPTTestOutput("partial"))
	})
	for _, tc := range []string{"missing model", "token failure", "callback failure", "canceled"} {
		t.Run(tc, func(t *testing.T) {
			copy := *p
			ctx := context.Background()
			want := errors.New("test callback failure")
			switch tc {
			case "missing model":
				copy.defaultModel = ""
			case "token failure":
				copy.tokenSource = func(context.Context, string) (string, error) { return "", errors.New("credential removed") }
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			doneCount := 0
			err := copy.ChatStreamEvents(ctx, "", nil, nil, func(ev StreamEvent) error {
				if ev.Type == StreamDone {
					doneCount++
					if ev.Err == nil {
						t.Error("missing terminal error")
					}
					if tc == "canceled" && ev.StopReason != StopAborted {
						t.Errorf("canceled stop = %s", ev.StopReason)
					}
				}
				if tc == "callback failure" && ev.Type == StreamTextDelta {
					return want
				}
				return nil
			})
			if err == nil || doneCount != 1 {
				t.Fatalf("err=%v, done=%d", err, doneCount)
			}
			if tc == "callback failure" && !errors.Is(err, want) {
				t.Fatalf("lost callback error: %v", err)
			}
		})
	}
}

func TestChatGPTReasoningDoneContinuation(t *testing.T) {
	p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_item.added", "output_index": 0,
			"item": map[string]interface{}{"type": "reasoning", "id": "rs-1", "encrypted_content": "incomplete-added-ciphertext", "summary": []interface{}{}}})
		chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_item.done", "output_index": 0,
			"item": map[string]interface{}{"type": "reasoning", "id": "rs-1", "encrypted_content": "complete-done-ciphertext", "summary": []interface{}{}}})
		chatGPTTestCompleted(w, []interface{}{
			map[string]interface{}{"type": "reasoning", "id": "rs-1", "summary": []interface{}{}},
			map[string]interface{}{"type": "function_call", "id": "fc-1", "call_id": "call-1", "name": "myaaw.lookup", "arguments": `{}`},
		})
	})
	message, err := p.Chat("", nil, []map[string]interface{}{{"type": "function", "function": map[string]interface{}{"name": "lookup"}}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := chatGPTInput([]Message{message, {Role: "tool", ToolCallID: "call-1", Content: "result"}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "complete-done-ciphertext") || strings.Contains(string(encoded), "incomplete-added-ciphertext") || !strings.Contains(string(encoded), `"namespace":"myaaw"`) || strings.Contains(string(encoded), `"name":"myaaw.lookup"`) {
		t.Fatalf("bad continuation: %s", encoded)
	}
	message.Content = "edited transcript"
	if _, err := chatGPTInput([]Message{message, {Role: "tool", ToolCallID: "call-1", Content: "result"}}); err == nil {
		t.Fatal("stale continuation metadata silently replayed")
	}
}

func TestChatGPTCompletedWithEmptyOutputUsesStreamedItems(t *testing.T) {
	// The Responses API terminates with "output": [] even though the stream
	// already delivered every item. Tool calls and reasoning continuation only
	// exist in those streamed items, so an empty terminal snapshot must fall back
	// to them instead of reporting a successful empty turn.
	for _, tc := range []struct {
		name  string
		item  map[string]interface{}
		check func(*testing.T, Message, string)
	}{
		{
			name: "function call",
			item: map[string]interface{}{"type": "function_call", "id": "fc-1", "call_id": "call-1", "name": "bash", "namespace": "myaaw", "arguments": `{"command":"sleep 3"}`},
			check: func(t *testing.T, message Message, input string) {
				if len(message.ToolCalls) != 1 || message.ToolCalls[0].Function.Name != "bash" || ToolCallArgumentsString(message.ToolCalls[0].Function.Arguments) != `{"command":"sleep 3"}` {
					t.Fatalf("streamed tool call lost: %+v", message)
				}
				if !strings.Contains(input, `"call_id":"call-1"`) {
					t.Fatalf("tool call missing from continuation: %s", input)
				}
			},
		},
		{
			name: "message",
			item: map[string]interface{}{"type": "message", "id": "msg-1", "role": "assistant", "content": []interface{}{map[string]interface{}{"type": "output_text", "text": "Hi, I am Vivy.", "annotations": []interface{}{}}}},
			check: func(t *testing.T, message Message, input string) {
				if MessageText(message) != "Hi, I am Vivy." {
					t.Fatalf("streamed text lost: %+v", message)
				}
				if !strings.Contains(input, `"text":"Hi, I am Vivy."`) {
					t.Fatalf("message missing from continuation: %s", input)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_item.added", "output_index": 0,
					"item": map[string]interface{}{"type": tc.item["type"], "id": tc.item["id"], "status": "in_progress"}})
				chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_item.done", "output_index": 0, "item": tc.item})
				chatGPTTestCompleted(w, []interface{}{})
			})
			message, err := p.Chat("", nil, []map[string]interface{}{{"type": "function", "function": map[string]interface{}{"name": "bash"}}})
			if err != nil {
				t.Fatal(err)
			}
			if len(message.ProviderMetadata[chatgpt.ProviderType]) == 0 {
				t.Fatal("streamed continuation was not preserved")
			}
			input, err := chatGPTInput(append([]Message{message}, continuationResults(message)...))
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, message, string(encoded))
		})
	}
}

// continuationResults closes the pending tool calls of a replayed assistant
// turn, so chatGPTInput accepts the continuation they carry.
func continuationResults(message Message) []Message {
	results := make([]Message, 0, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		results = append(results, Message{Role: "tool", Name: call.Function.Name, ToolCallID: call.ID, Content: "ok"})
	}
	return results
}

func TestChatGPTEmptyOutputDoesNotAdvertiseUnfinishedItems(t *testing.T) {
	p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_item.added", "output_index": 0,
			"item": map[string]interface{}{"type": "function_call", "id": "fc-1", "status": "in_progress", "call_id": "call-1", "name": "bash", "namespace": "myaaw", "arguments": ""}})
		chatGPTTestSSE(w, map[string]interface{}{"type": "response.function_call_arguments.delta", "output_index": 0, "item_id": "fc-1", "delta": `{"comm`})
		chatGPTTestCompleted(w, []interface{}{})
	})
	if _, err := p.Chat("", nil, []map[string]interface{}{{"type": "function", "function": map[string]interface{}{"name": "bash"}}}); err == nil || !strings.Contains(err.Error(), "unfinished output item") {
		t.Fatalf("unfinished streamed item accepted: %v", err)
	}
}

func TestChatGPTRejectsUnknownOrTruncatedTools(t *testing.T) {
	for _, item := range []chatGPTOutputItem{
		{Type: "function_call", CallID: "call-1", Namespace: "foreign", Name: "lookup", Arguments: `{}`},
		{Type: "function_call", CallID: "call-1", Namespace: "myaaw", Name: "unknown", Arguments: `{}`},
		{Type: "function_call", CallID: "call-1", Namespace: "myaaw", Name: "lookup", Arguments: `{"x":`},
		{Type: "function_call", Namespace: "myaaw", Name: "lookup", Arguments: `{}`},
		{Type: "tool_search_call"},
	} {
		if _, err := chatGPTMessage([]chatGPTOutputItem{item}, map[string]string{"lookup": "lookup"}, true); err == nil {
			t.Errorf("accepted invalid output: %+v", item)
		}
	}
}
