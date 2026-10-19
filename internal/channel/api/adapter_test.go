package api

import "testing"

func TestParseIncomingMapsConversationID(t *testing.T) {
	adapter := NewAPIAdapter()

	msg, err := adapter.ParseIncoming([]byte(`{"user_id":7,"text":"hi","conversation_id":"conv-1"}`))
	if err != nil {
		t.Fatalf("ParseIncoming: %v", err)
	}
	if msg.ConversationID != "conv-1" {
		t.Fatalf("conversation id = %q", msg.ConversationID)
	}

	msg, err = adapter.ParseIncoming([]byte(`{"user_id":7,"text":"hi","conversation_id":"NEW"}`))
	if err != nil {
		t.Fatalf("ParseIncoming NEW: %v", err)
	}
	if msg.ConversationID != "NEW" {
		t.Fatalf("conversation id = %q", msg.ConversationID)
	}

	msg, err = adapter.ParseIncoming([]byte(`{"user_id":7,"text":"hi"}`))
	if err != nil {
		t.Fatalf("ParseIncoming without id: %v", err)
	}
	if msg.ConversationID != "NEW" {
		t.Fatalf("missing conversation id should start a new session, got %q", msg.ConversationID)
	}
}
