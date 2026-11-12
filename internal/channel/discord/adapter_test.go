package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"myaaw/internal/channel"

	"github.com/bwmarrin/discordgo"
)

func TestOpenGatewayRetriesThenSucceeds(t *testing.T) {
	attempts := 0
	open := func() error {
		attempts++
		if attempts < 3 {
			return errors.New("network unreachable")
		}
		return nil
	}

	var sleeps []time.Duration
	sleep := func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}

	if err := openGateway(context.Background(), open, sleep); err != nil {
		t.Fatalf("openGateway: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if len(sleeps) != 2 || sleeps[0] != time.Second || sleeps[1] != 2*time.Second {
		t.Fatalf("backoff = %v, want [1s 2s]", sleeps)
	}
}

func TestOpenGatewayStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	open := func() error {
		attempts++
		return errors.New("network unreachable")
	}
	sleep := func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}

	err := openGateway(ctx, open, sleep)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestOpenGatewayGivesUpOnAuthError(t *testing.T) {
	open := func() error {
		return errors.New("websocket: close 4004: Authentication failed")
	}
	sleep := func(context.Context, time.Duration) error {
		t.Fatal("authentication failures must not be retried")
		return nil
	}

	if err := openGateway(context.Background(), open, sleep); err == nil {
		t.Fatal("expected an authentication error")
	}
}

func TestOpenGatewayReturnsErrorWhenContextAlreadyDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	open := func() error {
		t.Fatal("open must not run after cancellation")
		return nil
	}

	if err := openGateway(ctx, open, sleepContext); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestProtectRecoversFromPanic(t *testing.T) {
	protect("test handler", func() {
		panic("boom")
	})
}

func TestSendErrorWithoutTransportReturnsError(t *testing.T) {
	adapter := &DiscordAdapter{}
	msg := &channel.IncomingMessage{RawMeta: DiscordMeta{ChannelID: "chan"}}

	if err := adapter.SendError(msg, "failed"); err == nil {
		t.Fatal("expected an error when no transport is configured")
	}
}

func TestSendErrorRejectsUnexpectedMetadata(t *testing.T) {
	adapter := &DiscordAdapter{transport: &scriptedTransport{}}
	msg := &channel.IncomingMessage{RawMeta: "not-discord"}

	if err := adapter.SendError(msg, "failed"); err == nil {
		t.Fatal("expected an error for unexpected metadata")
	}
}

// decodePayload unmarshals a queue payload so tests can assert the wire format
// that ParseIncoming (and messageQueueKey) depend on.
func decodePayload(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return data
}

func lookup(t *testing.T, data map[string]any, keys ...string) any {
	t.Helper()
	var current any = data
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("path %v: %q is not an object", keys, key)
		}
		current, ok = m[key]
		if !ok {
			t.Fatalf("path %v: missing %q", keys, key)
		}
	}
	return current
}

func guildMessage() *discordgo.MessageCreate {
	return &discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "m1",
		ChannelID: "999888",
		GuildID:   "777",
		Content:   "hello there",
		Author: &discordgo.User{
			ID:         "456",
			Username:   "yin",
			GlobalName: "Yin",
		},
	}}
}

func TestBuildMessagePayloadGuildChannel(t *testing.T) {
	raw := buildMessagePayload(
		guildMessage(),
		&discordgo.Channel{ID: "999888", Name: "general", GuildID: "777"},
		&discordgo.Guild{ID: "777", Name: "Myaaw HQ"},
		[]string{"data:image/png;base64,xx"},
	)
	data := decodePayload(t, raw)

	if got := lookup(t, data, "message", "chat", "type"); got != "guild" {
		t.Fatalf("chat.type = %v, want guild", got)
	}
	if got := lookup(t, data, "message", "chat", "name"); got != "general" {
		t.Fatalf("chat.name = %v, want general", got)
	}
	if got := lookup(t, data, "message", "guild", "name"); got != "Myaaw HQ" {
		t.Fatalf("guild.name = %v, want Myaaw HQ", got)
	}
	if got := lookup(t, data, "message", "from", "first_name"); got != "Yin" {
		t.Fatalf("from.first_name = %v, want Yin", got)
	}
	if got := lookup(t, data, "discord_author", "username"); got != "yin" {
		t.Fatalf("discord_author.username = %v, want yin", got)
	}
	if got := lookup(t, data, "original_source"); got != "discord" {
		t.Fatalf("original_source = %v, want discord", got)
	}
	if got := lookup(t, data, "message", "chat", "is_thread"); got != false {
		t.Fatalf("chat.is_thread = %v, want false", got)
	}
}

func TestBuildMessagePayloadThread(t *testing.T) {
	msg := guildMessage()
	msg.ChannelID = "999777"

	raw := buildMessagePayload(
		msg,
		&discordgo.Channel{ID: "999777", Name: "bug-report", GuildID: "777", Type: discordgo.ChannelTypeGuildPublicThread},
		&discordgo.Guild{ID: "777", Name: "Myaaw HQ"},
		nil,
	)
	data := decodePayload(t, raw)

	if got := lookup(t, data, "message", "chat", "is_thread"); got != true {
		t.Fatalf("chat.is_thread = %v, want true", got)
	}

	parsed, err := (&DiscordAdapter{}).ParseIncoming(raw)
	if err != nil {
		t.Fatalf("ParseIncoming: %v", err)
	}
	if !parsed.IsThread || parsed.ChatID != "999777" {
		t.Fatalf("parsed thread = %v / %q", parsed.IsThread, parsed.ChatID)
	}
}

func TestBuildMessagePayloadDMHasNoChannelName(t *testing.T) {
	msg := guildMessage()
	msg.GuildID = ""
	msg.ChannelID = "999888"

	// DM channel objects also carry a Name for the recipient; it must never be
	// mistaken for a guild channel name in the dashboard.
	raw := buildMessagePayload(msg, &discordgo.Channel{ID: "999888", Name: "yin"}, nil, nil)
	data := decodePayload(t, raw)

	if got := lookup(t, data, "message", "chat", "type"); got != "dm" {
		t.Fatalf("chat.type = %v, want dm", got)
	}
	if got := lookup(t, data, "message", "chat", "name"); got != "" {
		t.Fatalf("chat.name = %v, want empty for a DM", got)
	}
}

func TestBuildMessagePayloadToleratesMissingState(t *testing.T) {
	// Threads are not always cached, so names may be unavailable while the
	// channel id (and therefore the scope) stays intact.
	raw := buildMessagePayload(guildMessage(), nil, nil, nil)
	data := decodePayload(t, raw)

	if got := lookup(t, data, "message", "chat", "name"); got != "" {
		t.Fatalf("chat.name = %v, want empty", got)
	}
	if got := lookup(t, data, "message", "guild", "name"); got != "" {
		t.Fatalf("guild.name = %v, want empty", got)
	}
}

func TestParseIncomingGuildMessage(t *testing.T) {
	raw := buildMessagePayload(
		guildMessage(),
		&discordgo.Channel{ID: "999888", Name: "general", GuildID: "777"},
		&discordgo.Guild{ID: "777", Name: "Myaaw HQ"},
		nil,
	)

	msg, err := (&DiscordAdapter{}).ParseIncoming(raw)
	if err != nil {
		t.Fatalf("ParseIncoming: %v", err)
	}

	if msg.Scope != "dc:999888" {
		t.Fatalf("Scope = %q, want dc:999888", msg.Scope)
	}
	if msg.ChatName != "Myaaw HQ" {
		t.Fatalf("ChatName = %q, want Myaaw HQ", msg.ChatName)
	}
	if msg.TopicName != "general" {
		t.Fatalf("TopicName = %q, want general", msg.TopicName)
	}
	if msg.ChatType != "guild" || msg.ChatID != "999888" || msg.IsThread {
		t.Fatalf("chat = %q / %q / thread %v", msg.ChatType, msg.ChatID, msg.IsThread)
	}
	if msg.SenderID != "456" || msg.SenderName != "Yin" || msg.SenderUsername != "yin" {
		t.Fatalf("sender = %q/%q/%q, want 456/Yin/yin", msg.SenderID, msg.SenderName, msg.SenderUsername)
	}
	if msg.UserID != 456 {
		t.Fatalf("UserID = %d, want 456", msg.UserID)
	}
	if msg.Text != "hello there" {
		t.Fatalf("Text = %q, want hello there", msg.Text)
	}
	meta, ok := msg.RawMeta.(DiscordMeta)
	if !ok || meta.ChannelID != "999888" || meta.MessageID != "m1" {
		t.Fatalf("RawMeta = %#v, want channel 999888 message m1", msg.RawMeta)
	}
}

func TestParseIncomingDMMessageKeepsLegacyScope(t *testing.T) {
	msg := guildMessage()
	msg.GuildID = ""

	raw := buildMessagePayload(msg, &discordgo.Channel{ID: "999888", Name: "yin"}, nil, nil)
	parsed, err := (&DiscordAdapter{}).ParseIncoming(raw)
	if err != nil {
		t.Fatalf("ParseIncoming: %v", err)
	}

	if parsed.Scope != "" {
		t.Fatalf("Scope = %q, want empty for a DM", parsed.Scope)
	}
	if parsed.ChatName != "" || parsed.TopicName != "" {
		t.Fatalf("names = %q/%q, want both empty for a DM", parsed.ChatName, parsed.TopicName)
	}
	if parsed.ChatType != "dm" || parsed.ChatID != "999888" || parsed.IsThread {
		t.Fatalf("chat = %q / %q / thread %v", parsed.ChatType, parsed.ChatID, parsed.IsThread)
	}
}

func TestParseIncomingLegacyPayloadHasNoScope(t *testing.T) {
	// Payloads queued by older builds have no chat.type; they must stay on the
	// legacy per-user session instead of inventing a scope.
	raw := []byte(`{"message":{"text":"hi","chat":{"id":"999888"}},"original_source":"discord","discord_author":{"id":"456","username":"yin"}}`)

	msg, err := (&DiscordAdapter{}).ParseIncoming(raw)
	if err != nil {
		t.Fatalf("ParseIncoming: %v", err)
	}
	if msg.Scope != "" {
		t.Fatalf("Scope = %q, want empty for a legacy payload", msg.Scope)
	}
	if msg.SenderName != "yin" {
		t.Fatalf("SenderName = %q, want yin", msg.SenderName)
	}
	if msg.ChatType != "" || msg.ChatID != "999888" || msg.IsThread {
		t.Fatalf("legacy chat = %q / %q / thread %v", msg.ChatType, msg.ChatID, msg.IsThread)
	}
}

func TestDiscordFileBatchesSplitAtTen(t *testing.T) {
	files := make([]*discordgo.File, 20)
	for i := range files {
		files[i] = &discordgo.File{Name: fmt.Sprintf("f%d", i)}
	}

	batches := discordFileBatches(files)
	if len(batches) != 2 {
		t.Fatalf("batches = %d, want 2", len(batches))
	}
	if len(batches[0]) != 10 || len(batches[1]) != 10 {
		t.Fatalf("batch sizes = %d/%d, want 10/10", len(batches[0]), len(batches[1]))
	}

	small := discordFileBatches(files[:3])
	if len(small) != 1 || len(small[0]) != 3 {
		t.Fatalf("small batches = %v, want one batch of 3", small)
	}
}
