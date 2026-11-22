package telegram

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"myaaw/internal/chatdb"
	"myaaw/internal/provider"
)

func openTelegramTestDB(t *testing.T) *sql.DB {
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
	if err := chatdb.EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return db
}

func TestTelegramExtraMessagesSendAndCallbackRoundTrip(t *testing.T) {
	db := openTelegramTestDB(t)

	var (
		mu      sync.Mutex
		methods []string
		bodies  []map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		method := parts[len(parts)-1]

		if method != "sendChatAction" {
			mu.Lock()
			methods = append(methods, method)
			bodies = append(bodies, body)
			mu.Unlock()
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":77}}`))
	}))
	defer srv.Close()

	prevBase := telegramAPIBaseURL
	telegramAPIBaseURL = srv.URL
	defer func() { telegramAPIBaseURL = prevBase }()

	extras := []provider.ExtraMessage{
		{
			ID: "em_test",
			Blocks: []provider.ExtraBlock{
				{
					ID:          "blk_media",
					Type:        "media",
					Interactive: false,
					Data: json.RawMessage(`{
						"items": [
							{"url": "https://example.com/1.png", "kind": "image"},
							{"url": "https://example.com/2.png", "kind": "image"}
						],
						"caption": "Gallery"
					}`),
				},
				{
					ID:          "blk_btn",
					Type:        "buttons",
					Interactive: true,
					Data: json.RawMessage(`{
						"prompt": "Deploy?",
						"buttons": [
							{"id": "yes", "label": "Confirm", "type": "action", "value": "deploy_yes"},
							{"id": "docs", "label": "Docs", "type": "link", "url": "https://example.com/docs"}
						]
					}`),
					State: &provider.InteractionState{Status: "pending"},
				},
			},
		},
	}

	// Persist conversation so callback can lock block state in conversations.db.
	msgs := []provider.Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", Content: "ready", ExtraMessages: extras},
	}
	rawMsgs, _ := json.Marshal(msgs)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?)`,
		"conv-tg-1", 101, "tg", string(rawMsgs), now, now,
	); err != nil {
		t.Fatalf("insert conv: %v", err)
	}

	if err := sendTelegramExtraMessages(db, 101, 0, 5, 101, "conv-tg-1", extras); err != nil {
		t.Fatalf("sendTelegramExtraMessages: %v", err)
	}

	mu.Lock()
	gotMethods := append([]string(nil), methods...)
	gotBodies := append([]map[string]any(nil), bodies...)
	mu.Unlock()

	if len(gotMethods) != 2 || gotMethods[0] != "sendMediaGroup" || gotMethods[1] != "sendMessage" {
		t.Fatalf("unexpected methods: %v", gotMethods)
	}

	// Extract callback_data from the sent inline keyboard.
	markup, _ := gotBodies[1]["reply_markup"].(map[string]any)
	kb, _ := markup["inline_keyboard"].([]any)
	firstRow, _ := kb[0].([]any)
	firstBtn, _ := firstRow[0].(map[string]any)
	cbData, _ := firstBtn["callback_data"].(string)
	if !strings.HasPrefix(cbData, extraCallbackPrefix) {
		t.Fatalf("expected em: callback_data, got %q", cbData)
	}

	// Simulate user clicking the button in Telegram.
	cbPayload := []byte(`{
		"update_id": 999,
		"callback_query": {
			"id": "cbq_1",
			"data": "` + cbData + `",
			"from": {"id": 101, "first_name": "Alice", "username": "alice"},
			"message": {
				"message_id": 77,
				"chat": {"id": 101, "type": "private"}
			}
		}
	}`)

	synth, handled := handleExtraMessageCallbackWithDB(db, cbPayload)
	if !handled || len(synth) == 0 {
		t.Fatalf("expected handled synthetic payload, handled=%v len=%d", handled, len(synth))
	}
	if !strings.Contains(string(synth), "<interaction kind=\\\"button\\\">") && !strings.Contains(string(synth), `\u003cinteraction kind=\"button\"\u003e`) {
		t.Fatalf("synthetic message missing <interaction> block: %s", string(synth))
	}

	// Second click on the same button must be rejected (single-use lock).
	synth2, handled2 := handleExtraMessageCallbackWithDB(db, cbPayload)
	if !handled2 || len(synth2) != 0 {
		t.Fatalf("expected duplicate click rejected, handled=%v len=%d", handled2, len(synth2))
	}
}
