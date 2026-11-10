package repository

import (
	"encoding/json"
	"reflect"
	"testing"

	"myaaw/internal/provider"
)

func TestConversationRoundTripPreservesToolContinuationMetadata(t *testing.T) {
	repo := newTestConversationRepo(t)
	conv, err := repo.CreateConversationWithId(1, "continuation", "api", "Existing chat")
	if err != nil {
		t.Fatal(err)
	}
	messages := []provider.Message{
		{Role: "user", Content: "look it up", Version: 2},
		{Role: "assistant", Version: 2, ProviderMetadata: map[string]json.RawMessage{"openai_chatgpt": json.RawMessage(`{"output":[{"type":"reasoning","id":"rs_1","encrypted_content":"opaque-encrypted","summary":[]},{"type":"function_call","call_id":"call_1","name":"lookup","namespace":"myaaw","arguments":"{}"}]}`)}, ToolCalls: []provider.ToolCall{{ID: "call_1", Type: "function", Function: provider.FunctionCall{Name: "lookup", Arguments: `{}`}}}},
		{Role: "tool", ToolCallID: "call_1", Name: "lookup", Content: "found it", Version: 2},
	}
	if err := repo.UpdateConversationById(conv.Id, messages, ""); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetConversationById(conv.Id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.Messages, messages) {
		t.Fatal("SQLite JSON round trip changed the content/tool-call/metadata pair")
	}
	// A second save/load also exercises raw JSON decoded from storage, rather
	// than only metadata produced in memory by a provider.
	if err := repo.UpdateConversationById(conv.Id, stored.Messages, ""); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.GetConversationById(conv.Id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.Messages, messages) {
		t.Fatal("second round trip lost continuation")
	}
}
