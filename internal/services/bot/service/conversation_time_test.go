package service

import (
	"testing"
	"time"

	"myaaw/internal/channel"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
)

func TestStampTurnDurationMarksTheFinalAnswer(t *testing.T) {
	transcript := []provider.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "intermediate"},
		{Role: "tool", Content: "result"},
		{Role: "assistant", Content: "final"},
	}
	start := time.Now().Add(-3 * time.Second)
	end := time.Now()

	stampTurnDuration(transcript, start, end)

	if transcript[1].DurationMs != 0 {
		t.Fatalf("intermediate assistant = %d, want 0", transcript[1].DurationMs)
	}
	if transcript[3].DurationMs < 2000 {
		t.Fatalf("final duration = %d, want the run length", transcript[3].DurationMs)
	}
}

func TestSaveConversationPersistsTimesAndDuration(t *testing.T) {
	repo := newFakeConversationRepo()
	repo.conversations["conv-1"] = &model.Conversation{Id: "conv-1", UserId: 1, Title: "Existing"}
	service := &BotServiceImpl{conversationRepo: repo}

	sent := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	transcript := []provider.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "hi", CreatedAt: &sent},
		{Role: "assistant", Content: "hello"},
	}
	stampTurnDuration(transcript, time.Now().Add(-2*time.Second), time.Now())

	if err := service.saveConversation(
		&model.User{UserId: 1},
		&channel.IncomingMessage{UserID: 1, ConversationID: "conv-1"},
		transcript,
	); err != nil {
		t.Fatalf("saveConversation: %v", err)
	}
	if len(repo.updates) == 0 {
		t.Fatal("no conversation update was recorded")
	}

	messages := repo.updates[len(repo.updates)-1].messages
	if len(messages) != 2 {
		t.Fatalf("messages = %+v, want the system prompt stripped", messages)
	}
	if messages[0].CreatedAt == nil || !messages[0].CreatedAt.Equal(sent) {
		t.Fatalf("user time = %v, want %v", messages[0].CreatedAt, sent)
	}
	if messages[1].CreatedAt == nil {
		t.Fatal("the final answer should get a persisted time")
	}
	if messages[1].DurationMs < 1000 {
		t.Fatalf("duration = %d, want the turn length", messages[1].DurationMs)
	}
}
