package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		scope TEXT NOT NULL DEFAULT '',
		channel TEXT NOT NULL DEFAULT '',
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

func TestConversationListFiltersByChannel(t *testing.T) {
	dbPath := setupConversationAgent(t)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(`UPDATE conversations SET channel = 'telegram' WHERE id = 'conv-1'`); err != nil {
		t.Fatalf("update conv-1: %v", err)
	}
	if _, err := db.Exec(`UPDATE conversations SET channel = 'api' WHERE id = 'conv-2'`); err != nil {
		t.Fatalf("update conv-2: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, channel, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
		"conv-tg", 7, "telegram", "Second telegram", "[]", 0, now, now,
	); err != nil {
		t.Fatalf("insert conv-tg: %v", err)
	}
	db.Close()

	conversations := NewConversationService()
	telegram, err := conversations.List("myfriend", 7, 10, 0, "", ConversationFilter{Channel: "telegram"})
	if err != nil {
		t.Fatalf("List telegram: %v", err)
	}
	if telegram.Total != 2 || len(telegram.Conversations) != 2 {
		t.Fatalf("telegram page = %+v", telegram)
	}
	for _, conversation := range telegram.Conversations {
		if conversation.Channel != "telegram" {
			t.Fatalf("channel leaked into the filtered page: %+v", telegram.Conversations)
		}
	}

	apiPage, err := conversations.List("myfriend", 7, 10, 0, "", ConversationFilter{Channel: "api"})
	if err != nil {
		t.Fatalf("List api: %v", err)
	}
	if apiPage.Total != 1 || len(apiPage.Conversations) != 1 || apiPage.Conversations[0].ID != "conv-2" {
		t.Fatalf("api page = %+v", apiPage)
	}

	all, err := conversations.List("myfriend", 7, 10, 0, "", ConversationFilter{})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if all.Total != 3 {
		t.Fatalf("all total = %d, want 3", all.Total)
	}
}

func TestConversationServiceCarriesScope(t *testing.T) {
	dbPath := setupConversationAgent(t)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, scope, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
		"conv-topic", 7, "tg:-1001111:77", "Topic session", "[]", 1, now, now,
	); err != nil {
		t.Fatalf("insert scoped conversation: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS chat_scopes (
		scope TEXT PRIMARY KEY,
		chat_name TEXT NOT NULL DEFAULT '',
		topic_name TEXT NOT NULL DEFAULT '',
		is_forum INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL
	)`); err != nil {
		t.Fatalf("create chat_scopes: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO chat_scopes (scope, chat_name, topic_name, is_forum, updated_at) VALUES (?,?,?,?,?)`,
		"tg:-1001111:77", "Example Group", "Daily Report", 1, now,
	); err != nil {
		t.Fatalf("insert scope names: %v", err)
	}
	db.Close()

	conversations := NewConversationService()
	page, err := conversations.List("myfriend", 7, 10, 0, "", ConversationFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found *ConversationSummary
	for i := range page.Conversations {
		if page.Conversations[i].ID == "conv-topic" {
			found = &page.Conversations[i]
		}
	}
	if found == nil {
		t.Fatalf("scoped conversation missing from the list: %+v", page.Conversations)
	}
	if found.Scope != "tg:-1001111:77" {
		t.Fatalf("summary scope = %q", found.Scope)
	}
	if found.ChatName != "Example Group" || found.TopicName != "Daily Report" {
		t.Fatalf("summary names = %q / %q", found.ChatName, found.TopicName)
	}
	if !found.IsForum {
		t.Fatal("summary should carry is_forum")
	}

	detail, err := conversations.Get("myfriend", "conv-topic", 7, 10, 0)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail.Scope != "tg:-1001111:77" {
		t.Fatalf("detail scope = %q", detail.Scope)
	}
	if detail.ChatName != "Example Group" || detail.TopicName != "Daily Report" {
		t.Fatalf("detail names = %q / %q", detail.ChatName, detail.TopicName)
	}
	if !detail.IsForum {
		t.Fatal("detail should carry is_forum")
	}
}

func TestConversationServiceList(t *testing.T) {
	setupConversationAgent(t)
	conversations := NewConversationService()

	page, err := conversations.List("myfriend", 7, 10, 0, "", ConversationFilter{})
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

	other, err := conversations.List("myfriend", 8, 10, 0, "", ConversationFilter{})
	if err != nil {
		t.Fatalf("List other user: %v", err)
	}
	if other.Total != 1 || len(other.Conversations) != 1 || other.Conversations[0].ID != "conv-3" {
		t.Fatalf("other = %+v", other)
	}

	empty, err := conversations.List("myfriend", 0, 10, 0, "", ConversationFilter{})
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

	page, err := conversations.List("myfriend", 7, 10, 0, "fir", ConversationFilter{})
	if err != nil {
		t.Fatalf("List search: %v", err)
	}
	if page.Total != 1 || len(page.Conversations) != 1 || page.Conversations[0].ID != "conv-1" {
		t.Fatalf("search page = %+v", page)
	}

	none, err := conversations.List("myfriend", 7, 10, 0, "other", ConversationFilter{})
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

	escaped, err := conversations.List("myfriend", 7, 10, 0, "100%", ConversationFilter{})
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

	first, err := conversations.List("myfriend", 7, 1, 0, "", ConversationFilter{})
	if err != nil {
		t.Fatalf("List first: %v", err)
	}
	if first.Total != 2 || len(first.Conversations) != 1 || !first.HasMore {
		t.Fatalf("first page = %+v", first)
	}
	if first.Conversations[0].ID != "conv-2" {
		t.Fatalf("first page = %+v", first.Conversations)
	}

	second, err := conversations.List("myfriend", 7, 1, 1, "", ConversationFilter{})
	if err != nil {
		t.Fatalf("List second: %v", err)
	}
	if second.Total != 2 || len(second.Conversations) != 1 || second.HasMore {
		t.Fatalf("second page = %+v", second)
	}
	if second.Conversations[0].ID != "conv-1" {
		t.Fatalf("second page = %+v", second.Conversations)
	}

	empty, err := conversations.List("myfriend", 7, 1, 5, "", ConversationFilter{})
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

func TestConversationServiceGetLastPage(t *testing.T) {
	setupConversationAgent(t)
	conversations := NewConversationService()

	tail, err := conversations.Get("myfriend", "conv-1", 7, 2, -1)
	if err != nil {
		t.Fatalf("Get tail: %v", err)
	}
	if tail.TotalMessages != 3 || tail.Offset != 1 || len(tail.Messages) != 2 || tail.HasMore {
		t.Fatalf("tail = %+v", tail)
	}
	if tail.Messages[0].Content != "hello" {
		t.Fatalf("tail messages = %+v", tail.Messages)
	}
	// The turn summary covers the whole conversation, not just the page.
	if len(tail.Turns) != 1 || tail.Turns[0].Index != 1 || tail.Turns[0].Text != "hello" {
		t.Fatalf("tail turns = %+v", tail.Turns)
	}

	older, err := conversations.Get("myfriend", "conv-1", 7, 2, 0)
	if err != nil {
		t.Fatalf("Get older: %v", err)
	}
	if older.Offset != 0 || len(older.Messages) != 2 || !older.HasMore {
		t.Fatalf("older = %+v", older)
	}

	// A conversation shorter than the page starts at zero.
	short, err := conversations.Get("myfriend", "conv-1", 7, 50, -1)
	if err != nil {
		t.Fatalf("Get short: %v", err)
	}
	if short.Offset != 0 || len(short.Messages) != 3 {
		t.Fatalf("short = %+v", short)
	}

	// Asking for everything keeps the old behaviour.
	all, err := conversations.Get("myfriend", "conv-1", 7, 0, -1)
	if err != nil {
		t.Fatalf("Get all: %v", err)
	}
	if all.Offset != 0 || len(all.Messages) != 3 {
		t.Fatalf("all = %+v", all)
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

	page, err := NewConversationService().List("myfriend", 0, 10, 0, "", ConversationFilter{})
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

// readAtOf reads the stored watermark straight from the database.
func readAtOf(t *testing.T, dbPath, id string) time.Time {
	t.Helper()
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var at sql.NullTime
	if err := db.QueryRow(`SELECT read_at FROM conversations WHERE id = ?`, id).Scan(&at); err != nil {
		t.Fatalf("read read_at: %v", err)
	}
	if !at.Valid {
		t.Fatalf("read_at for %s is null", id)
	}
	return at.Time
}

func TestConversationMarkReadIsMonotonic(t *testing.T) {
	dbPath := setupConversationAgent(t)
	conversations := NewConversationService()

	// Rows that predate the column are backfilled as read, so an upgrade never
	// paints the existing list unread.
	page, err := conversations.List("myfriend", 7, 10, 0, "", ConversationFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, item := range page.Conversations {
		if item.ReadAt == nil {
			t.Fatalf("conversation %s has no read_at after the migration", item.ID)
		}
	}
	raw, err := json.Marshal(page.Conversations[0])
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if !strings.Contains(string(raw), `"read_at"`) {
		t.Fatalf("summary JSON has no read_at field: %s", raw)
	}

	// A conversation created after the column exists starts unread.
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	updated := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		"conv-new", 7, "Fresh", "[]", 0, updated, updated,
	); err != nil {
		t.Fatalf("insert conv-new: %v", err)
	}
	db.Close()

	page, err = conversations.List("myfriend", 7, 10, 0, "", ConversationFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var fresh *ConversationSummary
	for index := range page.Conversations {
		if page.Conversations[index].ID == "conv-new" {
			fresh = &page.Conversations[index]
		}
	}
	if fresh == nil {
		t.Fatal("conv-new missing from the list")
	}
	if fresh.ReadAt != nil {
		t.Fatalf("conv-new already read: %v", fresh.ReadAt)
	}

	// A timestamp past the stored turn is clamped to updated_at.
	future := updated.Add(24 * time.Hour)
	if err := conversations.MarkRead("myfriend", "conv-new", 7, &future); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if got := readAtOf(t, dbPath, "conv-new"); !got.Equal(updated) {
		t.Fatalf("read_at = %v, want %v", got, updated)
	}

	// Replaying an older timestamp never moves the marker backwards.
	past := updated.Add(-24 * time.Hour)
	if err := conversations.MarkRead("myfriend", "conv-new", 7, &past); err != nil {
		t.Fatalf("mark read backwards: %v", err)
	}
	if got := readAtOf(t, dbPath, "conv-new"); !got.Equal(updated) {
		t.Fatalf("read_at moved backwards to %v", got)
	}

	// No timestamp means now, and now is still clamped to updated_at.
	if err := conversations.MarkRead("myfriend", "conv-new", 7, nil); err != nil {
		t.Fatalf("mark read now: %v", err)
	}
	if got := readAtOf(t, dbPath, "conv-new"); !got.Equal(updated) {
		t.Fatalf("read_at = %v, want %v", got, updated)
	}

	// Another user cannot acknowledge it.
	if err := conversations.MarkRead("myfriend", "conv-new", 8, nil); !errors.Is(err, ErrConversationForbidden) {
		t.Fatalf("err = %v, want ErrConversationForbidden", err)
	}
}
