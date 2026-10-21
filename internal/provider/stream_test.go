package provider

import (
	"strings"
	"testing"
)

func collectDone(t *testing.T, run func(func(StreamEvent) error) error) StreamEvent {
	t.Helper()
	var done StreamEvent
	err := run(func(event StreamEvent) error {
		if event.Type == StreamDone {
			done = event
		}
		return nil
	})
	if err != nil {
		t.Fatalf("stream parser returned error: %v", err)
	}
	if done.Message == nil {
		t.Fatal("stream did not produce a StreamDone message")
	}
	return done
}

func TestConsumeOpenAISSEAccumulatesTextAndToolCalls(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		`data: {"choices":[{"delta":{"content":"Hello "}}]}`,
		`data: {"choices":[{"delta":{"content":"world"}}]}`,
		`data: {"choices":[{"delta":{"reasoning_content":"think"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"cmd\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		`data: [DONE]`,
		"",
	}, "\n")

	done := collectDone(t, func(onEvent func(StreamEvent) error) error {
		return consumeOpenAISSE(strings.NewReader(stream), onEvent)
	})

	if done.StopReason != StopNormal {
		t.Fatalf("stop reason = %q", done.StopReason)
	}
	if got := MessageText(*done.Message); got != "Hello world" {
		t.Fatalf("text = %q", got)
	}
	if done.Message.Thought != "think" {
		t.Fatalf("thought = %q", done.Message.Thought)
	}
	if done.Usage.TotalTokens != 15 {
		t.Fatalf("usage = %d", done.Usage.TotalTokens)
	}
	if len(done.Message.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d", len(done.Message.ToolCalls))
	}
	call := done.Message.ToolCalls[0]
	if call.ID != "call_1" || call.Function.Name != "bash" {
		t.Fatalf("tool call = %+v", call)
	}
	if call.Function.Arguments != `{"cmd":"ls"}` {
		t.Fatalf("tool arguments = %q", call.Function.Arguments)
	}
}

func TestConsumeOpenAISSEMapsLength(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"partial"},"finish_reason":"length"}]}`,
		`data: [DONE]`,
		"",
	}, "\n")

	done := collectDone(t, func(onEvent func(StreamEvent) error) error {
		return consumeOpenAISSE(strings.NewReader(stream), onEvent)
	})
	if done.StopReason != StopLength {
		t.Fatalf("stop reason = %q, want length", done.StopReason)
	}
}

func TestConsumeOpenAISSEIgnoresUsageOnlyChunkChoices(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
		`data: [DONE]`,
		"",
	}, "\n")

	done := collectDone(t, func(onEvent func(StreamEvent) error) error {
		return consumeOpenAISSE(strings.NewReader(stream), onEvent)
	})
	if done.Usage.TotalTokens != 3 {
		t.Fatalf("usage = %d", done.Usage.TotalTokens)
	}
}

func TestConsumeGeminiStream(t *testing.T) {
	payload := `[
	  {"candidates":[{"content":{"parts":[{"text":"thinking","thought":true},{"text":"Hello"}]}}]},
	  {"candidates":[{"content":{"role":"model","parts":[{"text":" world"},{"functionCall":{"name":"bash","args":{"cmd":"ls"}}}]},"finishReason":"STOP"}],
	   "usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":4,"totalTokenCount":7}}
	]`

	g := &GeminiProvider{}
	done := collectDone(t, func(onEvent func(StreamEvent) error) error {
		return g.consumeGeminiStream(strings.NewReader(payload), onEvent)
	})

	if done.StopReason != StopNormal {
		t.Fatalf("stop reason = %q", done.StopReason)
	}
	if got := MessageText(*done.Message); got != "Hello world" {
		t.Fatalf("text = %q", got)
	}
	if done.Message.Thought != "thinking" {
		t.Fatalf("thought = %q", done.Message.Thought)
	}
	if done.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %d", done.Usage.TotalTokens)
	}
	if len(done.Message.ToolCalls) != 1 || done.Message.ToolCalls[0].Function.Name != "bash" {
		t.Fatalf("tool calls = %+v", done.Message.ToolCalls)
	}
}

func TestConsumeGeminiStreamMapsMaxTokens(t *testing.T) {
	payload := `[{"candidates":[{"content":{"parts":[{"text":"partial"}]},"finishReason":"MAX_TOKENS"}]}]`
	g := &GeminiProvider{}
	done := collectDone(t, func(onEvent func(StreamEvent) error) error {
		return g.consumeGeminiStream(strings.NewReader(payload), onEvent)
	})
	if done.StopReason != StopLength {
		t.Fatalf("stop reason = %q, want length", done.StopReason)
	}
}

func TestConsumeOllamaStream(t *testing.T) {
	payload := strings.Join([]string{
		`{"message":{"role":"assistant","content":"Hello"},"done":false}`,
		`{"message":{"role":"assistant","content":" world"},"done":false}`,
		`{"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"bash","arguments":{"cmd":"ls"}}}]},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":3,"eval_count":4}`,
	}, "\n")

	o := &OllamaProvider{}
	done := collectDone(t, func(onEvent func(StreamEvent) error) error {
		return o.consumeOllamaStream(strings.NewReader(payload), onEvent)
	})

	if done.StopReason != StopNormal {
		t.Fatalf("stop reason = %q", done.StopReason)
	}
	if got := MessageText(*done.Message); got != "Hello world" {
		t.Fatalf("text = %q", got)
	}
	if done.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %d", done.Usage.TotalTokens)
	}
	if len(done.Message.ToolCalls) != 1 || done.Message.ToolCalls[0].Function.Name != "bash" {
		t.Fatalf("tool calls = %+v", done.Message.ToolCalls)
	}
}

func TestConsumeOllamaStreamMapsLength(t *testing.T) {
	payload := `{"message":{"role":"assistant","content":"partial"},"done":true,"done_reason":"length"}`
	o := &OllamaProvider{}
	done := collectDone(t, func(onEvent func(StreamEvent) error) error {
		return o.consumeOllamaStream(strings.NewReader(payload), onEvent)
	})
	if done.StopReason != StopLength {
		t.Fatalf("stop reason = %q, want length", done.StopReason)
	}
}
