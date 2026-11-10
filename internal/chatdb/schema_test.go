package chatdb

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// stored returns the raw text of a DATETIME column. The driver converts DATETIME
// values to time.Time, so a plain scan would format them instead of showing what
// the migration actually wrote.
func stored(t *testing.T, db *sql.DB, id, column string) string {
	t.Helper()
	var value string
	if err := db.QueryRow(
		`SELECT CAST(`+column+` AS TEXT) FROM conversations WHERE id = ?`, id,
	).Scan(&value); err != nil {
		t.Fatalf("read %s of %s: %v", column, id, err)
	}
	return value
}

// A database written by the older daemon has conversation timestamps in the
// machine's local time. They must be normalized to UTC on open, or the list
// (which orders by the text column) would interleave new UTC rows with old local
// ones.
func TestEnsureSchemaNormalizesConversationTimestampsToUTC(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "myaaw.db"))
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

	insert := func(id, created, updated string) {
		t.Helper()
		if _, err := db.Exec(
			`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, 1, id, "[]", 1, created, updated,
		); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	// Local time, the shape the old code wrote.
	insert("local", "2026-11-06 16:36:49.466539+07:00", "2026-11-06 16:37:07.959102+07:00")
	// Already UTC: it must be left byte-for-byte alone.
	insert("utc", "2026-11-06 09:00:00+00:00", "2026-11-06 09:00:00+00:00")
	// Unparseable: the migration must not blank it.
	insert("broken", "not-a-date", "not-a-date")

	if err := EnsureSchema(db); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	layout := "2006-01-02 15:04:05.999999999-07:00"
	for _, test := range []struct {
		column string
		want   string
	}{
		{"created_at", "2026-11-06T09:36:49.466539Z"},
		{"updated_at", "2026-11-06T09:37:07.959102Z"},
	} {
		raw := stored(t, db, "local", test.column)
		if !strings.HasSuffix(raw, "+00:00") {
			t.Fatalf("%s = %q, want a UTC suffix", test.column, raw)
		}
		got, err := time.Parse(layout, raw)
		if err != nil {
			t.Fatalf("parse %s %q: %v", test.column, raw, err)
		}
		want, _ := time.Parse(time.RFC3339Nano, test.want)
		// SQLite's %f rounds to the millisecond.
		if diff := got.Sub(want); diff < -2*time.Millisecond || diff > 2*time.Millisecond {
			t.Fatalf("%s = %v, want %v", test.column, got, want)
		}
	}

	if raw := stored(t, db, "utc", "updated_at"); raw != "2026-11-06 09:00:00+00:00" {
		t.Fatalf("utc row was rewritten: %q", raw)
	}
	if raw := stored(t, db, "broken", "updated_at"); raw != "not-a-date" {
		t.Fatalf("broken timestamp was replaced: %q", raw)
	}

	// Running it again must not change the normalized row.
	if err := EnsureSchema(db); err != nil {
		t.Fatalf("ensure schema twice: %v", err)
	}
	if raw := stored(t, db, "local", "created_at"); !strings.HasSuffix(raw, "+00:00") {
		t.Fatalf("second pass changed created_at: %q", raw)
	}
}
