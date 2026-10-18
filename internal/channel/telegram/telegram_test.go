package telegram

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTelegramMessagesDisableLinkPreviews(t *testing.T) {
	send := telegramSendBody(1, 2, "https://example.com", "markdown")
	if !send.DisableWebPagePreview {
		t.Fatal("send message did not set disable_web_page_preview")
	}
	if send.LinkPreviewOptions == nil || !send.LinkPreviewOptions.IsDisabled {
		t.Fatal("send message did not disable link preview options")
	}
	if send.ReplyToMessageID != 2 || send.ParseMode != "markdown" {
		t.Fatalf("send body fields were not preserved: %+v", send)
	}

	edit := telegramEditBody(1, 0, 9, "https://example.com", "HTML")
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
}
