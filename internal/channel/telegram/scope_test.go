package telegram

import "testing"

func TestTelegramScope(t *testing.T) {
	cases := []struct {
		name   string
		chat   Chat
		thread int
		want   string
	}{
		{"private", Chat{Id: 42, Type: "private"}, 0, ""},
		{"group without topics", Chat{Id: -1001111, Type: "group"}, 0, "tg:-1001111"},
		{"supergroup without topics", Chat{Id: -1001111, Type: "supergroup"}, 0, "tg:-1001111"},
		{"forum General message", Chat{Id: -1001111, Type: "supergroup"}, 1, "tg:-1001111"},
		{"forum General service message", Chat{Id: -1001111, Type: "supergroup"}, 0, "tg:-1001111"},
		{"forum topic", Chat{Id: -1001111, Type: "supergroup"}, 77, "tg:-1001111:77"},
		{"another group", Chat{Id: -1002222, Type: "supergroup"}, 0, "tg:-1002222"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := telegramScope(c.chat, c.thread); got != c.want {
				t.Fatalf("telegramScope = %q, want %q", got, c.want)
			}
		})
	}
}

func TestParseIncomingSetsThreadAndScope(t *testing.T) {
	adapter := NewTelegramAdapter(nil)

	payload := []byte(`{"update_id":1,"message":{"message_id":5,"message_thread_id":77,"text":"hi","chat":{"id":-1001111,"title":"Example Group","type":"supergroup","is_forum":true},"from":{"id":42,"first_name":"Alice","username":"alice"}}}`)
	msg, err := adapter.ParseIncoming(payload)
	if err != nil {
		t.Fatalf("ParseIncoming: %v", err)
	}

	meta, ok := msg.RawMeta.(TelegramMeta)
	if !ok {
		t.Fatalf("RawMeta type = %T", msg.RawMeta)
	}
	if meta.ThreadID != 77 {
		t.Fatalf("ThreadID = %d, want 77", meta.ThreadID)
	}
	if meta.ChatID != -1001111 || meta.MessageID != 5 {
		t.Fatalf("meta = %+v", meta)
	}
	if msg.Scope != "tg:-1001111:77" {
		t.Fatalf("scope = %q", msg.Scope)
	}
	if msg.SenderID != "42" || msg.SenderName != "Alice" || msg.SenderUsername != "alice" {
		t.Fatalf("sender = %q / %q / %q", msg.SenderID, msg.SenderName, msg.SenderUsername)
	}
	if msg.ChatName != "Example Group" {
		t.Fatalf("chat name = %q", msg.ChatName)
	}
	if !msg.IsForum {
		t.Fatal("forum flag was not carried")
	}
	if msg.ChatType != "supergroup" || msg.ChatID != "-1001111" {
		t.Fatalf("chat = %q / %q", msg.ChatType, msg.ChatID)
	}
	if msg.TopicID != "77" || !msg.IsThread {
		t.Fatalf("topic = %q, thread = %v", msg.TopicID, msg.IsThread)
	}

	// A direct chat keeps the legacy empty scope.
	dm, err := adapter.ParseIncoming([]byte(`{"message":{"message_id":1,"text":"hi","chat":{"id":42,"type":"private"},"from":{"id":42}}}`))
	if err != nil {
		t.Fatalf("ParseIncoming DM: %v", err)
	}
	if dm.Scope != "" {
		t.Fatalf("DM scope = %q, want empty", dm.Scope)
	}
	if dm.ChatType != "private" || dm.ChatID != "42" || dm.TopicID != "" || dm.IsThread {
		t.Fatalf("DM chat = %q / %q / %q / %v", dm.ChatType, dm.ChatID, dm.TopicID, dm.IsThread)
	}
}

func TestParseIncomingGeneralTopicUsesChatScope(t *testing.T) {
	adapter := NewTelegramAdapter(nil)

	msg, err := adapter.ParseIncoming([]byte(`{"message":{"message_id":1,"message_thread_id":1,"text":"hi","chat":{"id":-1001111,"type":"supergroup","is_forum":true},"from":{"id":42}}}`))
	if err != nil {
		t.Fatalf("ParseIncoming: %v", err)
	}
	if msg.Scope != "tg:-1001111" {
		t.Fatalf("scope = %q, want chat-level scope", msg.Scope)
	}
	if msg.TopicID != "" || msg.IsThread {
		t.Fatalf("General topic = %q / %v, want no topic id and no thread", msg.TopicID, msg.IsThread)
	}
}
