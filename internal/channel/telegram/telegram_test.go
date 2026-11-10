package telegram

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTelegramMessagesDisableLinkPreviews(t *testing.T) {
	send := telegramSendBody(1, 0, 2, "https://example.com", "markdown")
	if !send.DisableWebPagePreview {
		t.Fatal("send message did not set disable_web_page_preview")
	}
	if send.LinkPreviewOptions == nil || !send.LinkPreviewOptions.IsDisabled {
		t.Fatal("send message did not disable link preview options")
	}
	if send.ReplyToMessageID != 2 || send.ParseMode != "markdown" {
		t.Fatalf("send body fields were not preserved: %+v", send)
	}

	edit := telegramEditBody(1, 0, 0, 9, "https://example.com", "HTML")
	if !edit.DisableWebPagePreview {
		t.Fatal("edit message did not set disable_web_page_preview")
	}
	if edit.LinkPreviewOptions == nil || !edit.LinkPreviewOptions.IsDisabled {
		t.Fatal("edit message did not disable link preview options")
	}
	if edit.MessageID != 9 || edit.ParseMode != "HTML" {
		t.Fatalf("edit body fields were not preserved: %+v", edit)
	}

	payload, err := json.Marshal(send)
	if err != nil {
		t.Fatalf("marshal send body: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, `"disable_web_page_preview":true`) {
		t.Fatalf("send payload is missing disable_web_page_preview: %s", text)
	}
	if !strings.Contains(text, `"link_preview_options":{"is_disabled":true}`) {
		t.Fatalf("send payload is missing link_preview_options: %s", text)
	}
	// Thread 0 means the chat's default thread, so the field must be omitted.
	if strings.Contains(text, "message_thread_id") {
		t.Fatalf("send payload should omit message_thread_id for thread 0: %s", text)
	}
}

func TestTelegramMessagesCarryTopicThread(t *testing.T) {
	send := telegramSendBody(-1001111, 77, 5, "hello", "")
	if send.MessageThreadID != 77 {
		t.Fatalf("send thread = %d, want 77", send.MessageThreadID)
	}
	payload, err := json.Marshal(send)
	if err != nil {
		t.Fatalf("marshal send body: %v", err)
	}
	if !strings.Contains(string(payload), `"message_thread_id":77`) {
		t.Fatalf("send payload is missing message_thread_id: %s", payload)
	}

	edit := telegramEditBody(-1001111, 77, 0, 9, "hello", "")
	if edit.MessageThreadID != 77 {
		t.Fatalf("edit thread = %d, want 77", edit.MessageThreadID)
	}
	payload, err = json.Marshal(edit)
	if err != nil {
		t.Fatalf("marshal edit body: %v", err)
	}
	if !strings.Contains(string(payload), `"message_thread_id":77`) {
		t.Fatalf("edit payload is missing message_thread_id: %s", payload)
	}
}
