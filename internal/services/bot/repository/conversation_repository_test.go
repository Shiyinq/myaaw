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
		scope TEXT NOT NULL DEFAULT '',
		title TEXT,
		messages TEXT,
		active BOOLEAN,
		created_at DATETIME,
		updated_at DATETIME
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX idx_conversations_active_scope
		ON conversations(scope) WHERE active = 1 AND scope <> ''`); err != nil {
		t.Fatalf("create scope index: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE chat_scopes (
		scope TEXT PRIMARY KEY,
		chat_name TEXT NOT NULL DEFAULT '',
		topic_name TEXT NOT NULL DEFAULT '',
		is_forum INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL
	)`); err != nil {
		t.Fatalf("create chat_scopes: %v", err)
	}
	return &ConversationRepositoryImpl{db: db}
}

func TestUpsertScopeNames(t *testing.T) {
	repo := newTestConversationRepo(t)

	if err := repo.UpsertScopeNames("tg:-100:77", "Example Group", "Daily Report", true); err != nil {
		t.Fatalf("UpsertScopeNames: %v", err)
	}

	var (
		chat    string
		topic   string
		isForum bool
	)
	if err := repo.db.QueryRow(`SELECT chat_name, topic_name, is_forum FROM chat_scopes WHERE scope = ?`, "tg:-100:77").Scan(&chat, &topic, &isForum); err != nil {
		t.Fatalf("read names: %v", err)
	}
	if chat != "Example Group" || topic != "Daily Report" || !isForum {
		t.Fatalf("names = %q / %q / %v", chat, topic, isForum)
	}

	// An update that only knows the chat name keeps the topic name.
	if err := repo.UpsertScopeNames("tg:-100:77", "Renamed Group", "", true); err != nil {
		t.Fatalf("UpsertScopeNames: %v", err)
	}
	if err := repo.db.QueryRow(`SELECT chat_name, topic_name, is_forum FROM chat_scopes WHERE scope = ?`, "tg:-100:77").Scan(&chat, &topic, &isForum); err != nil {
		t.Fatalf("read names: %v", err)
	}
	if chat != "Renamed Group" || topic != "Daily Report" || !isForum {
		t.Fatalf("names = %q / %q / %v", chat, topic, isForum)
	}

	// Direct chats have no scope and must not create a row.
	if err := repo.UpsertScopeNames("", "ignored", "ignored", true); err != nil {
		t.Fatalf("UpsertScopeNames empty scope: %v", err)
	}
	var count int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM chat_scopes`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("scope rows = %d, want 1", count)
	}
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

func TestCreateConversationInScopeRotatesOnlyItsScope(t *testing.T) {
	repo := newTestConversationRepo(t)

	firstPhoto, err := repo.CreateConversationInScope(1, "tg:-100:77", "")
	if err != nil {
		t.Fatalf("CreateConversationInScope: %v", err)
	}
	notif, err := repo.CreateConversationInScope(1, "tg:-100:78", "")
	if err != nil {
		t.Fatalf("CreateConversationInScope: %v", err)
	}
	secondPhoto, err := repo.CreateConversationInScope(2, "tg:-100:77", "")
	if err != nil {
		t.Fatalf("CreateConversationInScope: %v", err)
	}

	activePhoto, err := repo.GetActiveConversationByScope(1, "tg:-100:77")
	if err != nil {
		t.Fatalf("GetActiveConversationByScope: %v", err)
	}
	if activePhoto == nil || activePhoto.Id != secondPhoto.Id {
		t.Fatalf("active photo conversation = %+v, want %q", activePhoto, secondPhoto.Id)
	}

	activeNotif, err := repo.GetActiveConversationByScope(1, "tg:-100:78")
	if err != nil {
		t.Fatalf("GetActiveConversationByScope: %v", err)
	}
	if activeNotif == nil || activeNotif.Id != notif.Id {
		t.Fatalf("active notification conversation = %+v, want %q", activeNotif, notif.Id)
	}

	old, err := repo.GetConversationById(firstPhoto.Id)
	if err != nil {
		t.Fatalf("GetConversationById: %v", err)
	}
	if old == nil || old.Active {
		t.Fatalf("previous generation should stay as inactive history: %+v", old)
	}
}

func TestGetActiveConversationByScopeIsSharedAcrossUsers(t *testing.T) {
	repo := newTestConversationRepo(t)

	created, err := repo.CreateConversationInScope(111, "tg:-100:77", "")
	if err != nil {
		t.Fatalf("CreateConversationInScope: %v", err)
	}

	// A different member of the same topic resolves the same session.
	active, err := repo.GetActiveConversationByScope(222, "tg:-100:77")
	if err != nil {
		t.Fatalf("GetActiveConversationByScope: %v", err)
	}
	if active == nil || active.Id != created.Id {
		t.Fatalf("active = %+v, want %q", active, created.Id)
	}
}

func TestScopedSessionsDoNotTouchLegacyDMs(t *testing.T) {
	repo := newTestConversationRepo(t)

	dm, err := repo.CreateConversation(1, "")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	topic, err := repo.CreateConversationInScope(1, "tg:-100:77", "")
	if err != nil {
		t.Fatalf("CreateConversationInScope: %v", err)
	}

	// A DM "new chat" must not deactivate the topic session.
	if _, err := repo.CreateConversation(1, ""); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	active, err := repo.GetActiveConversationByScope(1, "tg:-100:77")
	if err != nil {
		t.Fatalf("GetActiveConversationByScope: %v", err)
	}
	if active == nil || active.Id != topic.Id {
		t.Fatalf("topic session was touched by a DM rotation: %+v", active)
	}

	legacy, err := repo.GetActiveConversationByUserId(1)
	if err != nil {
		t.Fatalf("GetActiveConversationByUserId: %v", err)
	}
	if legacy == nil || legacy.Id == dm.Id {
		t.Fatalf("legacy DM should have rotated: %+v", legacy)
	}
}
