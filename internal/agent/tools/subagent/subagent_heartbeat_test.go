package subagent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A finished task reports back through /heartbeat, and that endpoint refuses an
// api/cli target without a session id (it answers 500). The tool must therefore
// forward the whole chat target, not just the user id.
func TestSendHeartbeatCarriesTheChatTarget(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/heartbeat" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tool := NewSubAgentTool()
	tool.sendHeartbeat(SubAgentTask{Name: "Reminder"}, heartbeatTarget{
		channel:   "api",
		to:        "123456789",
		sessionID: "0f4e8b3a-1111-4222-8333-944455556666",
	}, server.URL, "batch_1", 1, 1, "done")

	for key, want := range map[string]string{
		"channel":    "api",
		"to":         "123456789",
		"session_id": "0f4e8b3a-1111-4222-8333-944455556666",
		"trigger":    "subagent",
	} {
		if value, _ := got[key].(string); value != want {
			t.Errorf("heartbeat %s = %q, want %q", key, value, want)
		}
	}
}

// A Telegram topic and a Discord DM keep the fields their channel needs, the same
// way a cron delivery does.
func TestSendHeartbeatKeepsTopicAndChatType(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tool := NewSubAgentTool()
	tool.sendHeartbeat(SubAgentTask{Name: "Standup"}, heartbeatTarget{
		channel:  "telegram",
		to:       "-1001234567890",
		threadID: "77",
	}, server.URL, "batch_1", 2, 3, "done")

	if got["thread_id"] != "77" {
		t.Errorf("thread_id = %v, want 77", got["thread_id"])
	}
	if got["to"] != "-1001234567890" {
		t.Errorf("to = %v, want the group id", got["to"])
	}

	tool.sendHeartbeat(SubAgentTask{Name: "DM"}, heartbeatTarget{
		channel:  "discord",
		to:       "123456789",
		chatType: "dm",
	}, server.URL, "batch_1", 1, 1, "done")

	if got["chat_type"] != "dm" {
		t.Errorf("chat_type = %v, want dm", got["chat_type"])
	}
}
