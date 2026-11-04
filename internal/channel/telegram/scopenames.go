package telegram

import (
	"encoding/json"
	"strings"
)

// ScopeNames is the display metadata learned for a chat target: the group name
// and, for forum supergroups, the topic name.
type ScopeNames struct {
	Scope     string
	ChatName  string
	TopicName string
	IsForum   bool
}

var scopeNameSink func(ScopeNames)

// SetScopeNameSink registers the store that persists scope display names. The
// sink is optional: without it these names are simply not recorded.
func SetScopeNameSink(fn func(ScopeNames)) {
	scopeNameSink = fn
}

// RecordScopeNames extracts display names from an update and hands them to the
// sink. It must run before the content filter, because chat renames and topic
// creation are service messages with no content of their own.
//
// Names come from three places:
//   - message.chat.title (present on every group message),
//   - top-level forum_topic_created / forum_topic_edited service messages,
//   - reply_to_message.forum_topic_created / forum_topic_edited, which is how a
//     reply to the topic's first message reveals the topic name.
func RecordScopeNames(payload json.RawMessage) {
	if scopeNameSink == nil {
		return
	}

	type topicNamePayload struct {
		Name string `json:"name"`
	}
	var update struct {
		Message *struct {
			Chat struct {
				ID      int    `json:"id"`
				Type    string `json:"type"`
				Title   string `json:"title"`
				IsForum bool   `json:"is_forum"`
			} `json:"chat"`
			MessageThreadID   int               `json:"message_thread_id"`
			NewChatTitle      string            `json:"new_chat_title"`
			ForumTopicCreated *topicNamePayload `json:"forum_topic_created"`
			ForumTopicEdited  *topicNamePayload `json:"forum_topic_edited"`
			ReplyToMessage    *struct {
				ForumTopicCreated *topicNamePayload `json:"forum_topic_created"`
				ForumTopicEdited  *topicNamePayload `json:"forum_topic_edited"`
			} `json:"reply_to_message"`
		} `json:"message"`
	}
	if err := json.Unmarshal(payload, &update); err != nil || update.Message == nil {
		return
	}

	scope := telegramScope(
		Chat{Id: update.Message.Chat.ID, Type: update.Message.Chat.Type},
		update.Message.MessageThreadID,
	)
	if scope == "" {
		return
	}

	chatName := strings.TrimSpace(update.Message.Chat.Title)
	if renamed := strings.TrimSpace(update.Message.NewChatTitle); renamed != "" {
		chatName = renamed
	}

	topicName := ""
	switch {
	case update.Message.ForumTopicCreated != nil:
		topicName = strings.TrimSpace(update.Message.ForumTopicCreated.Name)
	case update.Message.ForumTopicEdited != nil:
		topicName = strings.TrimSpace(update.Message.ForumTopicEdited.Name)
	case update.Message.ReplyToMessage != nil && update.Message.ReplyToMessage.ForumTopicCreated != nil:
		topicName = strings.TrimSpace(update.Message.ReplyToMessage.ForumTopicCreated.Name)
	case update.Message.ReplyToMessage != nil && update.Message.ReplyToMessage.ForumTopicEdited != nil:
		topicName = strings.TrimSpace(update.Message.ReplyToMessage.ForumTopicEdited.Name)
	}

	if chatName == "" && topicName == "" {
		return
	}
	// Plain messages are covered by the conversation service, which stores
	// chat.title when it resolves the session. Only write here for events that
	// the service never sees: renames and topic creation.
	if topicName == "" && strings.TrimSpace(update.Message.NewChatTitle) == "" {
		return
	}
	scopeNameSink(ScopeNames{
		Scope:     scope,
		ChatName:  chatName,
		TopicName: topicName,
		IsForum:   update.Message.Chat.IsForum,
	})
}
