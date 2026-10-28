package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"myaaw/internal/agent/tools"
	"myaaw/internal/provider"
)

type scriptedProvider struct {
	turns      []provider.Message
	repeatLast bool
	calls      int
	requests   [][]provider.Message
}

func (p *scriptedProvider) ProviderName() string { return "test" }

func (p *scriptedProvider) DefaultModel(modelName string) string {
	if modelName == "" {
		return "test-model"
	}
	return modelName
}

func (p *scriptedProvider) Models() ([]string, error) { return []string{"test-model"}, nil }

func (p *scriptedProvider) next() (provider.Message, bool) {
	if p.calls < len(p.turns) {
		msg := p.turns[p.calls]
		p.calls++
		return msg, true
	}
	if p.repeatLast && len(p.turns) > 0 {
		p.calls++
		return p.turns[len(p.turns)-1], true
	}
	return provider.Message{}, false
}

func (p *scriptedProvider) Chat(modelName string, messages []provider.Message, toolsList []map[string]interface{}) (provider.Message, error) {
	msg, ok := p.next()
	if !ok {
		return provider.Message{}, errors.New("no scripted turn")
	}
	return msg, nil
}

func (p *scriptedProvider) ChatStream(modelName string, messages []provider.Message, callback func(provider.Message) error, toolsList []map[string]interface{}) error {
	return p.ChatStreamEvents(context.Background(), modelName, messages, toolsList, func(event provider.StreamEvent) error {
		switch event.Type {
		case provider.StreamTextDelta:
			return callback(provider.Message{Role: "assistant", Content: event.Text})
		case provider.StreamThinkingDelta:
			return callback(provider.Message{Role: "assistant", Thought: event.Thought})
		case provider.StreamDone:
			if event.Message != nil {
				return callback(*event.Message)
			}
		}
		return nil
	})
}

func (p *scriptedProvider) ChatStreamEvents(
	ctx context.Context,
	modelName string,
	messages []provider.Message,
	toolsList []map[string]interface{},
	onEvent func(provider.StreamEvent) error,
) error {
	copied := make([]provider.Message, len(messages))
	copy(copied, messages)
	p.requests = append(p.requests, copied)

	if err := ctx.Err(); err != nil {
		return err
	}
	msg, ok := p.next()
	if !ok {
		return errors.New("no scripted turn")
	}
	if text := provider.MessageText(msg); text != "" {
		if err := onEvent(provider.StreamEvent{Type: provider.StreamTextDelta, Text: text}); err != nil {
			return err
		}
	}
	if msg.Thought != "" {
		if err := onEvent(provider.StreamEvent{Type: provider.StreamThinkingDelta, Thought: msg.Thought}); err != nil {
			return err
		}
	}
	if msg.Usage.TotalTokens > 0 {
		if err := onEvent(provider.StreamEvent{Type: provider.StreamUsage, Usage: msg.Usage}); err != nil {
			return err
		}
	}
	stop := msg.StopReason
	if stop == "" {
		stop = provider.StopNormal
	}
	return onEvent(provider.StreamEvent{
		Type:       provider.StreamDone,
		Message:    &msg,
		Usage:      msg.Usage,
		StopReason: stop,
	})
}

type fakeTool struct {
	name          string
	mode          tools.ExecutionMode
	panic         bool
	sleep         time.Duration
	ignoreContext bool
	run           func(args string) string
}

func (f *fakeTool) CallTool(arguments string, _ *tools.ToolsContext) string {
	return f.CallToolContext(context.Background(), arguments, nil)
}

func (f *fakeTool) CallToolContext(ctx context.Context, arguments string, _ *tools.ToolsContext) string {
	if f.panic {
		panic("boom")
	}
	if f.sleep > 0 {
		if f.ignoreContext {
			time.Sleep(f.sleep)
		} else {
			select {
			case <-time.After(f.sleep):
			case <-ctx.Done():
				return "canceled"
			}
		}
	}
	if f.run != nil {
		return f.run(arguments)
	}
	return "ok:" + arguments
}

func (f *fakeTool) ToolDefinition() []byte {
	return []byte(`{"type":"function","function":{"name":"` + f.name + `","parameters":{"type":"object","properties":{"x":{"type":"string"}},"required":["x"]}}}`)
}

func (f *fakeTool) ExecutionMode() tools.ExecutionMode {
	if f.mode == "" {
		return tools.ExecutionSequential
	}
	return f.mode
}

func registerFakeTool(t *testing.T, name string, tool *fakeTool) {
	t.Helper()
	if tool.name == "" {
		tool.name = name
	}
	tools.Register(name, tool)
	t.Cleanup(func() { tools.ClearRegistryKey(name) })
}

func toolCall(id, name, args string) provider.ToolCall {
	return provider.ToolCall{
		ID:   id,
		Type: "function",
		Function: provider.FunctionCall{
			Name:      name,
			Arguments: args,
		},
	}
}

func TestAgentLoopExecutesToolsAndAggregatesUsage(t *testing.T) {
	registerFakeTool(t, "test_echo", &fakeTool{
		run: func(args string) string { return "echo:" + args },
	})

	prov := &scriptedProvider{turns: []provider.Message{
		{
			Role:      "assistant",
			ToolCalls: []provider.ToolCall{toolCall("c1", "test_echo", `{"x":"1"}`)},
			Usage:     provider.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		},
		{
			Role:    "assistant",
			Content: "final answer",
			Usage:   provider.Usage{PromptTokens: 4, CompletionTokens: 6, TotalTokens: 10},
		},
	}}
	a := NewAgent(prov)

	result, err := a.RunEvents(context.Background(), RunConfig{
		Model:               "test-model",
		MaxIterations:       5,
		WarningThreshold:    4,
		ExcludedTools:       []string{},
		RepeatToolCallLimit: 3,
	}, nil, nil)
	if err != nil {
		t.Fatalf("RunEvents returned error: %v", err)
	}
	if result.StopReason != provider.StopNormal {
		t.Fatalf("stop reason = %q, want normal", result.StopReason)
	}
	if got := provider.MessageText(result.Final); got != "final answer" {
		t.Fatalf("final text = %q", got)
	}
	if result.Usage.TotalTokens != 25 {
		t.Fatalf("aggregated usage = %d, want 25", result.Usage.TotalTokens)
	}
	if len(result.Final.Trace) != 1 {
		t.Fatalf("trace length = %d, want 1", len(result.Final.Trace))
	}
	if result.Final.Trace[0].Action != "test_echo" {
		t.Fatalf("trace action = %q", result.Final.Trace[0].Action)
	}

	var foundTool bool
	for _, msg := range result.Transcript {
		if msg.Role == "tool" {
			foundTool = true
			if msg.Content != `echo:{"x":"1"}` {
				t.Fatalf("tool result = %q", msg.Content)
			}
			if msg.ToolCallID != "c1" {
				t.Fatalf("tool call id = %q", msg.ToolCallID)
			}
		}
	}
	if !foundTool {
		t.Fatal("transcript is missing the tool result message")
	}
}

func TestAgentLoopStopsAtMaxIterations(t *testing.T) {
	registerFakeTool(t, "test_loop", &fakeTool{})

	prov := &scriptedProvider{
		repeatLast: true,
		turns: []provider.Message{{
			Role:      "assistant",
			ToolCalls: []provider.ToolCall{toolCall("c1", "test_loop", `{"x":"1"}`)},
		}},
	}
	a := NewAgent(prov)

	result, err := a.RunEvents(context.Background(), RunConfig{
		Model:            "test-model",
		MaxIterations:    2,
		WarningThreshold: 1,
		ExcludedTools:    []string{},
	}, nil, nil)
	if err != nil {
		t.Fatalf("RunEvents returned error: %v", err)
	}
	if prov.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", prov.calls)
	}
	if result.StopReason != provider.StopMaxIterations {
		t.Fatalf("stop reason = %q, want max_iterations", result.StopReason)
	}
	if provider.MessageText(result.Final) != MaxIterationsMessage {
		t.Fatalf("final text = %q", provider.MessageText(result.Final))
	}
	if len(prov.requests) != 2 {
		t.Fatalf("recorded requests = %d", len(prov.requests))
	}
	if !strings.Contains(provider.MessageText(prov.requests[1][0]), "steps remaining") {
		t.Fatalf("second request is missing the step warning: %q", provider.MessageText(prov.requests[1][0]))
	}
}

func TestAgentLoopFailsTruncatedToolCalls(t *testing.T) {
	var executed int
	registerFakeTool(t, "test_truncated", &fakeTool{
		run: func(args string) string {
			executed++
			return "should not run"
		},
	})

	prov := &scriptedProvider{turns: []provider.Message{
		{
			Role:       "assistant",
			ToolCalls:  []provider.ToolCall{toolCall("c1", "test_truncated", `{"x":"1"}`)},
			StopReason: provider.StopLength,
		},
		{
			Role:    "assistant",
			Content: "recovered",
		},
	}}
	a := NewAgent(prov)

	result, err := a.RunEvents(context.Background(), RunConfig{
		Model:            "test-model",
		MaxIterations:    5,
		WarningThreshold: 4,
		ExcludedTools:    []string{},
	}, nil, nil)
	if err != nil {
		t.Fatalf("RunEvents returned error: %v", err)
	}
	if executed != 0 {
		t.Fatalf("truncated tool call executed %d times", executed)
	}
	if provider.MessageText(result.Final) != "recovered" {
		t.Fatalf("final text = %q", provider.MessageText(result.Final))
	}
	var sawError bool
	for _, msg := range result.Transcript {
		if msg.Role == "tool" && msg.IsError {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("truncated tool call did not produce an error tool result")
	}
}

func TestAgentLoopCancellation(t *testing.T) {
	prov := &scriptedProvider{turns: []provider.Message{{Role: "assistant", Content: "never"}}}
	a := NewAgent(prov)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := a.RunEvents(ctx, RunConfig{
		Model:         "test-model",
		MaxIterations: 5,
		ExcludedTools: []string{},
	}, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if result.StopReason != provider.StopAborted {
		t.Fatalf("stop reason = %q, want aborted", result.StopReason)
	}
	if prov.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", prov.calls)
	}
}

func TestAgentLoopExecutesParallelToolsInCallOrder(t *testing.T) {
	registerFakeTool(t, "test_slow", &fakeTool{
		mode:  tools.ExecutionParallel,
		sleep: 80 * time.Millisecond,
		run:   func(args string) string { return "slow" },
	})
	registerFakeTool(t, "test_fast", &fakeTool{
		mode:  tools.ExecutionParallel,
		sleep: 80 * time.Millisecond,
		run:   func(args string) string { return "fast" },
	})

	prov := &scriptedProvider{turns: []provider.Message{
		{
			Role: "assistant",
			ToolCalls: []provider.ToolCall{
				toolCall("c1", "test_slow", `{"x":"1"}`),
				toolCall("c2", "test_fast", `{"x":"1"}`),
			},
		},
		{Role: "assistant", Content: "done"},
	}}
	a := NewAgent(prov)

	start := time.Now()
	result, err := a.RunEvents(context.Background(), RunConfig{
		Model:            "test-model",
		MaxIterations:    3,
		WarningThreshold: 2,
		ExcludedTools:    []string{},
	}, nil, nil)
	if err != nil {
		t.Fatalf("RunEvents returned error: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 140*time.Millisecond {
		t.Fatalf("tools did not run in parallel: %v", elapsed)
	}

	var toolContents []string
	for _, msg := range result.Transcript {
		if msg.Role == "tool" {
			if text, ok := msg.Content.(string); ok {
				toolContents = append(toolContents, text)
			}
		}
	}
	if len(toolContents) != 2 || toolContents[0] != "slow" || toolContents[1] != "fast" {
		t.Fatalf("tool result order = %v", toolContents)
	}
}

func TestAgentLoopRecoversToolPanic(t *testing.T) {
	registerFakeTool(t, "test_panic", &fakeTool{panic: true})

	prov := &scriptedProvider{turns: []provider.Message{
		{
			Role:      "assistant",
			ToolCalls: []provider.ToolCall{toolCall("c1", "test_panic", `{"x":"1"}`)},
		},
		{Role: "assistant", Content: "survived"},
	}}
	a := NewAgent(prov)

	result, err := a.RunEvents(context.Background(), RunConfig{
		Model:            "test-model",
		MaxIterations:    3,
		WarningThreshold: 2,
		ExcludedTools:    []string{},
	}, nil, nil)
	if err != nil {
		t.Fatalf("RunEvents returned error: %v", err)
	}
	if provider.MessageText(result.Final) != "survived" {
		t.Fatalf("final text = %q", provider.MessageText(result.Final))
	}
	var sawPanic bool
	for _, msg := range result.Transcript {
		if msg.Role == "tool" && msg.IsError && strings.Contains(provider.MessageText(msg), "panicked") {
			sawPanic = true
		}
	}
	if !sawPanic {
		t.Fatal("panic was not converted into an error tool result")
	}
}

func TestAgentLoopToolTimeoutDoesNotHang(t *testing.T) {
	registerFakeTool(t, "test_slow_tool", &fakeTool{
		sleep:         300 * time.Millisecond,
		ignoreContext: true,
		run:           func(args string) string { return "late" },
	})

	prov := &scriptedProvider{turns: []provider.Message{
		{
			Role:      "assistant",
			ToolCalls: []provider.ToolCall{toolCall("c1", "test_slow_tool", `{"x":"1"}`)},
		},
		{Role: "assistant", Content: "done"},
	}}
	a := NewAgent(prov)

	start := time.Now()
	result, err := a.RunEvents(context.Background(), RunConfig{
		Model:            "test-model",
		MaxIterations:    3,
		WarningThreshold: 2,
		ExcludedTools:    []string{},
		ToolTimeout:      30 * time.Millisecond,
	}, nil, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("RunEvents returned error: %v", err)
	}
	if elapsed >= 250*time.Millisecond {
		t.Fatalf("agent waited for the tool despite its timeout: %v", elapsed)
	}
	if provider.MessageText(result.Final) != "done" {
		t.Fatalf("final text = %q", provider.MessageText(result.Final))
	}
	var sawTimeout bool
	for _, message := range result.Transcript {
		if message.Role == "tool" && message.IsError && strings.Contains(provider.MessageText(message), "timed out") {
			sawTimeout = true
		}
	}
	if !sawTimeout {
		t.Fatal("tool timeout was not surfaced as an error tool result")
	}
}

func TestAgentLoopWarnsOnRepeatedToolCalls(t *testing.T) {
	registerFakeTool(t, "test_repeat", &fakeTool{})

	call := provider.Message{
		Role:      "assistant",
		ToolCalls: []provider.ToolCall{toolCall("c1", "test_repeat", `{"x":"1"}`)},
	}
	prov := &scriptedProvider{
		repeatLast: true,
		turns:      []provider.Message{call},
	}
	a := NewAgent(prov)

	_, err := a.RunEvents(context.Background(), RunConfig{
		Model:               "test-model",
		MaxIterations:       5,
		WarningThreshold:    4,
		ExcludedTools:       []string{},
		RepeatToolCallLimit: 2,
	}, nil, nil)
	if err != nil {
		t.Fatalf("RunEvents returned error: %v", err)
	}

	found := false
	for _, request := range prov.requests {
		if len(request) == 0 {
			continue
		}
		if strings.Contains(provider.MessageText(request[0]), "identical arguments") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("repeat warning was never injected into a request")
	}
}

func TestValidateToolArguments(t *testing.T) {
	schema := json.RawMessage(`{"type":"function","function":{"parameters":{"type":"object","properties":{"x":{"type":"string"},"n":{"type":"integer"}},"required":["x"]}}}`)

	if err := validateToolArguments(schema, json.RawMessage(`{"x":"ok","n":1}`)); err != nil {
		t.Fatalf("valid args rejected: %v", err)
	}
	if err := validateToolArguments(schema, json.RawMessage(`{"n":1}`)); err == nil {
		t.Fatal("missing required argument was accepted")
	}
	if err := validateToolArguments(schema, json.RawMessage(`{"x":1}`)); err == nil {
		t.Fatal("wrong primitive type was accepted")
	}
	if err := validateToolArguments(schema, json.RawMessage(`not-json`)); err == nil {
		t.Fatal("invalid JSON was accepted")
	}
}

func TestRunEventsCarriesRunIdentityAndRequestTranscript(t *testing.T) {
	prov := &scriptedProvider{turns: []provider.Message{
		{
			Role:    "assistant",
			Content: "hello back",
			Usage:   provider.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
		},
	}}
	agentUnderTest := NewAgent(prov)

	var (
		events   []AgentEvent
		requests []AgentEvent
	)
	result, err := agentUnderTest.RunEvents(
		context.Background(),
		RunConfig{Model: "test-model", MaxIterations: 3},
		[]provider.Message{
			{Role: "system", Content: "be nice"},
			{Role: "user", Content: "hello"},
		},
		func(event AgentEvent) error {
			events = append(events, event)
			if event.Type == EventLLMRequestStart {
				requests = append(requests, event)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if result.RunID == "" {
		t.Fatal("RunResult.RunID is empty")
	}
	if result.StartedAt.IsZero() || result.EndedAt.IsZero() {
		t.Fatalf("run timestamps = %v / %v", result.StartedAt, result.EndedAt)
	}
	for _, event := range events {
		if event.RunID != result.RunID {
			t.Fatalf("event %d has run id %q, want %q", event.Type, event.RunID, result.RunID)
		}
		if event.Time.IsZero() {
			t.Fatalf("event %d has no timestamp", event.Type)
		}
	}

	if len(requests) != 1 {
		t.Fatalf("LLM request start events = %d, want 1", len(requests))
	}
	request := requests[0]
	if request.Model != "test-model" {
		t.Fatalf("request model = %q", request.Model)
	}
	if request.StepIndex != 1 {
		t.Fatalf("request turn index = %d, want 1", request.StepIndex)
	}
	if len(request.Messages) < 2 {
		t.Fatalf("request messages = %d, want system and user", len(request.Messages))
	}
	last := request.Messages[len(request.Messages)-1]
	if last.Role != "user" || provider.MessageText(last) != "hello" {
		t.Fatalf("last request message = %+v", last)
	}
}

func TestAddUsageAggregatesCachedInputTokens(t *testing.T) {
	var total provider.Usage
	addUsage(&total, provider.Usage{PromptTokens: 10, CachedInputTokens: 8, CompletionTokens: 2, TotalTokens: 12})
	addUsage(&total, provider.Usage{PromptTokens: 5, CachedInputTokens: 4, CompletionTokens: 1, TotalTokens: 6})

	if total.CachedInputTokens != 12 {
		t.Fatalf("cached input tokens = %d, want 12", total.CachedInputTokens)
	}
	if total.PromptTokens != 15 || total.CompletionTokens != 3 {
		t.Fatalf("usage = %+v", total)
	}
}
