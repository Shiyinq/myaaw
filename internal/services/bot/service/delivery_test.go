package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
)

// Streaming surfaces (Telegram) send photos from the attachments carried by
// chunks, so a delivered file must arrive in a chunk even though it is only
// known after the final text. This regressed once: the chunk was emitted before
// delivery directives were resolved.
func TestTelegramStreamReceivesPromotedPhoto(t *testing.T) {
	previous := agentdir.Active()
	if err := agentdir.SetActive("default"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agentdir.SetActive(previous) })

	answerText := "Ini fotonya:\n\n"
	answer := provider.Message{Role: "assistant", Content: answerText}
	ag := &checkpointAgent{
		transcript: []provider.Message{
			{Role: "system", Content: "prompt"},
			{Role: "user", Content: "kirim foto"},
			answer,
		},
		final: answer,
	}
	// The helper owns the temp HOME, so files must be created after it runs.
	svc, repo := newCheckpointService(t, ag)

	agentHome, err := agentdir.Home("default")
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(agentHome, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	photo := filepath.Join(workspace, "photo.jpg")
	if err := os.WriteFile(photo, []byte("JPEGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	ag.transcript[2].Content = answerText + photo + "\n"
	ag.final.Content = ag.transcript[2].Content

	var chunks []channel.StreamChunk
	out, err := svc.runConversation(context.Background(), &model.User{UserId: 1}, &channel.IncomingMessage{
		UserID:         1,
		Text:           "kirim foto",
		Channel:        "telegram",
		ConversationID: "conv-1",
	}, func(chunk channel.StreamChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("runConversation: %v", err)
	}

	if len(out.Attachments) != 1 {
		t.Fatalf("attachments = %+v (roots=%+v, text=%q)", out.Attachments, attachmentRoots(), provider.MessageText(ag.final))
	}
	if got := string(out.Attachments[0].Data); got != "JPEGDATA" {
		t.Fatalf("attachment bytes = %q", got)
	}

	streamed := 0
	for _, chunk := range chunks {
		if chunk.Kind == "attachment" {
			streamed += len(chunk.Attachments)
		}
	}
	if streamed != 1 {
		t.Fatalf("streamed attachments = %d, want 1", streamed)
	}

	// The dashboard renders history from stored metadata, so the messaging path
	// must persist the virtual path even though the platform received bytes.
	if len(repo.updates) == 0 {
		t.Fatal("conversation was never saved")
	}
	saved := repo.updates[len(repo.updates)-1].messages
	if len(saved) == 0 {
		t.Fatal("saved transcript is empty")
	}
	last := saved[len(saved)-1]
	if len(last.Attachments) != 1 {
		t.Fatalf("persisted attachments = %+v", last.Attachments)
	}
	if last.Attachments[0].Path != "/home/workspace/photo.jpg" || last.Attachments[0].FileName != "photo.jpg" {
		t.Fatalf("persisted attachment = %+v", last.Attachments[0])
	}
}
