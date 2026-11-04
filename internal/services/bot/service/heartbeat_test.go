package service

import (
	"strings"
	"testing"

	"myaaw/internal/channel/discord"
	"myaaw/internal/channel/telegram"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
)

func withOwnerIDs(t *testing.T, ids ...string) {
	t.Helper()
	prev := config.OwnerIDs
	config.OwnerIDs = ids
	t.Cleanup(func() { config.OwnerIDs = prev })
}

func TestHeartbeatScope(t *testing.T) {
	cases := []struct {
		name   string
		target HeartbeatTarget
		want   string
	}{
		{
			name:   "telegram dm is per user",
			target: HeartbeatTarget{Channel: "telegram", To: "123456789"},
			want:   "",
		},
		{
			name:   "telegram group",
			target: HeartbeatTarget{Channel: "telegram", To: "-1001234567890"},
			want:   "tg:-1001234567890",
		},
		{
			name:   "telegram general topic keeps the chat scope",
			target: HeartbeatTarget{Channel: "telegram", To: "-1001234567890", ThreadID: "1"},
			want:   "tg:-1001234567890",
		},
		{
			name:   "telegram forum topic",
			target: HeartbeatTarget{Channel: "telegram", To: "-1001234567890", ThreadID: "77"},
			want:   "tg:-1001234567890:77",
		},
		{
			name:   "discord channel",
			target: HeartbeatTarget{Channel: "discord", To: "222333444555666777"},
			want:   "dc:222333444555666777",
		},
		{
			name:   "discord dm is per user",
			target: HeartbeatTarget{Channel: "discord", To: "111222333444555666", ChatType: "dm"},
			want:   "",
		},
		{
			name:   "api is per user unless pinned",
			target: HeartbeatTarget{Channel: "api", To: "111222333444555666"},
			want:   "",
		},
		{
			name:   "cli is per user unless pinned",
			target: HeartbeatTarget{Channel: "cli", To: "0"},
			want:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := heartbeatScope(tc.target); got != tc.want {
				t.Fatalf("heartbeatScope(%+v) = %q, want %q", tc.target, got, tc.want)
			}
		})
	}
}

// A scheduled turn in a forum topic must look exactly like a live message in
// that topic: scope, ChatID/TopicID, thread flag, names, and the transport meta
// that carries the thread id to sendMessage.
func TestHeartbeatMessageTelegramTopic(t *testing.T) {
	withOwnerIDs(t, "123456789")
	repo := newFakeConversationRepo()
	repo.scopeNames["tg:-1001234567890:77"] = [2]string{"Example Group", "Daily Report"}
	svc := &BotServiceImpl{conversationRepo: repo}

	msg, err := svc.heartbeatMessage("Time to hydrate", HeartbeatTarget{
		Channel:  " Telegram ",
		To:       " -1001234567890 ",
		ThreadID: " 77 ",
	}, "cron")
	if err != nil {
		t.Fatalf("heartbeatMessage: %v", err)
	}

	if msg.Scope != "tg:-1001234567890:77" {
		t.Errorf("Scope = %q", msg.Scope)
	}
	if msg.ChatID != "-1001234567890" || msg.TopicID != "77" || !msg.IsThread {
		t.Errorf("room ids = chat %q topic %q thread %v", msg.ChatID, msg.TopicID, msg.IsThread)
	}
	if msg.ChatType != "supergroup" {
		t.Errorf("ChatType = %q", msg.ChatType)
	}
	if msg.UserID != 123456789 {
		t.Errorf("UserID = %d, want the configured owner", msg.UserID)
	}
	if msg.ChatName != "Example Group" || msg.TopicName != "Daily Report" {
		t.Errorf("names = %q / %q", msg.ChatName, msg.TopicName)
	}
	if msg.TriggerType != "cron" || msg.Text != "Time to hydrate" {
		t.Errorf("turn = %q / %q", msg.TriggerType, msg.Text)
	}

	meta, ok := msg.RawMeta.(telegram.TelegramMeta)
	if !ok {
		t.Fatalf("RawMeta = %T", msg.RawMeta)
	}
	if meta.ChatID != -1001234567890 || meta.ThreadID != 77 {
		t.Errorf("telegram meta = %+v", meta)
	}
}

func TestHeartbeatMessageTelegramDM(t *testing.T) {
	withOwnerIDs(t, "999999999")
	svc := &BotServiceImpl{conversationRepo: newFakeConversationRepo()}

	msg, err := svc.heartbeatMessage("Time to hydrate", HeartbeatTarget{
		Channel: "telegram",
		To:      "123456789",
	}, "cron")
	if err != nil {
		t.Fatalf("heartbeatMessage: %v", err)
	}
	if msg.Scope != "" {
		t.Errorf("Scope = %q, want the legacy empty DM scope", msg.Scope)
	}
	if msg.UserID != 123456789 {
		t.Errorf("UserID = %d, want the DM recipient", msg.UserID)
	}
	if msg.ChatType != "private" || msg.IsThread {
		t.Errorf("chat type = %q thread = %v", msg.ChatType, msg.IsThread)
	}
	if msg.TopicID != "" {
		t.Errorf("TopicID = %q", msg.TopicID)
	}
}

func TestHeartbeatMessageDiscordChannel(t *testing.T) {
	withOwnerIDs(t, "123456789")
	repo := newFakeConversationRepo()
	repo.scopeNames["dc:222333444555666777"] = [2]string{"Example Server", ""}
	svc := &BotServiceImpl{conversationRepo: repo}

	msg, err := svc.heartbeatMessage("Daily report", HeartbeatTarget{
		Channel: "discord",
		To:      "222333444555666777",
	}, "cron")
	if err != nil {
		t.Fatalf("heartbeatMessage: %v", err)
	}
	if msg.Scope != "dc:222333444555666777" || msg.ChatID != "222333444555666777" {
		t.Errorf("scope = %q chat = %q", msg.Scope, msg.ChatID)
	}
	if msg.UserID != 123456789 {
		t.Errorf("UserID = %d, want the configured owner", msg.UserID)
	}
	meta, ok := msg.RawMeta.(discord.DiscordMeta)
	if !ok || meta.ChannelID != "222333444555666777" || meta.IsDM {
		t.Fatalf("discord meta = %#v", msg.RawMeta)
	}
}

// A Discord DM targets a user id, not a channel id: ChatID must stay empty so
// delivery opens the DM channel instead of writing to a numeric channel.
func TestHeartbeatMessageDiscordDM(t *testing.T) {
	withOwnerIDs(t, "123456789")
	svc := &BotServiceImpl{conversationRepo: newFakeConversationRepo()}

	msg, err := svc.heartbeatMessage("Ping", HeartbeatTarget{
		Channel:  "discord",
		To:       "111222333444555666",
		ChatType: "DM",
	}, "cron")
	if err != nil {
		t.Fatalf("heartbeatMessage: %v", err)
	}
	if msg.Scope != "" || msg.ChatID != "" {
		t.Errorf("scope = %q chat = %q", msg.Scope, msg.ChatID)
	}
	if msg.UserID != 111222333444555666 {
		t.Errorf("UserID = %d, want the DM recipient", msg.UserID)
	}
	if msg.ChatType != "dm" {
		t.Errorf("ChatType = %q", msg.ChatType)
	}
	meta, ok := msg.RawMeta.(discord.DiscordMeta)
	if !ok || meta.UserID != "111222333444555666" || !meta.IsDM {
		t.Fatalf("discord meta = %#v", msg.RawMeta)
	}
}

// A pinned session is authoritative: its owner and scope win, and the turn is
// appended to that exact conversation.
func TestHeartbeatMessagePinnedSession(t *testing.T) {
	withOwnerIDs(t, "999999999")
	repo := newFakeConversationRepo()
	repo.conversations["0f4e8b3a-1111-4222-8333-944455556666"] = &model.Conversation{
		Id:     "0f4e8b3a-1111-4222-8333-944455556666",
		UserId: 111222333444555666,
		Scope:  "dc:222333444555666777",
		Active: true,
	}
	svc := &BotServiceImpl{conversationRepo: repo}

	msg, err := svc.heartbeatMessage("Reminder", HeartbeatTarget{
		Channel:   "api",
		SessionID: "0f4e8b3a-1111-4222-8333-944455556666",
	}, "cron")
	if err != nil {
		t.Fatalf("heartbeatMessage: %v", err)
	}
	if msg.ConversationID != "0f4e8b3a-1111-4222-8333-944455556666" {
		t.Errorf("ConversationID = %q", msg.ConversationID)
	}
	if msg.UserID != 111222333444555666 {
		t.Errorf("UserID = %d, want the conversation owner", msg.UserID)
	}
	if msg.Scope != "dc:222333444555666777" {
		t.Errorf("Scope = %q, want the conversation scope", msg.Scope)
	}
}

// The chat id sign is authoritative for Telegram: a job created in a DM but
// aimed at a group must not tell the model it is talking in a direct message.
func TestHeartbeatMessageTelegramDerivesChatTypeFromChatID(t *testing.T) {
	withOwnerIDs(t, "123456789")
	svc := &BotServiceImpl{conversationRepo: newFakeConversationRepo()}

	group, err := svc.heartbeatMessage("Reminder", HeartbeatTarget{
		Channel:  "telegram",
		To:       "-1001234567890",
		ChatType: "private",
	}, "cron")
	if err != nil {
		t.Fatalf("heartbeatMessage: %v", err)
	}
	if group.ChatType != "supergroup" {
		t.Errorf("group ChatType = %q, want supergroup", group.ChatType)
	}

	dm, err := svc.heartbeatMessage("Reminder", HeartbeatTarget{
		Channel:  "telegram",
		To:       "123456789",
		ChatType: "supergroup",
	}, "cron")
	if err != nil {
		t.Fatalf("heartbeatMessage: %v", err)
	}
	if dm.ChatType != "private" {
		t.Errorf("dm ChatType = %q, want private", dm.ChatType)
	}
}

// A pinned guild session is a guild room, even when the job inherited chat_type
// "dm" from the DM that created it.
func TestHeartbeatMessageDiscordPinnedSessionIsGuild(t *testing.T) {
	withOwnerIDs(t, "123456789")
	repo := newFakeConversationRepo()
	repo.conversations["2b3c4d5e-1111-4222-8333-944455556666"] = &model.Conversation{
		Id:     "2b3c4d5e-1111-4222-8333-944455556666",
		UserId: 123456789,
		Scope:  "dc:222333444555666777",
		Active: true,
	}
	svc := &BotServiceImpl{conversationRepo: repo}

	msg, err := svc.heartbeatMessage("Reminder", HeartbeatTarget{
		Channel:   "discord",
		To:        "222333444555666777",
		SessionID: "2b3c4d5e-1111-4222-8333-944455556666",
		ChatType:  "dm",
	}, "cron")
	if err != nil {
		t.Fatalf("heartbeatMessage: %v", err)
	}
	if msg.ChatType != "guild" {
		t.Errorf("ChatType = %q, want guild", msg.ChatType)
	}
	meta, ok := msg.RawMeta.(discord.DiscordMeta)
	if !ok || meta.IsDM || meta.ChannelID != "222333444555666777" {
		t.Fatalf("discord meta = %#v", msg.RawMeta)
	}
}

// A pinned Discord conversation without a scope is a DM, even when the job says
// guild: guild rooms always carry a dc:<channel> scope.
func TestHeartbeatMessageDiscordPinnedSessionWithoutScopeIsDM(t *testing.T) {
	withOwnerIDs(t, "123456789")
	repo := newFakeConversationRepo()
	repo.conversations["2b3c4d5e-1111-4222-8333-944455556666"] = &model.Conversation{
		Id:     "2b3c4d5e-1111-4222-8333-944455556666",
		UserId: 123456789,
		Active: true,
	}
	svc := &BotServiceImpl{conversationRepo: repo}

	msg, err := svc.heartbeatMessage("Reminder", HeartbeatTarget{
		Channel:   "discord",
		To:        "111222333444555666",
		SessionID: "2b3c4d5e-1111-4222-8333-944455556666",
		ChatType:  "guild",
	}, "cron")
	if err != nil {
		t.Fatalf("heartbeatMessage: %v", err)
	}
	if msg.ChatType != "dm" {
		t.Errorf("ChatType = %q, want dm", msg.ChatType)
	}
	meta, ok := msg.RawMeta.(discord.DiscordMeta)
	if !ok || !meta.IsDM || meta.UserID != "111222333444555666" {
		t.Fatalf("discord meta = %#v", msg.RawMeta)
	}
}

// Multi-agent guard: a session id that this agent's database does not know is
// rejected instead of silently starting a new conversation.
func TestHeartbeatMessageRejectsForeignSession(t *testing.T) {
	svc := &BotServiceImpl{conversationRepo: newFakeConversationRepo()}

	_, err := svc.heartbeatMessage("Reminder", HeartbeatTarget{
		Channel:   "api",
		SessionID: "1a2b3c4d-1111-4222-8333-944455556666",
	}, "cron")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want a missing-session error", err)
	}
}

func TestHeartbeatMessageRejectsInvalidTargets(t *testing.T) {
	cases := []struct {
		name   string
		target HeartbeatTarget
	}{
		{name: "no channel", target: HeartbeatTarget{To: "123456789"}},
		{name: "no id", target: HeartbeatTarget{Channel: "telegram"}},
		{name: "unknown channel", target: HeartbeatTarget{Channel: "slack", To: "123456789"}},
		// A web/terminal target has no platform room id, so it needs a session.
		{name: "api without a session", target: HeartbeatTarget{Channel: "api", To: "123456789"}},
		{name: "cli without a session", target: HeartbeatTarget{Channel: "cli", To: "0"}},
		{name: "telegram chat id", target: HeartbeatTarget{Channel: "telegram", To: "not-a-number"}},
		{
			name:   "telegram thread id",
			target: HeartbeatTarget{Channel: "telegram", To: "-1001234567890", ThreadID: "topic-one"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &BotServiceImpl{conversationRepo: newFakeConversationRepo()}
			if _, err := svc.heartbeatMessage("Reminder", tc.target, "cron"); err == nil {
				t.Fatalf("heartbeatMessage(%+v) = nil error", tc.target)
			}
		})
	}
}

func TestOwnerUserID(t *testing.T) {
	withOwnerIDs(t, "", "not-a-number", "123456789")
	if got := ownerUserID(); got != 123456789 {
		t.Fatalf("ownerUserID() = %d, want 123456789", got)
	}

	withOwnerIDs(t)
	if got := ownerUserID(); got != 0 {
		t.Fatalf("ownerUserID() with no owners = %d, want 0", got)
	}
}

// A scheduled turn must join the conversation that already owns its scope, so
// the whole group or topic keeps one transcript.
func TestHeartbeatJoinsExistingScopedConversation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	withOwnerIDs(t, "123456789")

	repo := newFakeConversationRepo()
	repo.conversations["2b3c4d5e-1111-4222-8333-944455556666"] = &model.Conversation{
		Id:     "2b3c4d5e-1111-4222-8333-944455556666",
		UserId: 123456789,
		Scope:  "tg:-1001234567890:77",
		Active: true,
		Messages: []provider.Message{
			{Role: "user", Content: "morning"},
			{Role: "assistant", Content: "good morning"},
		},
	}
	svc := &BotServiceImpl{
		conversationRepo: repo,
		llmProvider:      provider.NewOpenAIProvider("http://127.0.0.1:1", "sk-test", "test-model"),
	}

	msg, err := svc.heartbeatMessage("Time to hydrate", HeartbeatTarget{
		Channel:  "telegram",
		To:       "-1001234567890",
		ThreadID: "77",
	}, "cron")
	if err != nil {
		t.Fatalf("heartbeatMessage: %v", err)
	}

	messages, err := svc.buildConversationMessages(&model.User{UserId: 123456789}, msg)
	if err != nil {
		t.Fatalf("buildConversationMessages: %v", err)
	}
	if msg.ConversationID != "2b3c4d5e-1111-4222-8333-944455556666" {
		t.Fatalf("ConversationID = %q, want the scoped conversation", msg.ConversationID)
	}
	if len(messages) != 4 {
		t.Fatalf("messages = %d, want 4 (system + 2 history + new)", len(messages))
	}
}
