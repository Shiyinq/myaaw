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
	other := []provider.Message{
		{Role: "user", Content: "other", Version: 2},
	}
	olderJSON, _ := json.Marshal(older)
	newerJSON, _ := json.Marshal(newer)
	otherJSON, _ := json.Marshal(other)

	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		"conv-1", 7, "First", string(olderJSON), 1, now.Add(-2*time.Hour), now.Add(-time.Hour),
	); err != nil {
		t.Fatalf("insert conv-1: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		"conv-2", 7, "Second", string(newerJSON), 1, now, now,
	); err != nil {
		t.Fatalf("insert conv-2: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		"conv-3", 8, "Other user", string(otherJSON), 1, now, now,
	); err != nil {
		t.Fatalf("insert conv-3: %v", err)
	}

	return dbPath
}

func TestConversationServiceList(t *testing.T) {
	setupConversationAgent(t)
	conversations := NewConversationService()

	page, err := conversations.List("myfriend", 7, 10, 0, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Total != 2 || len(page.Conversations) != 2 || page.HasMore {
		t.Fatalf("page = %+v", page)
	}
	if page.Conversations[0].ID != "conv-2" {
		t.Fatalf("newest conversation should be first: %+v", page.Conversations)
	}
	if page.Conversations[1].MessageCount != 3 {
		t.Fatalf("message count = %d, want 3", page.Conversations[1].MessageCount)
	}

	other, err := conversations.List("myfriend", 8, 10, 0, "")
	if err != nil {
		t.Fatalf("List other user: %v", err)
	}
	if other.Total != 1 || len(other.Conversations) != 1 || other.Conversations[0].ID != "conv-3" {
		t.Fatalf("other = %+v", other)
	}

	empty, err := conversations.List("myfriend", 0, 10, 0, "")
	if err != nil {
		t.Fatalf("List user 0: %v", err)
	}
	if empty.Total != 0 || len(empty.Conversations) != 0 {
		t.Fatalf("user 0 should not see other users' conversations: %+v", empty)
	}
}

func TestConversationServiceListSearch(t *testing.T) {
	dbPath := setupConversationAgent(t)
	conversations := NewConversationService()

	page, err := conversations.List("myfriend", 7, 10, 0, "fir")
	if err != nil {
		t.Fatalf("List search: %v", err)
	}
	if page.Total != 1 || len(page.Conversations) != 1 || page.Conversations[0].ID != "conv-1" {
		t.Fatalf("search page = %+v", page)
	}

	none, err := conversations.List("myfriend", 7, 10, 0, "other")
	if err != nil {
		t.Fatalf("List search miss: %v", err)
	}
	if none.Total != 0 || len(none.Conversations) != 0 {
		t.Fatalf("search should not match other users: %+v", none)
	}

	// LIKE wildcards in user input must be treated as literals.
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		"conv-4", 7, "100% done", `[]`, 1, now, now,
	); err != nil {
		db.Close()
		t.Fatalf("insert conv-4: %v", err)
	}
	db.Close()

	escaped, err := conversations.List("myfriend", 7, 10, 0, "100%")
	if err != nil {
		t.Fatalf("List escaped search: %v", err)
	}
	if escaped.Total != 1 || len(escaped.Conversations) != 1 || escaped.Conversations[0].ID != "conv-4" {
		t.Fatalf("escaped search = %+v", escaped)
	}
}

func TestConversationServiceListPagination(t *testing.T) {
	setupConversationAgent(t)
	conversations := NewConversationService()

	first, err := conversations.List("myfriend", 7, 1, 0, "")
	if err != nil {
		t.Fatalf("List first: %v", err)
	}
	if first.Total != 2 || len(first.Conversations) != 1 || !first.HasMore {
		t.Fatalf("first page = %+v", first)
	}
	if first.Conversations[0].ID != "conv-2" {
		t.Fatalf("first page = %+v", first.Conversations)
	}

	second, err := conversations.List("myfriend", 7, 1, 1, "")
	if err != nil {
		t.Fatalf("List second: %v", err)
	}
	if second.Total != 2 || len(second.Conversations) != 1 || second.HasMore {
		t.Fatalf("second page = %+v", second)
	}
	if second.Conversations[0].ID != "conv-1" {
		t.Fatalf("second page = %+v", second.Conversations)
	}

	empty, err := conversations.List("myfriend", 7, 1, 5, "")
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(empty.Conversations) != 0 || empty.HasMore {
		t.Fatalf("empty page = %+v", empty)
	}
}

func TestConversationServiceGet(t *testing.T) {
	setupConversationAgent(t)
	conversations := NewConversationService()

	detail, err := conversations.Get("myfriend", "conv-1", 7, 0, 0)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail.Title != "First" || detail.TotalMessages != 3 || len(detail.Messages) != 3 {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.Messages[1].Role != "user" || detail.Messages[1].Content != "hello" {
		t.Fatalf("messages = %+v", detail.Messages)
	}
	if detail.HasMore {
		t.Fatalf("detail = %+v", detail)
	}

	page, err := conversations.Get("myfriend", "conv-1", 7, 1, 1)
	if err != nil {
		t.Fatalf("Get page: %v", err)
	}
	if page.TotalMessages != 3 || len(page.Messages) != 1 || !page.HasMore {
		t.Fatalf("page = %+v", page)
	}
	if page.Messages[0].Content != "hello" {
		t.Fatalf("page messages = %+v", page.Messages)
	}

	if _, err := conversations.Get("myfriend", "conv-1", 8, 0, 0); err != ErrConversationForbidden {
		t.Fatalf("err = %v, want ErrConversationForbidden", err)
	}
	if _, err := conversations.Get("myfriend", "missing", 7, 0, 0); err != ErrConversationNotFound {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
}

func TestConversationServiceRename(t *testing.T) {
	setupConversationAgent(t)
	conversations := NewConversationService()

	title, err := conversations.Rename("myfriend", "conv-1", 7, "  Renamed  ")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if title != "Renamed" {
		t.Fatalf("title = %q", title)
	}

	detail, err := conversations.Get("myfriend", "conv-1", 7, 0, 0)
	if err != nil {
		t.Fatalf("Get after rename: %v", err)
	}
	if detail.Title != "Renamed" {
		t.Fatalf("title = %q", detail.Title)
	}

	if _, err := conversations.Rename("myfriend", "conv-1", 8, "Stolen"); err != ErrConversationForbidden {
		t.Fatalf("err = %v, want ErrConversationForbidden", err)
	}
	if _, err := conversations.Rename("myfriend", "missing", 7, "Nope"); err != ErrConversationNotFound {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
	if _, err := conversations.Rename("myfriend", "conv-1", 7, "   "); err != ErrInvalidTitle {
		t.Fatalf("err = %v, want ErrInvalidTitle", err)
	}
}

func TestConversationServiceDelete(t *testing.T) {
	setupConversationAgent(t)
	conversations := NewConversationService()

	if err := conversations.Delete("myfriend", "conv-1", 8); err != ErrConversationForbidden {
		t.Fatalf("err = %v, want ErrConversationForbidden", err)
	}
	if _, err := conversations.Get("myfriend", "conv-1", 7, 0, 0); err != nil {
		t.Fatalf("conversation should still exist: %v", err)
	}

	if err := conversations.Delete("myfriend", "conv-2", 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := conversations.Get("myfriend", "conv-2", 7, 0, 0); err != ErrConversationNotFound {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
	if err := conversations.Delete("myfriend", "missing", 7); err != ErrConversationNotFound {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
}

func TestConversationServiceWithoutDatabase(t *testing.T) {
	setupConfigAgent(t, nil)

	page, err := NewConversationService().List("myfriend", 0, 10, 0, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Total != 0 || len(page.Conversations) != 0 {
		t.Fatalf("page = %+v", page)
	}
	if _, err := NewConversationService().Get("myfriend", "missing", 0, 0, 0); err != ErrConversationNotFound {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
	if _, err := NewConversationService().Rename("myfriend", "missing", 0, "x"); err != ErrConversationNotFound {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
	if err := NewConversationService().Delete("myfriend", "missing", 0); err != ErrConversationNotFound {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
}

func TestEscapeLike(t *testing.T) {
	got := escapeLike(`50%_off\now`)
	want := `50\%\_off\\now`
	if got != want {
		t.Fatalf("escapeLike = %q, want %q", got, want)
	}
}
