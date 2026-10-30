package service

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupActivityAgent seeds conversations on known local days so the histogram
// can be asserted exactly.
func setupActivityAgent(t *testing.T) (time.Time, time.Time) {
	t.Helper()

	agentRoot := setupConfigAgent(t, nil)
	dbDir := filepath.Join(agentRoot, "database")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("mkdir database: %v", err)
	}

	db, err := sql.Open("sqlite3", filepath.Join(dbDir, "myaaw.db"))
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

	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	second := from.AddDate(0, 0, 1)
	to := from.AddDate(0, 0, 2)

	seed := []struct {
		id      string
		userID  int
		created time.Time
	}{
		{"a-1", 7, from.Add(9 * time.Hour)},
		{"a-2", 7, from.Add(20 * time.Hour)},
		{"a-3", 7, second.Add(11 * time.Hour)},
		{"a-4", 8, second.Add(12 * time.Hour)}, // another user
		{"a-5", 7, from.AddDate(0, 0, -3)},     // before the window
	}
	for _, row := range seed {
		if _, err := db.Exec(
			`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
			row.id, row.userID, row.id, "[]", 1, row.created, row.created,
		); err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}
	return from, to
}

func TestConversationServiceActivity(t *testing.T) {
	from, to := setupActivityAgent(t)
	conversations := NewConversationService()

	activity, err := conversations.Activity("myfriend", 7, from, to)
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if len(activity.Days) != 3 {
		t.Fatalf("days = %+v, want 3 padded days", activity.Days)
	}
	if activity.Days[0].Date != from.Format("2006-01-02") || activity.Days[0].Count != 2 {
		t.Fatalf("day 0 = %+v", activity.Days[0])
	}
	if activity.Days[1].Count != 1 {
		t.Fatalf("day 1 = %+v (another user must not be counted)", activity.Days[1])
	}
	if activity.Days[2].Count != 0 {
		t.Fatalf("day 2 = %+v, want a zero-filled day", activity.Days[2])
	}
	if activity.Total != 3 {
		t.Fatalf("total = %d, want 3", activity.Total)
	}

	// A window that misses every row still returns one zero day per date.
	tight, err := conversations.Activity("myfriend", 7, to, to)
	if err != nil {
		t.Fatalf("Activity tight: %v", err)
	}
	if len(tight.Days) != 1 || tight.Days[0].Count != 0 || tight.Total != 0 {
		t.Fatalf("tight window = %+v", tight)
	}
}
