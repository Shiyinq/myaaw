package service

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"myaaw/internal/provider"
)

func setupConversationAgent(t *testing.T) string {
	t.Helper()

	agentRoot := setupConfigAgent(t, nil)
	dbDir := filepath.Join(agentRoot, "database")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("mkdir database: %v", err)
	}
	dbPath := filepath.Join(dbDir, "myaaw.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE conversations (
		id TEXT PRIMARY KEY,
		user_id INTEGER,
		title TEXT,
		messages TEXT,
		active INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	older := []provider.Message{
		{Role: "system", Content: "system", Version: 2},
		{Role: "user", Content: "hello", Version: 2},
		{Role: "assistant", Content: "hi", Version: 2},
	}
	newer := []provider.Message{
		{Role: "user", Content: "second", Version: 2},
	}
	olderJSON, _ := json.Marshal(older)
	newerJSON, _ := json.Marshal(newer)

	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		"conv-1", 7, "First", string(olderJSON), 1, now.Add(-2*time.Hour), now.Add(-time.Hour),
	); err != nil {
		t.Fatalf("insert conv-1: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		"conv-2", 8, "Second", string(newerJSON), 1, now, now,
	); err != nil {
		t.Fatalf("insert conv-2: %v", err)
	}

	return dbPath
}

func TestConversationServiceList(t *testing.T) {
	setupConversationAgent(t)
	conversations := NewConversationService()

	items, err := conversations.List("myfriend", 0, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].ID != "conv-2" {
		t.Fatalf("newest conversation should be first: %+v", items)
	}
	if items[1].MessageCount != 3 {
		t.Fatalf("message count = %d, want 3", items[1].MessageCount)
	}

	filtered, err := conversations.List("myfriend", 7, 10)
	if err != nil {
		t.Fatalf("List filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "conv-1" {
		t.Fatalf("filtered = %+v", filtered)
	}
}

func TestConversationServiceGet(t *testing.T) {
	setupConversationAgent(t)
	conversations := NewConversationService()

	conv, err := conversations.Get("myfriend", "conv-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if conv.Title != "First" || len(conv.Messages) != 3 {
		t.Fatalf("conversation = %+v", conv)
	}
	if conv.Messages[1].Role != "user" || conv.Messages[1].Content != "hello" {
		t.Fatalf("messages = %+v", conv.Messages)
	}

	if _, err := conversations.Get("myfriend", "missing"); err != ErrConversationNotFound {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
}

func TestConversationServiceWithoutDatabase(t *testing.T) {
	setupConfigAgent(t, nil)

	items, err := NewConversationService().List("myfriend", 0, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %+v", items)
	}
	if _, err := NewConversationService().Get("myfriend", "missing"); err != ErrConversationNotFound {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
}
