package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolCallArgumentsString(t *testing.T) {
	cases := []struct {
		name string
		args interface{}
		want string
	}{
		{name: "nil", args: nil, want: "{}"},
		{name: "empty string", args: "  ", want: "{}"},
		{name: "json string passed through", args: `{"action":"list"}`, want: `{"action":"list"}`},
		{
			name: "decoded map is encoded",
			args: map[string]interface{}{"action": "set_default", "name": "deepseek"},
			want: `{"action":"set_default","name":"deepseek"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToolCallArgumentsString(tc.args); got != tc.want {
				t.Fatalf("ToolCallArgumentsString(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// A transcript written by Gemini stores arguments as a decoded object. Replaying
// it against an OpenAI-compatible provider must send a JSON string, otherwise
// the request is rejected before the tool ever runs.
func TestMessagesToOpenAIMessagesNormalizeToolCalls(t *testing.T) {
	messages := []Message{{
		Role: "assistant",
		ToolCalls: []ToolCall{{
			ID:   "call_0",
			Type: "function",
			Function: FunctionCall{
				Name:             "provider",
				Arguments:        map[string]interface{}{"action": "set_default", "name": "deepseek"},
				ThoughtSignature: "gemini-signature",
			},
		}},
	}}

	encoded, err := json.Marshal(messagesToOpenAIMessages(messages))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(encoded)

	if !strings.Contains(body, `"arguments":"{\"action\":\"set_default\",\"name\":\"deepseek\"}"`) {
		t.Fatalf("arguments are not a JSON string: %s", body)
	}
	if strings.Contains(body, "thought_signature") {
		t.Fatalf("gemini thought signature leaked into the OpenAI payload: %s", body)
	}
}

func TestMessagesToOpenAIMessagesKeepStringArguments(t *testing.T) {
	messages := []Message{{
		Role: "assistant",
		ToolCalls: []ToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: FunctionCall{Name: "bash", Arguments: `{"command":"ls"}`},
		}},
	}}

	encoded, err := json.Marshal(messagesToOpenAIMessages(messages))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"arguments":"{\"command\":\"ls\"}"`) {
		t.Fatalf("string arguments changed: %s", encoded)
	}
}

// Regression: a conversation switched from Gemini to DeepSeek persisted this
// exact assistant turn. Replaying it produced a 422 "invalid type: map,
// expected a string" before the tool could run.
// Regression: a run stopped while the model was thinking left an assistant turn
// with only reasoning stored. Replaying it made DeepSeek answer 400 "Invalid
// assistant message: content or tool_calls must be set". The reasoning is now
// promoted to the message text, so the model keeps its plan and the request is
// valid, while clients still render it as a thought.
func TestMessagesToOpenAIMessagesPromoteThinkingTurn(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "write an essay"},
		{Role: "assistant", Thought: "planning", Content: nil},
		{Role: "assistant", Content: "   "},
		{Role: "assistant", Content: "the answer"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_0", Type: "function", Function: FunctionCall{Name: "bash", Arguments: "{}"}}}},
	}

	out := messagesToOpenAIMessages(messages)
	// The thinking turn is kept; the turn with neither text nor reasoning is not.
	if len(out) != 4 {
		t.Fatalf("messages = %+v, want user, thinking, answer and the tool call", out)
	}
	if out[1].Role != "assistant" || out[1].Content != "[stopped while thinking]\nplanning" {
		t.Fatalf("thinking turn = %+v", out[1])
	}
	if out[1].ReasoningContent != "" {
		t.Fatalf("reasoning was sent twice: %+v", out[1])
	}
	if out[2].Content != "the answer" {
		t.Fatalf("messages = %+v", out)
	}
	if len(out[3].ToolCalls) != 1 || out[3].ToolCalls[0].Function.Name != "bash" {
		t.Fatalf("tool call turn was dropped: %+v", out[3])
	}
}

func TestOpenAIPayloadFromStoredGeminiTranscript(t *testing.T) {
	const stored = `{"role":"assistant","tool_calls":[{"id":"call_0","type":"function","function":{"name":"provider","arguments":{"action":"set_default","name":"deepseek"},"thought_signature":"EswHCskH"}}],"thought":"switch it","v":2}`

	var msg Message
	if err := json.Unmarshal([]byte(stored), &msg); err != nil {
		t.Fatalf("unmarshal stored message: %v", err)
	}

	encoded, err := json.Marshal(messagesToOpenAIMessages([]Message{msg}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"arguments":"{\"action\":\"set_default\",\"name\":\"deepseek\"}"`) {
		t.Fatalf("arguments are not a JSON string: %s", encoded)
	}
}
