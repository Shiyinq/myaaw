package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/channel"
	"myaaw/internal/common"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
)

// blockingAgent behaves like a provider that is still writing an answer: it
// waits for the turn context and then reports the partial text it produced.
type blockingAgent struct {
	started chan struct{}
}

func (a *blockingAgent) notifyStarted() {
	select {
	case a.started <- struct{}{}:
	default:
	}
}

func (a *blockingAgent) Run(string, []provider.Message) (provider.Message, error) {
	return provider.Message{}, nil
}

func (a *blockingAgent) RunContext(context.Context, string, []provider.Message) (provider.Message, error) {
	return provider.Message{}, nil
}

func (a *blockingAgent) RunStream(string, []provider.Message, func(provider.Message) error) error {
	return nil
}

func (a *blockingAgent) RunEvents(
	ctx context.Context,
	_ agent.RunConfig,
	history []provider.Message,
	_ func(agent.AgentEvent) error,
) (agent.RunResult, error) {
	a.notifyStarted()
	<-ctx.Done()

	transcript := append([]provider.Message{}, history...)
	transcript = append(transcript, provider.Message{Role: "assistant", Content: "partial answer"})
	return agent.RunResult{
		Transcript: transcript,
		Final:      provider.Message{Role: "assistant", Content: "partial answer"},
		StopReason: provider.StopNormal,
	}, ctx.Err()
}

func (a *blockingAgent) WithExcludedTools(...string) agent.AgentProvider { return a }

// TestNewMessageInterruptsTheRunningTurn proves the channel equivalent of
// "interrupt on send": a second message stops the first turn, and a stop
// command stops the turn after it.
func TestNewMessageInterruptsTheRunningTurn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	ag := &blockingAgent{started: make(chan struct{}, 4)}
	repo := newFakeConversationRepo()
	svc := &BotServiceImpl{
		conversationRepo: repo,
		agent:            ag,
		llmProvider:      provider.NewOpenAIProvider("http://127.0.0.1:1", "sk-test", "test-model"),
		runs:             &runRegistry{},
	}
	user := &model.User{UserId: 1}

	type runResult struct {
		out *channel.OutgoingMessage
		err error
	}
	start := func(text string) <-chan runResult {
		done := make(chan runResult, 1)
		go func() {
			out, err := svc.runConversation(context.Background(), user, &channel.IncomingMessage{
				UserID:  1,
				Text:    text,
				Channel: "telegram",
				ChatID:  "42",
			}, nil)
			done <- runResult{out: out, err: err}
		}()
		return done
	}

	first := start("first question")
	waitForStarted(t, ag)

	second := start("second question")

	select {
	case got := <-first:
		if got.err != nil {
			t.Fatalf("first turn: %v", got.err)
		}
		if got.out == nil || !got.out.Stopped {
			t.Fatalf("first turn = %+v, want a stopped partial answer", got.out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the first turn was not interrupted")
	}

	waitForStarted(t, ag)

	handled, text, err := svc.command(user, &channel.IncomingMessage{
		Channel: "telegram",
		ChatID:  "42",
		Text:    "!stop",
	})
	if err != nil || !handled || text != common.CommandStopped() {
		t.Fatalf("command(!stop) = %v, %q, %v", handled, text, err)
	}

	select {
	case got := <-second:
		if got.err != nil {
			t.Fatalf("second turn: %v", got.err)
		}
		if got.out == nil || !got.out.Stopped {
			t.Fatalf("second turn = %+v, want a stopped partial answer", got.out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second turn was not stopped")
	}

	if len(repo.updates) < 2 {
		t.Fatalf("saved updates = %d, want both partial turns persisted", len(repo.updates))
	}
	saved := false
	for _, update := range repo.updates {
		for _, message := range update.messages {
			if strings.Contains(provider.MessageText(message), "partial answer") {
				saved = true
			}
		}
	}
	if !saved {
		t.Fatal("the stopped partial answer never reached the repository")
	}
}

func waitForStarted(t *testing.T, ag *blockingAgent) {
	t.Helper()
	select {
	case <-ag.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the turn never started")
	}
}
