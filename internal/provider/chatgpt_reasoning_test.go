package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"myaaw/internal/auth/chatgpt"
)

func chatGPTReasoningItem(id string, texts ...string) map[string]interface{} {
	summary := make([]interface{}, 0, len(texts))
	for _, text := range texts {
		summary = append(summary, map[string]interface{}{"type": "summary_text", "text": text})
	}
	return map[string]interface{}{"type": "reasoning", "id": id, "summary": summary, "encrypted_content": "opaque-secret"}
}

func chatGPTReasoningEvent(kind string, output, summary int, text string) map[string]interface{} {
	ev := map[string]interface{}{"type": "response." + kind, "output_index": output,
		"summary_index": summary, "item_id": fmt.Sprintf("rs-%d", output)}
	switch {
	case strings.HasPrefix(kind, "reasoning_summary_part."):
		ev["part"] = map[string]interface{}{"type": "summary_text", "text": text}
	case strings.HasSuffix(kind, ".delta"):
		ev["delta"] = text
	default:
		ev["text"] = text
	}
	return ev
}

func chatGPTReasoningFrames(events ...map[string]interface{}) string {
	var wire strings.Builder
	for _, ev := range events {
		data, err := json.Marshal(ev)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(&wire, "data: %s\n\n", data)
	}
	return wire.String()
}

func TestChatGPTReasoningRequestPolicy(t *testing.T) {
	for _, model := range []string{"gpt-5.2-codex", "gpt-5.4", "gpt-6.1-sol", "o3", "o4-mini", "gpt-4o", "gpt-4.1", "gpt-5-chat-latest", "unknown-model"} {
		t.Run(model, func(t *testing.T) {
			want := model == "gpt-5.2-codex" || model == "gpt-5.4" || model == "gpt-6.1-sol" || model == "o3" || model == "o4-mini"
			p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				var body map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != model || body["store"] != false || body["stream"] != true || !reflect.DeepEqual(body["include"], []interface{}{"reasoning.encrypted_content"}) {
					t.Errorf("request invariants lost: %#v", body)
				}
				reasoning, exists := body["reasoning"]
				if want && !reflect.DeepEqual(reasoning, map[string]interface{}{"summary": "auto"}) {
					t.Errorf("reasoning = %#v; want summary:auto only, without forcing effort", reasoning)
				} else if !want && exists {
					t.Errorf("nonreasoning model received reasoning: %#v", reasoning)
				}
				chatGPTTestCompleted(w, chatGPTTestOutput("answer"))
			})
			if _, err := p.Chat(model, nil, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestChatGPTReasoningSummarySources(t *testing.T) {
	added := func(index int, item map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"type": "response.output_item.added", "output_index": index, "item": item}
	}
	done := func(index int, item map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"type": "response.output_item.done", "output_index": index, "item": item}
	}
	for _, tc := range []struct {
		name   string
		events []map[string]interface{}
		output []interface{}
		parts  []string
	}{
		{"delta done item terminal dedup", []map[string]interface{}{
			added(0, chatGPTReasoningItem("rs-0")),
			chatGPTReasoningEvent("reasoning_summary_part.added", 0, 0, ""),
			chatGPTReasoningEvent("reasoning_summary_text.delta", 0, 0, "Inspect "),
			chatGPTReasoningEvent("reasoning_summary_text.delta", 0, 0, "closely"),
			chatGPTReasoningEvent("reasoning_summary_text.done", 0, 0, "Inspect closely"),
			chatGPTReasoningEvent("reasoning_summary_part.done", 0, 0, "Inspect closely"),
			done(0, chatGPTReasoningItem("rs-0", "Inspect closely")),
		}, []interface{}{chatGPTReasoningItem("rs-0", "Inspect closely")}, []string{"Inspect closely"}},
		{"done only", []map[string]interface{}{
			added(0, chatGPTReasoningItem("rs-0")),
			chatGPTReasoningEvent("reasoning_summary_text.done", 0, 0, "Only done"),
		}, nil, []string{"Only done"}},
		{"done extends delta", []map[string]interface{}{
			added(0, chatGPTReasoningItem("rs-0")),
			chatGPTReasoningEvent("reasoning_summary_text.delta", 0, 0, "Inspect "),
			chatGPTReasoningEvent("reasoning_summary_text.done", 0, 0, "Inspect closely"),
		}, nil, []string{"Inspect closely"}},
		{"part snapshots without deltas", []map[string]interface{}{
			added(0, chatGPTReasoningItem("rs-0")),
			chatGPTReasoningEvent("reasoning_summary_part.added", 0, 0, "Part "),
			chatGPTReasoningEvent("reasoning_summary_part.done", 0, 0, "Part snapshot"),
		}, nil, []string{"Part snapshot"}},
		{"part done only", []map[string]interface{}{
			added(0, chatGPTReasoningItem("rs-0")),
			chatGPTReasoningEvent("reasoning_summary_part.done", 0, 0, "Part done"),
		}, nil, []string{"Part done"}},
		{"added item snapshot", []map[string]interface{}{added(0, chatGPTReasoningItem("rs-0", "Added snapshot"))}, nil, []string{"Added snapshot"}},
		{"done item snapshot", []map[string]interface{}{done(0, chatGPTReasoningItem("rs-0", "Done snapshot"))}, nil, []string{"Done snapshot"}},
		{"terminal only", nil, []interface{}{chatGPTReasoningItem("rs-0", "Final summary")}, []string{"Final summary"}},
		{"equal parts keep identity and order", []map[string]interface{}{
			added(0, chatGPTReasoningItem("rs-0")),
			chatGPTReasoningEvent("reasoning_summary_text.delta", 0, 0, "Same"),
			chatGPTReasoningEvent("reasoning_summary_text.done", 0, 0, "Same"),
			chatGPTReasoningEvent("reasoning_summary_part.done", 0, 1, "Same"),
			added(1, chatGPTReasoningItem("rs-1")),
			chatGPTReasoningEvent("reasoning_summary_text.done", 1, 0, "Same"),
			chatGPTReasoningEvent("reasoning_summary_text.done", 1, 1, "Last"),
		}, []interface{}{chatGPTReasoningItem("rs-0", "Same", "Same"), chatGPTReasoningItem("rs-1", "Same", "Last")}, []string{"Same", "Same", "Same", "Last"}},
		{"terminal multipart ordering", nil, []interface{}{chatGPTReasoningItem("rs-0", "First", "Second"), chatGPTReasoningItem("rs-1", "Third")}, []string{"First", "Second", "Third"}},
		{"encrypted and tokens are not thought", nil, []interface{}{chatGPTReasoningItem("rs-0")}, nil},
		{"absent summary is not thought", nil, []interface{}{map[string]interface{}{"type": "reasoning", "id": "rs-0", "encrypted_content": "opaque-secret"}}, nil},
		{"tokens alone are not thought", nil, chatGPTTestOutput("Answer"), nil},
		{"anonymous summary compatibility", []map[string]interface{}{{"type": "response.reasoning_summary_text.delta", "delta": "Anonymous"}}, nil, []string{"Anonymous"}},
		{"legacy raw delta compatibility", []map[string]interface{}{{"type": "response.reasoning_text.delta", "delta": "Legacy"}}, nil, []string{"Legacy"}},
		{"raw and summary have separate identities", []map[string]interface{}{
			added(0, chatGPTReasoningItem("rs-0")),
			chatGPTReasoningEvent("reasoning_text.delta", 0, 0, "Detailed reasoning"),
			chatGPTReasoningEvent("reasoning_text.done", 0, 0, "Detailed reasoning"),
			chatGPTReasoningEvent("reasoning_summary_text.delta", 0, 0, "Brief summary"),
			chatGPTReasoningEvent("reasoning_summary_text.done", 0, 0, "Brief summary"),
		}, []interface{}{chatGPTReasoningItem("rs-0", "Brief summary")}, []string{"Detailed reasoning", "Brief summary"}},
		{"summary then raw have separate identities", []map[string]interface{}{
			added(0, chatGPTReasoningItem("rs-0")),
			chatGPTReasoningEvent("reasoning_summary_text.delta", 0, 0, "Brief summary"),
			chatGPTReasoningEvent("reasoning_text.delta", 0, 0, "Detailed reasoning"),
			chatGPTReasoningEvent("reasoning_text.done", 0, 0, "Detailed reasoning"),
			chatGPTReasoningEvent("reasoning_summary_text.done", 0, 0, "Brief summary"),
		}, []interface{}{chatGPTReasoningItem("rs-0", "Brief summary")}, []string{"Brief summary", "Detailed reasoning"}},
		{"interleaved raw and summary preserve delivery order", []map[string]interface{}{
			added(0, chatGPTReasoningItem("rs-0")),
			chatGPTReasoningEvent("reasoning_text.delta", 0, 0, "Detailed "),
			chatGPTReasoningEvent("reasoning_summary_text.delta", 0, 0, "Brief summary"),
			chatGPTReasoningEvent("reasoning_text.delta", 0, 0, "reasoning"),
			chatGPTReasoningEvent("reasoning_text.done", 0, 0, "Detailed reasoning"),
			chatGPTReasoningEvent("reasoning_summary_text.done", 0, 0, "Brief summary"),
		}, []interface{}{chatGPTReasoningItem("rs-0", "Brief summary")}, []string{"Detailed ", "Brief summary", "reasoning"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, chatGPTReasoningFrames(tc.events...))
				chatGPTTestCompleted(w, tc.output)
			})
			var thought strings.Builder
			dones := 0
			err := p.ChatStreamEvents(context.Background(), "gpt-5.4", nil, nil, func(ev StreamEvent) error {
				if ev.Type == StreamThinkingDelta {
					thought.WriteString(ev.Thought)
				}
				if ev.Type == StreamDone {
					dones++
					if ev.Err != nil || ev.StopReason != StopNormal || ev.Message == nil {
						t.Errorf("invalid successful completion: %+v", ev)
					} else if ev.Message.Thought != thought.String() {
						t.Errorf("final Thought %q != streamed %q", ev.Message.Thought, thought.String())
					}
					if ev.Usage.ThoughtsTokens != 2 {
						t.Errorf("reasoning usage lost: %+v", ev.Usage)
					}
				}
				return nil
			})
			if err != nil || dones != 1 {
				t.Fatalf("err=%v, StreamDone count=%d", err, dones)
			}
			got, want := thought.String(), strings.Join(tc.parts, "\n")
			if got != want && got != strings.Join(tc.parts, "\n\n") {
				t.Errorf("visible summary = %q, want ordered parts %q", got, tc.parts)
			}
		})
	}
}

func TestChatGPTReasoningMessageAndContinuation(t *testing.T) {
	item := chatGPTReasoningItem("rs-0", "Visible summary")
	item["summary"].([]interface{})[0].(map[string]interface{})["extra"] = map[string]interface{}{"retain": true}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var output chatGPTOutputItem
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	message, err := chatGPTMessage([]chatGPTOutputItem{output}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if message.Thought != "Visible summary" {
		t.Errorf("projected Thought = %q", message.Thought)
	}
	var saved chatGPTContinuation
	if err := json.Unmarshal(message.ProviderMetadata[chatgpt.ProviderType], &saved); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(saved.Output)
	if err != nil {
		t.Fatal(err)
	}
	var got, want interface{}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(append(append([]byte{'['}, data...), ']'), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("continuation changed raw summary or ciphertext: %s", encoded)
	}
	p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(body["input"], want) {
			t.Errorf("continuation replay changed raw summary or ciphertext: %#v", body["input"])
		}
		chatGPTTestCompleted(w, chatGPTTestOutput("Answer"))
	})
	if _, err := p.Chat("gpt-5.4", []Message{message}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestChatGPTReasoningInterruptedAndCallbackFailure(t *testing.T) {
	for _, callbackFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("callback=%t", callbackFailure), func(t *testing.T) {
			p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_item.added", "output_index": 0, "item": chatGPTReasoningItem("rs-0")})
				chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_item.added", "output_index": 1,
					"item": map[string]interface{}{"type": "function_call", "id": "fc-1", "call_id": "call-1", "name": "lookup", "arguments": `{"unfinished":`}})
				chatGPTTestSSE(w, chatGPTReasoningEvent("reasoning_summary_text.delta", 0, 0, "Partial summary"))
			})
			callbackErr := errors.New("thinking callback stopped")
			dones := 0
			var visible strings.Builder
			err := p.ChatStreamEvents(context.Background(), "gpt-5.4", nil, nil, func(ev StreamEvent) error {
				if ev.Type == StreamThinkingDelta {
					visible.WriteString(ev.Thought)
					if callbackFailure {
						return callbackErr
					}
				}
				if ev.Type == StreamDone {
					dones++
					if ev.Err == nil || ev.StopReason != StopError || ev.Message == nil {
						t.Errorf("missing error completion: %+v", ev)
					} else if !ev.Message.IsError || ev.Message.Thought != visible.String() || len(ev.Message.ToolCalls) != 0 {
						t.Errorf("partial thought lost or unfinished tool leaked: %+v", ev.Message)
					}
				}
				return nil
			})
			if err == nil || dones != 1 || visible.String() != "Partial summary" || (callbackFailure && !errors.Is(err, callbackErr)) {
				t.Fatalf("err=%v, done=%d, visible=%q", err, dones, visible.String())
			}
		})
	}
}

func TestChatGPTReasoningRejectsInvalidSummaryIdentity(t *testing.T) {
	for _, kind := range []string{"reasoning_summary_text.delta", "reasoning_summary_text.done", "reasoning_summary_part.added", "reasoning_summary_part.done"} {
		for _, invalid := range []string{"negative", "huge", "mismatching item"} {
			t.Run(kind+"/"+invalid, func(t *testing.T) {
				ev := chatGPTReasoningEvent(kind, 0, 0, "Must not leak")
				switch invalid {
				case "negative":
					ev["summary_index"] = -1
				case "huge":
					ev["summary_index"] = 1 << 30
				case "mismatching item":
					ev["item_id"] = "different-reasoning-item"
				}
				wire := chatGPTReasoningFrames(
					map[string]interface{}{"type": "response.output_item.added", "output_index": 0, "item": chatGPTReasoningItem("rs-0")}, ev,
					map[string]interface{}{"type": "response.completed", "response": map[string]interface{}{"status": "completed"}})
				visible := ""
				message, err := consumeChatGPTSSE(strings.NewReader(wire), nil, "", func(ev StreamEvent) error {
					if ev.Type == StreamThinkingDelta {
						visible += ev.Thought
					}
					return nil
				})
				if err == nil || visible != "" || message.Thought != "" {
					t.Errorf("invalid summary accepted: err=%v, visible=%q, Thought=%q", err, visible, message.Thought)
				}
			})
		}
	}
}
