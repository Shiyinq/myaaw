package service

import (
	"context"
	"testing"

	"myaaw/internal/agent"
	"myaaw/internal/channel"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
)

// checkpointAgent records whether the run received a checkpoint hook and calls
// it with a scripted transcript.
type checkpointAgent struct {
	transcript   []provider.Message
	final        provider.Message
	checkpointed bool
}

func (a *checkpointAgent) Run(string, []provider.Message) (provider.Message, error) {
	return a.final, nil
}

func (a *checkpointAgent) RunContext(context.Context, string, []provider.Message) (provider.Message, error) {
	return a.final, nil
}

func (a *checkpointAgent) RunStream(string, []provider.Message, func(provider.Message) error) error {
	return nil
}

func (a *checkpointAgent) RunEvents(ctx context.Context, cfg agent.RunConfig, _ []provider.Message, _ func(agent.AgentEvent) error) (agent.RunResult, error) {
	a.checkpointed = cfg.OnCheckpoint != nil
	if cfg.OnCheckpoint != nil {
		cfg.OnCheckpoint(ctx, a.transcript)
	}
	return agent.RunResult{
		Transcript: a.transcript,
		Final:      a.final,
		StopReason: provider.StopNormal,
	}, nil
}

func (a *checkpointAgent) WithExcludedTools(...string) agent.AgentProvider { return a }

func newCheckpointService(t *testing.T, ag agent.AgentProvider) (*BotServiceImpl, *fakeConversationRepo) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	repo := newFakeConversationRepo()
	repo.conversations["conv-1"] = &model.Conversation{
		Id:     "conv-1",
		UserId: 1,
		Title:  "Existing chat",
	}

	return &BotServiceImpl{
		conversationRepo: repo,
		agent:            ag,
		llmProvider:      provider.NewOpenAIProvider("http://127.0.0.1:1", "sk-test", "test-model"),
	}, repo
}

func TestRunConversationCheckpointsTheInProgressTranscript(t *testing.T) {
	ag := &checkpointAgent{
		transcript: []provider.Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "pertanyaan"},
		},
		final: provider.Message{Role: "assistant", Content: "jawaban"},
	}
	svc, repo := newCheckpointService(t, ag)

	msg := &channel.IncomingMessage{
		UserID:         1,
		Text:           "pertanyaan",
		Channel:        "api",
		ConversationID: "conv-1",
	}
	if _, err := svc.runConversation(context.Background(), &model.User{UserId: 1}, msg, nil); err != nil {
		t.Fatalf("runConversation: %v", err)
	}
	if !ag.checkpointed {
		t.Fatal("run did not receive a checkpoint hook")
	}
	if len(repo.updates) == 0 {
		t.Fatal("checkpoint never reached the repository")
	}

	first := repo.updates[0]
	if first.id != "conv-1" {
		t.Fatalf("checkpoint conversation = %q", first.id)
	}
	if len(first.messages) != 1 || first.messages[0].Role != "user" {
		t.Fatalf("first checkpoint = %+v, want only the user message", first.messages)
	}
	if first.messages[0].Content != "pertanyaan" {
		t.Fatalf("checkpointed content = %v", first.messages[0].Content)
	}
}

func TestRunConversationSkipsCheckpointsForHeartbeat(t *testing.T) {
	ag := &checkpointAgent{
		transcript: []provider.Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "[SYSTEM TRIGGER: HEARTBEAT]\nHEARTBEAT_OK"},
		},
		final: provider.Message{Role: "assistant", Content: "HEARTBEAT_OK"},
	}
	svc, repo := newCheckpointService(t, ag)

	msg := &channel.IncomingMessage{
		UserID:         1,
		Text:           "HEARTBEAT_OK",
		Channel:        "api",
		ConversationID: "conv-1",
		TriggerType:    "heartbeat",
	}
	if _, err := svc.runConversation(context.Background(), &model.User{UserId: 1}, msg, nil); err != nil {
		t.Fatalf("runConversation: %v", err)
	}
	if ag.checkpointed {
		t.Fatal("heartbeat runs must not checkpoint")
	}
	if len(repo.updates) != 0 {
		t.Fatalf("heartbeat wrote %d updates, want 0", len(repo.updates))
	}
}

func TestRunConversationWithoutProviderFailsClearly(t *testing.T) {
	svc := &BotServiceImpl{conversationRepo: newFakeConversationRepo()}

	_, err := svc.runConversation(context.Background(), &model.User{UserId: 1}, &channel.IncomingMessage{
		UserID:  1,
		Text:    "hi",
		Channel: "api",
	}, nil)
	if err == nil {
		t.Fatal("expected an error when no provider is configured")
	}
}
