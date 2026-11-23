package chatdb

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func openTestChatDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`CREATE TABLE conversations (
		id TEXT PRIMARY KEY,
		user_id INTEGER,
		title TEXT,
		messages TEXT,
		active BOOLEAN,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	if err != nil {
		t.Fatalf("create conversations: %v", err)
	}
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return db
}

func TestSaveAndGetInteractionCallback(t *testing.T) {
	db := openTestChatDB(t)

	key, err := SaveInteractionCallback(db, InteractionCallback{
		ConversationID: "conv-1",
		UserID:         42,
		Channel:        "telegram",
		BlockID:        "blk_abc",
		Kind:           "button",
		ActionID:       "btn_yes",
		Label:          "Approve",
		Value:          "approve",
	})
	if err != nil {
		t.Fatalf("SaveInteractionCallback: %v", err)
	}
	if len(key) != 8 {
		t.Fatalf("expected 8-char key, got %q", key)
	}

	got, ok, err := GetInteractionCallback(db, key)
	if err != nil || !ok {
		t.Fatalf("GetInteractionCallback: ok=%v err=%v", ok, err)
	}
	if got.ConversationID != "conv-1" || got.BlockID != "blk_abc" || got.ActionID != "btn_yes" || got.UserID != 42 {
		t.Fatalf("unexpected callback record: %+v", got)
	}
}

func TestMarkBlockSubmittedAtomicLock(t *testing.T) {
	db := openTestChatDB(t)

	msgs := []map[string]any{
		{"role": "user", "content": "pick one"},
		{
			"role":    "assistant",
			"content": "choose:",
			"extra_messages": []storedExtraMessage{
				{
					ID: "em_1",
					Blocks: []storedExtraBlock{
						{
							ID:          "blk_btn",
							Type:        "buttons",
							Interactive: true,
							Data:        json.RawMessage(`{"buttons":[{"id":"b1","label":"Go","type":"action","value":"go"}]}`),
							State:       &BlockInteractionState{Status: "pending"},
						},
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(msgs)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?)`,
		"conv-1", 42, "test", string(raw), now, now,
	); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	convID, already, state, err := MarkBlockSubmitted(db, "conv-1", "blk_btn", "telegram", "Go", map[string]any{"action_id": "b1"})
	if err != nil {
		t.Fatalf("first MarkBlockSubmitted: %v", err)
	}
	if already || convID != "conv-1" {
		t.Fatalf("first submit unexpected: convID=%q already=%v", convID, already)
	}
	if state.Status != "submitted" || state.Actor != "telegram" || state.Summary != "Go" {
		t.Fatalf("unexpected state: %+v", state)
	}

	// Second submit on the same single-use block must be rejected.
	convID2, already2, state2, err := MarkBlockSubmitted(db, "conv-1", "blk_btn", "web", "Go again", map[string]any{"action_id": "b1"})
	if err != nil {
		t.Fatalf("second MarkBlockSubmitted: %v", err)
	}
	if !already2 || convID2 != "conv-1" {
		t.Fatalf("expected alreadySubmitted=true on duplicate submit, got convID=%q already=%v", convID2, already2)
	}
	if state2.Actor != "telegram" {
		t.Fatalf("expected original actor preserved, got %+v", state2)
	}
}
