package telegram

import "fmt"

// telegramScope maps a chat and forum thread to a conversation scope.
//
// Threads 0 and 1 are collapsed into the chat-level scope on purpose:
//   - 0 means the update is chat-level (service messages, non-forum groups);
//   - 1 is the forum's "General" topic, whose messages may or may not carry
//     message_thread_id.
//
// Mapping both to the same scope keeps one session for General and lets a
// group's history continue if Topics are enabled later.
//
// The thread id itself is still forwarded to the Bot API so replies land in the
// right topic; only the session key is normalized.
func telegramScope(chat Chat, threadID int) string {
	if chat.Type == "private" {
		return ""
	}
	if threadID <= 1 {
		return fmt.Sprintf("tg:%d", chat.Id)
	}
	return fmt.Sprintf("tg:%d:%d", chat.Id, threadID)
}
