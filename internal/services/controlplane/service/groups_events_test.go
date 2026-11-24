package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"myaaw/internal/channel/api"
	"myaaw/internal/provider"
)

func TestGroupEventBridgePersistsAndBroadcastsCronAndHeartbeatTurns(t *testing.T) {
	sseReady := make(chan struct{}, 1)
	sseEvents := make(chan AgentTurnEvent, 8)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = fmt.Fprint(w, ": connected\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case sseReady <- struct{}{}:
		default:
		}
		for {
			select {
			case <-r.Context().Done():
				return
			case ev := <-sseEvents:
				raw, _ := json.Marshal(ev)
				_, _ = fmt.Fprintf(w, "event: conversation.updated\ndata: %s\n\n", raw)
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	}))
	defer srv.Close()

	dir := newMockAgentDirectory(
		AgentInfo{Name: "alice", AgentColor: "bleu", Running: true, BaseURL: srv.URL},
		AgentInfo{Name: "bob", AgentColor: "ambre", Running: false},
	)
	svc := NewGroupServiceWithStore(newTestGroupStore(t), dir, srv.Client())

	created, err := svc.Create(CreateGroupRequest{
		Name:    "Ops Room",
		Members: []string{"alice", "bob"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	roomCh, _, unsubRoom, err := svc.Subscribe(created.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsubRoom()

	globalCh, unsubGlobal := svc.SubscribeGlobal()
	defer unsubGlobal()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.startEventBridgeWithInterval(ctx, 50*time.Millisecond)

	select {
	case <-sseReady:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for event bridge to connect to /api/events")
	}

	// Emit a cron turn targeting this group, plus a duplicate message_id and a silent HEARTBEAT_OK.
	cronEvent := AgentTurnEvent{
		Agent:          "alice",
		ConversationID: "conv-alice-group",
		Scope:          "group:" + created.ID,
		Trigger:        "cron",
		At:             time.Now().UTC().Format(time.RFC3339Nano),
		MessageID:      "msg-cron-1",
		Text:           "Daily check: all systems green! [DONE]",
		Timeline: []api.TimelineItem{
			{Kind: "thought", Text: "Checking cluster health"},
			{Kind: "tool", ID: "step-1", Name: "bash", Arguments: `{"command":"uptime"}`, Result: "up 10 days"},
			{Kind: "text", Text: "Daily check: all systems green!"},
		},
		ExtraMessages: []provider.ExtraMessage{
			{
				ID: "extra-1",
				Blocks: []provider.ExtraBlock{
					{ID: "blk-1", Type: "callout", Data: json.RawMessage(`{"variant":"info","text":"Scheduled report"}`)},
				},
			},
		},
	}
	sseEvents <- cronEvent
	// Duplicate delivery must be ignored idempotently.
	sseEvents <- cronEvent
	// Silent HEARTBEAT_OK must not appear in group chat.
	sseEvents <- AgentTurnEvent{
		Agent:          "alice",
		ConversationID: "conv-alice-group",
		Scope:          "group:" + created.ID,
		Trigger:        "heartbeat",
		At:             time.Now().UTC().Format(time.RFC3339Nano),
		MessageID:      "msg-hb-ok",
		Text:           "HEARTBEAT_OK",
	}
	// Non-group DM cron must not touch the group.
	sseEvents <- AgentTurnEvent{
		Agent:          "alice",
		ConversationID: "conv-alice-dm",
		Scope:          "",
		Trigger:        "cron",
		At:             time.Now().UTC().Format(time.RFC3339Nano),
		MessageID:      "msg-dm-1",
		Text:           "Private DM reminder",
	}

	// Wait for the live room SSE message broadcast.
	select {
	case ev := <-roomCh:
		if ev.Name != "message" {
			t.Fatalf("room event name = %q, want message", ev.Name)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for room SSE message event")
	}

	// Global subscriber (used by /events for sidebar preview & unread badge) must also receive it.
	select {
	case ev := <-globalCh:
		if ev.Name != "message" {
			t.Fatalf("global event name = %q, want message", ev.Name)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for global SSE message event")
	}

	// Give duplicate/ignored events a brief moment to pass through the SSE loop.
	time.Sleep(150 * time.Millisecond)

	detail, err := svc.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(detail.Messages) != 1 {
		t.Fatalf("messages count = %d, want 1 (deduped & filtered); got %+v", len(detail.Messages), detail.Messages)
	}
	msg := detail.Messages[0]
	if msg.ID != "msg-cron-1" || msg.SenderType != "agent" || msg.SenderName != "alice" || msg.AgentColor != "bleu" {
		t.Fatalf("saved message metadata = %+v", msg)
	}
	if msg.Content != "Daily check: all systems green!" {
		t.Fatalf("saved message content = %q", msg.Content)
	}
	if len(msg.Timeline) < 2 || len(msg.ExtraMessages) != 1 {
		t.Fatalf("timeline/extra_messages missing: timeline=%+v extra=%+v", msg.Timeline, msg.ExtraMessages)
	}
	if detail.UnreadCount != 1 || detail.FirstUnreadMessageID != "msg-cron-1" {
		t.Fatalf("unread = (%d, %q), want (1, msg-cron-1)", detail.UnreadCount, detail.FirstUnreadMessageID)
	}

	// Verify alice's member cursor/session updated while bob's cursor stayed behind (so bob sees it next turn).
	for _, m := range detail.Group.Members {
		switch m.Name {
		case "alice":
			if m.LastSeenMessageID != "msg-cron-1" || m.ConversationID != "conv-alice-group" {
				t.Fatalf("alice member = %+v", m)
			}
		case "bob":
			if m.LastSeenMessageID != "" {
				t.Fatalf("bob LastSeenMessageID = %q, want empty so bob sees cron message next turn", m.LastSeenMessageID)
			}
		}
	}
}
