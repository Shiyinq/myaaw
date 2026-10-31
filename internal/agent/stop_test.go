package agent

import (
	"context"
	"errors"
	"testing"

	"myaaw/internal/provider"
)

// stoppingProvider streams a fragment and then waits until the run context is
// cancelled, which is what a stop request looks like from inside the loop.
type stoppingProvider struct {
	started chan struct{}
	text    string
}

func (p *stoppingProvider) ProviderName() string { return "test" }

func (p *stoppingProvider) DefaultModel(modelName string) string {
	if modelName == "" {
		return "test-model"
	}
	return modelName
}

func (p *stoppingProvider) Models() ([]string, error) { return []string{"test-model"}, nil }

func (p *stoppingProvider) Chat(string, []provider.Message, []map[string]interface{}) (provider.Message, error) {
	return provider.Message{}, errors.New("Chat is not used by this test")
}

func (p *stoppingProvider) ChatStream(modelName string, messages []provider.Message, callback func(provider.Message) error, tools []map[string]interface{}) error {
	return p.ChatStreamEvents(context.Background(), modelName, messages, tools, func(event provider.StreamEvent) error {
		switch event.Type {
		case provider.StreamTextDelta:
			return callback(provider.Message{Role: "assistant", Content: event.Text})
		case provider.StreamDone:
			if event.Message != nil {
				return callback(*event.Message)
			}
		}
		return nil
	})
}

func (p *stoppingProvider) ChatStreamEvents(
	ctx context.Context,
	_ string,
	_ []provider.Message,
	_ []map[string]interface{},
	onEvent func(provider.StreamEvent) error,
) error {
	if err := onEvent(provider.StreamEvent{Type: provider.StreamTextDelta, Text: p.text}); err != nil {
		return err
	}
	close(p.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestAgentLoopKeepsPartialTextOnUserStop(t *testing.T) {
	prov := &stoppingProvider{started: make(chan struct{}), text: "half an answer"}
	a := NewAgent(prov)

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	go func() {
		<-prov.started
		cancel(ErrRunStopped)
	}()

	result, err := a.RunEvents(ctx, RunConfig{
		Model:         "test-model",
		MaxIterations: 3,
		ExcludedTools: []string{},
	}, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if result.StopReason != provider.StopAborted {
		t.Fatalf("stop reason = %q, want aborted", result.StopReason)
	}
	if got := provider.MessageText(result.Final); got != "half an answer" {
		t.Fatalf("final text = %q, want the partial answer", got)
	}
	if result.Final.IsError || result.Final.ErrorMessage != "" {
		t.Fatalf("user stop was recorded as an error: %+v", result.Final)
	}
}

func TestAgentLoopUserStopSkipsAbortMarker(t *testing.T) {
	prov := &stoppingProvider{started: make(chan struct{}), text: "never"}
	a := NewAgent(prov)

	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(ErrRunStopped)

	result, err := a.RunEvents(ctx, RunConfig{
		Model:         "test-model",
		MaxIterations: 3,
		ExcludedTools: []string{},
	}, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if len(result.Transcript) != 0 {
		t.Fatalf("transcript = %+v, want no abort marker", result.Transcript)
	}
}

// thinkingProvider streams reasoning and then waits, like a model that was
// interrupted before it produced any answer text.
type thinkingProvider struct {
	started chan struct{}
	thought string
}

func (p *thinkingProvider) ProviderName() string { return "test" }

func (p *thinkingProvider) DefaultModel(modelName string) string {
	if modelName == "" {
		return "test-model"
	}
	return modelName
}

func (p *thinkingProvider) Models() ([]string, error) { return []string{"test-model"}, nil }

func (p *thinkingProvider) Chat(string, []provider.Message, []map[string]interface{}) (provider.Message, error) {
	return provider.Message{}, errors.New("Chat is not used by this test")
}

func (p *thinkingProvider) ChatStream(modelName string, messages []provider.Message, callback func(provider.Message) error, tools []map[string]interface{}) error {
	return p.ChatStreamEvents(context.Background(), modelName, messages, tools, func(event provider.StreamEvent) error {
		switch event.Type {
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

func (p *thinkingProvider) ChatStreamEvents(
	ctx context.Context,
	_ string,
	_ []provider.Message,
	_ []map[string]interface{},
	onEvent func(provider.StreamEvent) error,
) error {
	if err := onEvent(provider.StreamEvent{Type: provider.StreamThinkingDelta, Thought: p.thought}); err != nil {
		return err
	}
	close(p.started)
	<-ctx.Done()
	return ctx.Err()
}

// A turn stopped while the model was still thinking has no answer and no tool
// calls, so it must not reach the transcript: providers reject it on the next
// request with "Invalid assistant message: content or tool_calls must be set".
func TestAgentLoopUserStopDuringThinkingLeavesNoEmptyTurn(t *testing.T) {
	prov := &thinkingProvider{started: make(chan struct{}), thought: "planning the answer"}
	a := NewAgent(prov)

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	go func() {
		<-prov.started
		cancel(ErrRunStopped)
	}()

	result, err := a.RunEvents(ctx, RunConfig{
		Model:         "test-model",
		MaxIterations: 3,
		ExcludedTools: []string{},
	}, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	for _, message := range result.Transcript {
		if message.Role == "assistant" && provider.MessageText(message) == "" && len(message.ToolCalls) == 0 {
			t.Fatalf("empty assistant turn was stored: %+v", message)
		}
	}
	if result.Final.Thought == "" {
		// The reasoning still reaches the UI even though it is not persisted.
		t.Fatal("final message lost the thought")
	}
}

func TestStoppedByClientOnlyMatchesItsCause(t *testing.T) {
	stopped, cancelStopped := context.WithCancelCause(context.Background())
	cancelStopped(ErrRunStopped)
	if !StoppedByClient(stopped) {
		t.Fatal("ErrRunStopped cause was not detected")
	}

	plain, cancelPlain := context.WithCancel(context.Background())
	cancelPlain()
	if StoppedByClient(plain) {
		t.Fatal("a plain cancel is not a client stop")
	}
}
