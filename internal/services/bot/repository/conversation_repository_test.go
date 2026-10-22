package repository

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newTestConversationRepo(t *testing.T) *ConversationRepositoryImpl {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE conversations (
		id TEXT PRIMARY KEY,
		user_id INTEGER,
		title TEXT,
		messages TEXT,
		active BOOLEAN,
		created_at DATETIME,
		updated_at DATETIME
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return &ConversationRepositoryImpl{db: db}
}

func TestCreateConversationWithId(t *testing.T) {
	repo := newTestConversationRepo(t)

	conversation, err := repo.CreateConversationWithId(-1, "ctx-abc", "")
	if err != nil {
		t.Fatalf("CreateConversationWithId: %v", err)
	}
	if conversation.Id != "ctx-abc" || conversation.UserId != -1 || conversation.Title != "New Chat" {
		t.Fatalf("conversation = %+v", conversation)
	}

	stored, err := repo.GetConversationById("ctx-abc")
	if err != nil {
		t.Fatalf("GetConversationById: %v", err)
	}
	if stored == nil || stored.UserId != -1 {
		t.Fatalf("stored = %+v", stored)
	}
}
