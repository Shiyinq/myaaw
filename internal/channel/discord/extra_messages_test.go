package discord

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"

	"myaaw/internal/channel"
	"myaaw/internal/chatdb"
	"myaaw/internal/provider"
)

func openDiscordTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE conversations (
		id TEXT PRIMARY KEY,
		user_id INTEGER,
		title TEXT,
		messages TEXT,
		active BOOLEAN,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	if err != nil {
		t.Fatalf("create conversations: %v", err)
	}
	if err := chatdb.EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return db
}

type fakeDiscordQueue struct {
	published [][]byte
}

func (f *fakeDiscordQueue) PublishMessage(body []byte) error {
	f.published = append(f.published, append([]byte(nil), body...))
	return nil
}

func TestDiscordBuildBlocksAndButtonInteractionLock(t *testing.T) {
	db := openDiscordTestDB(t)

	extras := []provider.ExtraMessage{
		{
			ID: "em_d1",
			Blocks: []provider.ExtraBlock{
				{
					ID:          "blk_btn",
					Type:        "buttons",
					Interactive: true,
					Data: json.RawMessage(`{
						"prompt": "Pick an action",
						"buttons": [
							{"id": "ok", "label": "Approve", "type": "action", "value": "approved", "style": "primary"},
							{"id": "url", "label": "Docs", "type": "link", "url": "https://example.com"}
						]
					}`),
					State: &provider.InteractionState{Status: "pending"},
				},
				{
					ID:          "blk_form",
					Type:        "form",
					Interactive: true,
					Data: json.RawMessage(`{
						"id": "f1",
						"title": "Quick Survey",
						"fields": [
							{"name": "topic", "label": "Topic", "type": "text", "required": true},
							{"name": "notes", "label": "Notes", "type": "textarea"}
						]
					}`),
					State: &provider.InteractionState{Status: "pending"},
				},
				{
					ID:   "blk_summary",
					Type: "summary",
					Data: json.RawMessage(`{"title": "Metrics", "items": [{"label": "CPU", "value": "12%"}]}`),
				},
			},
		},
	}

	msgs := []provider.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "here", ExtraMessages: extras},
	}
	rawMsgs, _ := json.Marshal(msgs)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?)`,
		"conv-dc-1", 555, "dc", string(rawMsgs), now, now,
	); err != nil {
		t.Fatalf("insert conv: %v", err)
	}

	var sent []*discordgo.MessageSend
	err := sendDiscordExtraMessagesWithSender(db, "chan-1", 555, "conv-dc-1", extras, nil, func(msg *discordgo.MessageSend) error {
		sent = append(sent, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("sendDiscordExtraMessagesWithSender: %v", err)
	}
	if len(sent) != 3 {
		t.Fatalf("expected 3 Discord messages (buttons, form button, summary embed), got %d", len(sent))
	}

	// Verify button CustomID and form trigger button CustomID.
	row := sent[0].Components[0].(discordgo.ActionsRow)
	actionBtn := row.Components[0].(discordgo.Button)
	if !strings.HasPrefix(actionBtn.CustomID, discordExtraButtonPrefix) {
		t.Fatalf("expected em: CustomID, got %q", actionBtn.CustomID)
	}
	formRow := sent[1].Components[0].(discordgo.ActionsRow)
	formBtn := formRow.Components[0].(discordgo.Button)
	if !strings.HasPrefix(formBtn.CustomID, discordExtraFormPrefix) {
		t.Fatalf("expected em_form: CustomID, got %q", formBtn.CustomID)
	}
	if len(sent[2].Embeds) != 1 || len(sent[2].Embeds[0].Fields) != 1 {
		t.Fatalf("expected summary embed with 1 field, got %+v", sent[2].Embeds)
	}

	// Simulate clicking the button and verify single-use lock + queue publish.
	q := &fakeDiscordQueue{}
	ic := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type:      discordgo.InteractionMessageComponent,
			ChannelID: "chan-1",
			User:      &discordgo.User{ID: "555", Username: "bob"},
		},
	}
	key := strings.TrimPrefix(actionBtn.CustomID, discordExtraButtonPrefix)
	resolveDiscordButtonInteraction(db, nil, ic, q, key)
	if len(q.published) != 1 {
		t.Fatalf("expected 1 published interaction message, got %d", len(q.published))
	}
	if !strings.Contains(string(q.published[0]), "interaction") || !strings.Contains(string(q.published[0]), "approved") {
		t.Fatalf("unexpected published payload: %s", string(q.published[0]))
	}

	// Duplicate click must not publish again.
	resolveDiscordButtonInteraction(db, nil, ic, q, key)
	if len(q.published) != 1 {
		t.Fatalf("duplicate click should be blocked; got %d published messages", len(q.published))
	}
}

func TestDiscordCarouselLocalImageAndModalNumberParsing(t *testing.T) {
	db := openDiscordTestDB(t)

	extras := []provider.ExtraMessage{
		{
			ID: "em_car",
			Blocks: []provider.ExtraBlock{
				{
					ID:   "blk_car",
					Type: "carousel",
					Data: json.RawMessage(`{"cards":[{"id":"c1","title":"Local Card","image_path":"/home/pic.png"}]}`),
				},
				{
					ID:          "blk_num_form",
					Type:        "form",
					Interactive: true,
					Data: json.RawMessage(`{
						"id": "f_num",
						"title": "Quota",
						"fields": [{"name":"count","label":"Count","type":"number","min":1,"max":10,"required":true}]
					}`),
					State: &provider.InteractionState{Status: "pending"},
				},
			},
		},
	}

	msgs := []provider.Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", Content: "ok", ExtraMessages: extras},
	}
	rawMsgs, _ := json.Marshal(msgs)
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?)`,
		"conv-dc-2", 777, "dc2", string(rawMsgs), now, now,
	)

	remaining, carouselFiles := splitDiscordCarouselLocalAttachments([]channel.Attachment{
		{FileName: "pic.png", MimeType: "image/png", Path: "/home/pic.png", Data: []byte("pngbytes")},
		{FileName: "other.pdf", MimeType: "application/pdf", Path: "/home/other.pdf", Data: []byte("pdfbytes")},
	}, extras)
	if len(remaining) != 1 || remaining[0].FileName != "other.pdf" {
		t.Fatalf("expected only other.pdf remaining, got %+v", remaining)
	}

	var sent []*discordgo.MessageSend
	if err := sendDiscordExtraMessagesWithSender(db, "chan-2", 777, "conv-dc-2", extras, carouselFiles, func(msg *discordgo.MessageSend) error {
		sent = append(sent, msg)
		return nil
	}); err != nil {
		t.Fatalf("sendDiscordExtraMessagesWithSender: %v", err)
	}
	if len(sent[0].Files) != 1 || sent[0].Embeds[0].Image == nil || sent[0].Embeds[0].Image.URL != "attachment://card_1.png" {
		t.Fatalf("expected carousel embed image attached locally, got embeds=%+v files=%d", sent[0].Embeds, len(sent[0].Files))
	}

	formRow := sent[1].Components[0].(discordgo.ActionsRow)
	formBtn := formRow.Components[0].(discordgo.Button)
	key := strings.TrimPrefix(formBtn.CustomID, discordExtraFormPrefix)

	q := &fakeDiscordQueue{}
	badModal := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type:      discordgo.InteractionModalSubmit,
			ChannelID: "chan-2",
			User:      &discordgo.User{ID: "777", Username: "eve"},
			Data: discordgo.ModalSubmitInteractionData{
				CustomID: discordExtraModalPrefix + key + ":orig_msg_1",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.TextInput{CustomID: "count", Value: "not-a-number"},
					}},
				},
			},
		},
	}
	resolveDiscordFormModalSubmit(db, nil, badModal, q, key+":orig_msg_1")
	if len(q.published) != 0 {
		t.Fatalf("invalid number should be rejected without publishing")
	}

	goodModal := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type:      discordgo.InteractionModalSubmit,
			ChannelID: "chan-2",
			User:      &discordgo.User{ID: "777", Username: "eve"},
			Data: discordgo.ModalSubmitInteractionData{
				CustomID: discordExtraModalPrefix + key + ":orig_msg_1",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.TextInput{CustomID: "count", Value: "5"},
					}},
				},
			},
		},
	}
	resolveDiscordFormModalSubmit(db, nil, goodModal, q, key+":orig_msg_1")
	if len(q.published) != 1 || !strings.Contains(string(q.published[0]), `\"count\":5`) && !strings.Contains(string(q.published[0]), `"count":5`) {
		t.Fatalf("expected parsed numeric value 5 published, got %d %s", len(q.published), string(q.published[0]))
	}
}
