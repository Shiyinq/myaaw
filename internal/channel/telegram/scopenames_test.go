package telegram

import "testing"

func captureScopeNames(t *testing.T) *[]ScopeNames {
	t.Helper()
	var captured []ScopeNames
	SetScopeNameSink(func(names ScopeNames) {
		captured = append(captured, names)
	})
	t.Cleanup(func() { SetScopeNameSink(nil) })
	return &captured
}

func TestRecordScopeNamesFromTopicReply(t *testing.T) {
	captured := captureScopeNames(t)

	payload := []byte(`{"update_id":549738754,"message":{
		"message_id":44,
		"from":{"id":123456789,"first_name":"Alice","username":"Alice"},
		"chat":{"id":-1234567890,"title":"Example Group","is_forum":true,"type":"supergroup"},
		"message_thread_id":2,
		"reply_to_message":{"message_id":2,"message_thread_id":2,"forum_topic_created":{"name":"Daily Report"}},
		"text":"tadi aku ngomong apa"
	}}`)

	RecordScopeNames(payload)

	if len(*captured) != 1 {
		t.Fatalf("captured = %+v", *captured)
	}
	got := (*captured)[0]
	want := ScopeNames{Scope: "tg:-1234567890:2", ChatName: "Example Group", TopicName: "Daily Report", IsForum: true}
	if got != want {
		t.Fatalf("names = %+v, want %+v", got, want)
	}
}

func TestRecordScopeNamesFromTopicCreated(t *testing.T) {
	captured := captureScopeNames(t)

	RecordScopeNames([]byte(`{"message":{
		"message_id":3,
		"chat":{"id":-1001111,"title":"Example Group","is_forum":true,"type":"supergroup"},
		"message_thread_id":5,
		"forum_topic_created":{"name":"Alerts"}
	}}`))

	if len(*captured) != 1 {
		t.Fatalf("captured = %+v", *captured)
	}
	got := (*captured)[0]
	if got.Scope != "tg:-1001111:5" || got.TopicName != "Alerts" || got.ChatName != "Example Group" || !got.IsForum {
		t.Fatalf("names = %+v", got)
	}
}

func TestRecordScopeNamesFromChatRename(t *testing.T) {
	captured := captureScopeNames(t)

	RecordScopeNames([]byte(`{"message":{
		"message_id":9,
		"chat":{"id":-1001111,"title":"Example Group","type":"supergroup"},
		"new_chat_title":"Example Group"
	}}`))

	if len(*captured) != 1 {
		t.Fatalf("captured = %+v", *captured)
	}
	got := (*captured)[0]
	if got.Scope != "tg:-1001111" || got.ChatName != "Example Group" || got.TopicName != "" || got.IsForum {
		t.Fatalf("names = %+v", got)
	}
}

func TestRecordScopeNamesIgnoresDirectMessages(t *testing.T) {
	captured := captureScopeNames(t)

	RecordScopeNames([]byte(`{"message":{
		"message_id":1,
		"chat":{"id":42,"type":"private"},
		"text":"hi"
	}}`))

	if len(*captured) != 0 {
		t.Fatalf("direct messages must not record scope names: %+v", *captured)
	}
}

func TestRecordScopeNamesWithoutSinkIsSafe(t *testing.T) {
	SetScopeNameSink(nil)
	RecordScopeNames([]byte(`{"message":{"chat":{"id":-1,"title":"x","type":"group"},"text":"hi"}}`))
}
