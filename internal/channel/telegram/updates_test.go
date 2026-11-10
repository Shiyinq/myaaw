package telegram

import "testing"

func TestHasChatContent(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"text", `{"message":{"text":"hi"}}`, true},
		{"photo", `{"message":{"photo":[{"file_id":"x"}]}}`, true},
		{"document", `{"message":{"document":{"file_id":"x"}}}`, true},
		{"voice", `{"message":{"voice":{"file_id":"x"}}}`, true},
		{"topic created", `{"message":{"message_id":3,"message_thread_id":77,"forum_topic_created":{"name":"x"}}}`, false},
		{"member joined", `{"message":{"new_chat_members":[{"id":1}]}}`, false},
		{"chat title changed", `{"message":{"new_chat_title":"Myaaw Group"}}`, false},
		{"my chat member", `{"my_chat_member":{"chat":{"id":1}}}`, false},
		{"edited message", `{"edited_message":{"text":"hi"}}`, false},
		{"callback query", `{"callback_query":{"id":"cb","data":"x"}}`, false},
		{"invalid json", `not-json`, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasChatContent([]byte(c.body)); got != c.want {
				t.Fatalf("HasChatContent = %v, want %v", got, c.want)
			}
		})
	}
}
