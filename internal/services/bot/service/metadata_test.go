package service

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"myaaw/internal/channel"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
)

func TestBotCheckpointsAndFinalPersistencePreserveContinuation(t *testing.T) {
	metadata := map[string]json.RawMessage{"openai_chatgpt": json.RawMessage(`{"output":[{"type":"reasoning","encrypted_content":"encrypted-only","summary":[]}]}`)}
	call := provider.Message{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "call_1", Type: "function", Function: provider.FunctionCall{Name: "lookup", Arguments: `{}`}}}, ProviderMetadata: metadata}
	final := provider.Message{Role: "assistant", Content: "answer", ProviderMetadata: metadata}
	ag := &checkpointAgent{transcript: []provider.Message{
		{Role: "system", Content: "system"}, {Role: "user", Content: "question"}, call,
		{Role: "tool", ToolCallID: "call_1", Content: "result"}, final,
	}, final: final}
	svc, repo := newCheckpointService(t, ag)
	msg := &channel.IncomingMessage{UserID: 1, ConversationID: "conv-1", Channel: "api", Text: "question"}
	out, err := svc.runConversation(context.Background(), &model.User{UserId: 1}, msg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "answer" || out.Thought != "" {
		t.Fatal("opaque continuation leaked into display content")
	}
	if len(repo.updates) < 2 {
		t.Fatal("expected both checkpoint and final persistence")
	}
	for _, update := range repo.updates {
		if len(update.messages) != 4 {
			t.Fatalf("saved %d messages", len(update.messages))
		}
		for _, index := range []int{1, 3} {
			if !reflect.DeepEqual(update.messages[index].ProviderMetadata, metadata) {
				t.Fatal("saved continuation lost")
			}
		}
	}
	// Loading the persisted history for a new turn must not reconstruct assistant
	// messages from only their display text or Trace.
	repo.conversations["conv-1"].Messages = repo.updates[len(repo.updates)-1].messages
	history, err := svc.buildConversationMessages(&model.User{UserId: 1}, &channel.IncomingMessage{UserID: 1, ConversationID: "conv-1", Channel: "api", Text: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(history[2].ProviderMetadata, metadata) || !reflect.DeepEqual(history[4].ProviderMetadata, metadata) {
		t.Fatal("reloaded history lost continuation")
	}
	// Persisted copies must own both the map and raw bytes independently.
	repo.updates[0].messages[1].ProviderMetadata["openai_chatgpt"][0] = '!'
	delete(repo.updates[0].messages[1].ProviderMetadata, "openai_chatgpt")
	if !reflect.DeepEqual(ag.transcript[2].ProviderMetadata, metadata) || !reflect.DeepEqual(repo.updates[len(repo.updates)-1].messages[1].ProviderMetadata, metadata) {
		t.Fatal("persisted copy aliases another transcript")
	}
}
