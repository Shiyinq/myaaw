package service

import (
	"testing"

	"myaaw/internal/channel"
)

// One conversation (or one chat target before the id is known) must map to one
// lock, so a live chat and a scheduled turn cannot run at the same time.
func TestConversationLockKey(t *testing.T) {
	cases := []struct {
		name string
		msg  channel.IncomingMessage
		want string
	}{
		{
			name: "pinned session keys by conversation",
			msg: channel.IncomingMessage{
				ConversationID: " 0f4e8b3a-1111-4222-8333-944455556666 ",
				Channel:        "telegram",
				ChatID:         "-1001234567890",
				TopicID:        "77",
			},
			want: "conversation:0f4e8b3a-1111-4222-8333-944455556666",
		},
		{
			name: "unpinned turn keys by room",
			msg: channel.IncomingMessage{
				Channel: "Telegram",
				ChatID:  "-1001234567890",
				TopicID: "77",
			},
			want: "target:telegram:-1001234567890:77",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := conversationLockKey(&tc.msg); got != tc.want {
				t.Fatalf("conversationLockKey(%+v) = %q, want %q", tc.msg, got, tc.want)
			}
		})
	}

	// Two topics in one group, and the group itself, must not share a lock.
	group := conversationLockKey(&channel.IncomingMessage{Channel: "telegram", ChatID: "-1001234567890"})
	topic := conversationLockKey(&channel.IncomingMessage{Channel: "telegram", ChatID: "-1001234567890", TopicID: "77"})
	other := conversationLockKey(&channel.IncomingMessage{Channel: "telegram", ChatID: "-1001234567890", TopicID: "78"})
	if group == topic || topic == other {
		t.Fatalf("room locks collide: %q %q %q", group, topic, other)
	}

	// The same conversation from two entry points shares one key.
	chat := conversationLockKey(&channel.IncomingMessage{ConversationID: "conv-1"})
	heartbeat := conversationLockKey(&channel.IncomingMessage{ConversationID: "conv-1", Channel: "api"})
	if chat != heartbeat {
		t.Fatalf("same conversation got different keys: %q %q", chat, heartbeat)
	}
}
