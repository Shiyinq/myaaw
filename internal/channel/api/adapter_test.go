package api

import (
	"strings"
	"testing"
)

func TestParseIncomingImages(t *testing.T) {
	adapter := NewAPIAdapter()

	// One 1x1 PNG, base64 as the dashboard sends it.
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

	// Images alone are a valid message; text is optional when images are present.
	msg, err := adapter.ParseIncoming([]byte(`{"images":["` + png + `","` + png + `"]}`))
	if err != nil {
		t.Fatalf("ParseIncoming images: %v", err)
	}
	if len(msg.Images) != 2 || msg.Images[0] != png || msg.Text != "" {
		t.Fatalf("message = %+v", msg)
	}

	if _, err := adapter.ParseIncoming([]byte(`{}`)); err == nil {
		t.Fatal("a message without text or images should be rejected")
	}

	// Only image data is accepted, so a video or arbitrary blob never gets through.
	if _, err := adapter.ParseIncoming([]byte(`{"images":["AAAA"]}`)); err == nil {
		t.Fatal("non-image data should be rejected")
	}

	tooMany := `{"images":["` + strings.Repeat(png+`","`, 6) + png + `"]}`
	if _, err := adapter.ParseIncoming([]byte(tooMany)); err == nil {
		t.Fatal("more than six images should be rejected")
	}
}

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

	msg, err = adapter.ParseIncoming([]byte(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("ParseIncoming without user_id: %v", err)
	}
	if msg.UserID != 0 {
		t.Fatalf("missing user_id should default to 0, got %d", msg.UserID)
	}
	if msg.ConversationID != "NEW" {
		t.Fatalf("missing conversation id should start a new session, got %q", msg.ConversationID)
	}
}
