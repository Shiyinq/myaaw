package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"myaaw/internal/agent/approvals"
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

// retryProvider announces a retry before handing the request to the scripted
// turns, so the engine sees the same events a real provider emits.
type retryProvider struct {
	scriptedProvider
	attempts []int
}

func (p *retryProvider) ChatStreamEvents(
	ctx context.Context,
	modelName string,
	messages []provider.Message,
	toolsList []map[string]interface{},
	onEvent func(provider.StreamEvent) error,
) error {
	for _, attempt := range p.attempts {
		if err := onEvent(provider.StreamEvent{
			Type: provider.StreamAttempt, Attempt: attempt, Err: errors.New("transient failure"),
		}); err != nil {
			return err
		}
	}
	return p.scriptedProvider.ChatStreamEvents(ctx, modelName, messages, toolsList, onEvent)
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

// structuredFakeTool returns an image alongside text, exercising the vision
// loop end to end through the legacy factory adapter.
type structuredFakeTool struct{ fakeTool }

func (s *structuredFakeTool) CallToolStructured(_ context.Context, arguments string, _ *tools.ToolsContext) tools.StructuredResult {
	return tools.StructuredResult{
		Text:   "shot:" + arguments,
		Images: []tools.ImageResult{{MimeType: "image/png", Data: "AAAA"}},
		Attachments: []tools.FileAttachment{{
			FileName: "shot.png",
			MimeType: "image/png",
			Data:     []byte("PNGDATA"),
		}},
	}
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

func TestAgentLoopKeepsToolImagesInTranscript(t *testing.T) {
	probe := &structuredFakeTool{fakeTool{name: "test_image"}}
	tools.Register("test_image", probe)
	t.Cleanup(func() { tools.ClearRegistryKey("test_image") })

	prov := &scriptedProvider{turns: []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{toolCall("c1", "test_image", `{"x":"1"}`)}},
		{Role: "assistant", Content: "done"},
	}}
	result, err := NewAgent(prov).RunEvents(context.Background(), RunConfig{
		Model: "test-model", MaxIterations: 3, ExcludedTools: []string{},
	}, nil, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	var toolMessage provider.Message
	for _, msg := range result.Transcript {
		if msg.Role == "tool" && msg.Name == "test_image" {
			toolMessage = msg
		}
	}
	items, ok := toolMessage.Content.([]provider.ContentItem)
	if !ok {
		t.Fatalf("tool content type = %T; want []ContentItem", toolMessage.Content)
	}
	var sawText, sawImage bool
	for _, item := range items {
		if item.Type == "text" && item.Text != "" {
			sawText = true
		}
		if item.Type == "image_url" && item.ImageURL != nil && item.ImageURL.URL == "data:image/png;base64,AAAA" {
			sawImage = true
		}
	}
	if !sawText || !sawImage {
		t.Fatalf("tool content = %+v (text=%v image=%v)", items, sawText, sawImage)
	}

	// The trace stays text-only and the next provider request carries the image.
	if len(result.Final.Trace) != 1 || !strings.Contains(result.Final.Trace[0].Observation, "shot:") {
		t.Fatalf("trace = %+v", result.Final.Trace)
	}
	if len(prov.requests) < 2 {
		t.Fatalf("provider requests = %d", len(prov.requests))
	}
	var sentImage bool
	for _, msg := range prov.requests[1] {
		if msg.Role != "tool" {
			continue
		}
		for _, item := range provider.MessageContentItems(msg) {
			if item.Type == "image_url" && item.ImageURL != nil {
				sentImage = true
			}
		}
	}
	if !sentImage {
		t.Fatal("image did not survive into the next provider request")
	}
	if len(result.Attachments) != 1 || result.Attachments[0].FileName != "shot.png" || string(result.Attachments[0].Data) != "PNGDATA" {
		t.Fatalf("attachments = %+v", result.Attachments)
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

func TestRunEventsReportsProviderRetries(t *testing.T) {
	prov := &retryProvider{
		scriptedProvider: scriptedProvider{turns: []provider.Message{
			{Role: "assistant", Content: "ok"},
		}},
		attempts: []int{2, 3},
	}

	var retries []AgentEvent
	_, err := NewAgent(prov).RunEvents(
		context.Background(),
		RunConfig{Model: "test-model", MaxIterations: 1},
		[]provider.Message{{Role: "user", Content: "hi"}},
		func(event AgentEvent) error {
			if event.Type == EventLLMRetry {
				retries = append(retries, event)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(retries) != 2 {
		t.Fatalf("retry events = %d, want 2", len(retries))
	}
	if retries[0].Attempt != 2 || retries[1].Attempt != 3 {
		t.Fatalf("attempts = %d/%d", retries[0].Attempt, retries[1].Attempt)
	}
	for _, retry := range retries {
		if retry.Err == nil {
			t.Fatal("retry event lost the failure reason")
		}
		if retry.StepIndex != 1 {
			t.Fatalf("retry step = %d, want 1", retry.StepIndex)
		}
	}
}

func TestRunEventsReportsCompaction(t *testing.T) {
	big := strings.Repeat("x", 2000)
	prov := &scriptedProvider{turns: []provider.Message{
		{Role: "assistant", Content: "ok"},
	}}

	var compactions []AgentEvent
	_, err := NewAgent(prov).RunEvents(
		context.Background(),
		RunConfig{Model: "test-model", MaxIterations: 1, ContextMaxTokens: 100},
		[]provider.Message{
			{Role: "system", Content: "be brief"},
			{Role: "user", Content: big},
			{Role: "assistant", Content: big},
			{Role: "user", Content: "latest question"},
		},
		func(event AgentEvent) error {
			if event.Type == EventContextCompacted {
				compactions = append(compactions, event)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(compactions) != 1 {
		t.Fatalf("compaction events = %d, want 1", len(compactions))
	}
	stats := compactions[0].Compact
	if !stats.Changed || stats.DroppedMessages == 0 {
		t.Fatalf("compaction stats = %+v", stats)
	}
	if stats.Budget != 100 {
		t.Fatalf("budget = %d", stats.Budget)
	}
}

func TestRunEventsIsQuietWithoutCompaction(t *testing.T) {
	prov := &scriptedProvider{turns: []provider.Message{
		{Role: "assistant", Content: "ok"},
	}}

	var sawCompaction bool
	_, err := NewAgent(prov).RunEvents(
		context.Background(),
		RunConfig{Model: "test-model", MaxIterations: 1, ContextMaxTokens: 100000},
		[]provider.Message{{Role: "user", Content: "hi"}},
		func(event AgentEvent) error {
			if event.Type == EventContextCompacted {
				sawCompaction = true
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if sawCompaction {
		t.Fatal("a transcript that fits the budget must not report compaction")
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

type gateRequester struct {
	decision approvals.Decision
	calls    int
}

func (r *gateRequester) Request(_ context.Context, _ approvals.Request) (approvals.Decision, error) {
	r.calls++
	return r.decision, nil
}

type gateCtxTool struct {
	seen *approvals.Gate
}

func (t *gateCtxTool) ToolDefinition() []byte {
	return []byte(`{"type":"function","function":{"name":"test_gate_ctx","parameters":{"type":"object"}}}`)
}

func (t *gateCtxTool) CallTool(args string, _ *tools.ToolsContext) string {
	return t.CallToolContext(context.Background(), args, nil)
}

func (t *gateCtxTool) CallToolContext(ctx context.Context, _ string, _ *tools.ToolsContext) string {
	t.seen = approvals.GateFromContext(ctx)
	return "ok"
}

func transcriptToolMessage(t *testing.T, transcript []provider.Message, toolName string) provider.Message {
	t.Helper()
	for _, msg := range transcript {
		if msg.Role == "tool" && msg.Name == toolName {
			return msg
		}
	}
	t.Fatalf("tool message %q not found", toolName)
	return provider.Message{}
}

func TestApprovalGateBlocksToolWithoutRequester(t *testing.T) {
	var executed bool
	registerFakeTool(t, "test_gated", &fakeTool{run: func(string) string {
		executed = true
		return "should not run"
	}})

	prov := &scriptedProvider{turns: []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{toolCall("c1", "test_gated", `{"x":"1"}`)}},
		{Role: "assistant", Content: "done"},
	}}
	gate := &approvals.Gate{
		Policy: approvals.Policy{Mode: "manual"},
		Auto:   approvals.Deny,
	}

	result, err := NewAgent(prov).RunEvents(context.Background(), RunConfig{
		Model: "test-model", MaxIterations: 3, ApprovalGate: gate,
	}, nil, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if executed {
		t.Fatal("gated tool executed without approval")
	}
	msg := transcriptToolMessage(t, result.Transcript, "test_gated")
	if !msg.IsError || !strings.Contains(provider.MessageText(msg), "requires approval") {
		t.Fatalf("tool message = %+v", msg)
	}
}

func TestApprovalGateAutoAllowsUnattended(t *testing.T) {
	var executed bool
	registerFakeTool(t, "test_unattended", &fakeTool{run: func(string) string {
		executed = true
		return "ok"
	}})

	prov := &scriptedProvider{turns: []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{toolCall("c1", "test_unattended", `{"x":"1"}`)}},
		{Role: "assistant", Content: "done"},
	}}
	gate := &approvals.Gate{
		Policy: approvals.Policy{Mode: "manual"},
		Auto:   approvals.AllowOnce,
	}

	if _, err := NewAgent(prov).RunEvents(context.Background(), RunConfig{
		Model: "test-model", MaxIterations: 3, ApprovalGate: gate,
	}, nil, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !executed {
		t.Fatal("unattended allow did not execute the tool")
	}
}

func TestApprovalGateAsksRequester(t *testing.T) {
	var executed bool
	registerFakeTool(t, "test_ask", &fakeTool{run: func(string) string {
		executed = true
		return "ok"
	}})

	prov := &scriptedProvider{turns: []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{toolCall("c1", "test_ask", `{"x":"1"}`)}},
		{Role: "assistant", Content: "done"},
	}}
	requester := &gateRequester{decision: approvals.AllowOnce}
	gate := &approvals.Gate{
		Policy:    approvals.Policy{Mode: "manual", Timeout: time.Second},
		Requester: requester,
	}

	if _, err := NewAgent(prov).RunEvents(context.Background(), RunConfig{
		Model: "test-model", MaxIterations: 3, ApprovalGate: gate,
	}, nil, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !executed || requester.calls != 1 {
		t.Fatalf("executed=%v requests=%d", executed, requester.calls)
	}
}

func TestApprovalGateActionKeys(t *testing.T) {
	var clicked, captured bool
	registerFakeTool(t, "test_computer", &fakeTool{run: func(args string) string {
		if strings.Contains(args, "click") {
			clicked = true
		}
		if strings.Contains(args, "screenshot") {
			captured = true
		}
		return "ok"
	}})

	prov := &scriptedProvider{turns: []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{
			toolCall("c1", "test_computer", `{"action":"click","x":"1"}`),
			toolCall("c2", "test_computer", `{"action":"screenshot","x":"1"}`),
		}},
		{Role: "assistant", Content: "done"},
	}}
	gate := &approvals.Gate{
		Policy: approvals.Policy{Mode: "manual", Allow: []string{"test_computer:screenshot"}},
		Auto:   approvals.Deny,
	}

	if _, err := NewAgent(prov).RunEvents(context.Background(), RunConfig{
		Model: "test-model", MaxIterations: 3, ApprovalGate: gate,
	}, nil, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	if clicked {
		t.Fatal("click should have been blocked")
	}
	if !captured {
		t.Fatal("screenshot should have been allowed")
	}
}

func TestApprovalGateChainsBlockedTools(t *testing.T) {
	var executed bool
	registerFakeTool(t, "test_blocked_chain", &fakeTool{run: func(string) string {
		executed = true
		return "ok"
	}})

	prov := &scriptedProvider{turns: []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{toolCall("c1", "test_blocked_chain", `{"x":"1"}`)}},
		{Role: "assistant", Content: "done"},
	}}
	requester := &gateRequester{decision: approvals.AllowOnce}
	gate := &approvals.Gate{
		Policy:    approvals.Policy{Mode: "manual", Timeout: time.Second},
		Requester: requester,
	}

	_, err := NewAgent(prov).RunEvents(context.Background(), RunConfig{
		Model:         "test-model",
		MaxIterations: 3,
		ApprovalGate:  gate,
		BeforeToolCall: func(context.Context, ToolCallInput, json.RawMessage) *ToolBlock {
			return &ToolBlock{Block: true, Reason: "blocked earlier"}
		},
	}, nil, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if executed {
		t.Fatal("blocked tool executed")
	}
	if requester.calls != 0 {
		t.Fatal("approval should not be requested for a blocked tool")
	}
}

func TestApprovalGateExposedToTools(t *testing.T) {
	probe := &gateCtxTool{}
	tools.Register("test_gate_ctx", probe)
	t.Cleanup(func() { tools.ClearRegistryKey("test_gate_ctx") })

	prov := &scriptedProvider{turns: []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{toolCall("c1", "test_gate_ctx", `{}`)}},
		{Role: "assistant", Content: "done"},
	}}
	gate := &approvals.Gate{Policy: approvals.Policy{Mode: "off"}}

	if _, err := NewAgent(prov).RunEvents(context.Background(), RunConfig{
		Model: "test-model", MaxIterations: 3, ApprovalGate: gate,
	}, nil, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	if probe.seen != gate {
		t.Fatal("tool did not see the approval gate in its context")
	}
}
