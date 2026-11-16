package service

import (
	"encoding/base64"
	"strings"
	"testing"

	"myaaw/internal/channel"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
)

// TestBuildConversationMessagesKeepsEveryBatchedMessage proves a spam burst
// stays readable: every message of the batch is its own user entry with its own
// uploads, while the run answers the whole batch once.
func TestBuildConversationMessagesKeepsEveryBatchedMessage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n-first-photo"))
	repo := newFakeConversationRepo()
	svc := &BotServiceImpl{
		conversationRepo: repo,
		llmProvider:      provider.NewOpenAIProvider("http://127.0.0.1:1", "sk-test", "test-model"),
	}

	turn := func(text string, images []string) *channel.IncomingMessage {
		return &channel.IncomingMessage{
			UserID:  1,
			Channel: "telegram",
			ChatID:  "42",
			Text:    text,
			Images:  images,
		}
	}

	primary := turn("first caption", []string{png})
	primary.Batch = []*channel.IncomingMessage{
		turn("second caption", []string{png}),
		turn("third caption", nil),
	}

	messages, err := svc.buildConversationMessages(&model.User{UserId: 1}, primary)
	if err != nil {
		t.Fatalf("buildConversationMessages: %v", err)
	}

	var userMessages []provider.Message
	for _, msg := range messages {
		if msg.Role == "user" {
			userMessages = append(userMessages, msg)
		}
	}
	if len(userMessages) != 3 {
		t.Fatalf("user entries = %d, want one per batched message", len(userMessages))
	}

	// Separate bubbles: every entry has its own id so the dashboard can show the
	// uploads under the message they came from.
	seen := make(map[string]bool, len(userMessages))
	wantCaptions := []string{"first caption", "second caption", "third caption"}
	for i, want := range wantCaptions {
		entry := userMessages[i]
		if entry.ID == "" || seen[entry.ID] {
			t.Fatalf("entry %d id = %q (duplicate=%v)", i, entry.ID, seen[entry.ID])
		}
		seen[entry.ID] = true

		text := provider.StripContextBlocks(provider.MessageText(entry))
		if !strings.Contains(text, want) {
			t.Fatalf("entry %d text = %q, want it to carry %q", i, text, want)
		}
	}

	// Photos stay attached to the message that sent them, so five images across
	// five chats remain five captioned bubbles.
	if len(userMessages[0].Uploads) != 1 || len(userMessages[1].Uploads) != 1 {
		t.Fatalf("upload counts = %d/%d, want both captioned messages to keep their photo",
			len(userMessages[0].Uploads), len(userMessages[1].Uploads))
	}
	if len(userMessages[2].Uploads) != 0 {
		t.Fatalf("text-only entry carries %d uploads", len(userMessages[2].Uploads))
	}

	// The OpenAI shape sends images inline, so every photo-bearing message must
	// carry its own image item.
	for i := 0; i < 2; i++ {
		items, ok := userMessages[i].Content.([]provider.ContentItem)
		if !ok {
			t.Fatalf("entry %d content = %T, want inline content items", i, userMessages[i].Content)
		}
		images := 0
		for _, item := range items {
			if item.Type == "image_url" {
				images++
			}
		}
		if images != 1 {
			t.Fatalf("entry %d inline images = %d, want 1", i, images)
		}
	}
}

// TestTurnTextJoinsBatchedPrompts keeps traces and titles readable for a burst.
func TestTurnTextJoinsBatchedPrompts(t *testing.T) {
	primary := &channel.IncomingMessage{Text: "first"}
	primary.Batch = []*channel.IncomingMessage{
		{Text: " second "},
		{Text: "   "},
		{
			Text: "third",
			Batch: []*channel.IncomingMessage{
				{Text: "nested"},
			},
		},
	}

	// Only the direct batch is part of the turn; batching happens at the queue
	// edge, so a nested batch is not expected and not traversed.
	if got := primary.TurnText(); got != "first\nsecond\nthird" {
		t.Fatalf("TurnText = %q", got)
	}
	if got := (&channel.IncomingMessage{}).TurnText(); got != "" {
		t.Fatalf("empty TurnText = %q", got)
	}
}
