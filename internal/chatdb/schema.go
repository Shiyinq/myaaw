package chatdb

import (
	"database/sql"
	"strings"
)

// EnsureSchema adds the chat-organization columns and the folder table to an
// agent database. The agent creates the base `conversations` table in
// config.initSchema; the control plane runs the same helper because it also
// opens the file directly, possibly before an upgraded agent has started.
//
// Everything here is additive, so an older build keeps working: it simply
// ignores the new columns and table.
func EnsureSchema(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS chat_folders (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	)`); err != nil {
		return err
	}
	if _, err := db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_folders_user_name ON chat_folders(user_id, name COLLATE NOCASE)`,
	); err != nil {
		return err
	}

	// chat_scopes stores display names for chat targets (a Telegram group, a
	// forum topic, and later other channels). Conversations reference a scope by
	// value, so names survive /new rotations. is_forum marks a chat whose
	// chat-level scope is the default topic (Telegram "General").
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS chat_scopes (
		scope TEXT PRIMARY KEY,
		chat_name TEXT NOT NULL DEFAULT '',
		topic_name TEXT NOT NULL DEFAULT '',
		is_forum INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL
	)`); err != nil {
		return err
	}

	scopeColumns, err := tableColumns(db, "chat_scopes")
	if err != nil {
		return err
	}
	if !scopeColumns["is_forum"] {
		if _, err := db.Exec(`ALTER TABLE chat_scopes ADD COLUMN is_forum INTEGER NOT NULL DEFAULT 0`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}

	columns, err := tableColumns(db, "conversations")
	if err != nil {
		return err
	}
	// A database opened before the agent ever created a conversation has no
	// table yet; the agent's initSchema creates it, and this helper adds the
	// columns on a later run.
	if len(columns) == 0 {
		return nil
	}

	for _, column := range []struct {
		name string
		ddl  string
	}{
		{"pinned_at", `ALTER TABLE conversations ADD COLUMN pinned_at DATETIME`},
		{"folder_id", `ALTER TABLE conversations ADD COLUMN folder_id TEXT`},
		{"scope", `ALTER TABLE conversations ADD COLUMN scope TEXT NOT NULL DEFAULT ''`},
		// Channel records which platform owns a conversation (telegram, discord,
		// api, cli). Rows created before the column existed stay empty.
		{"channel", `ALTER TABLE conversations ADD COLUMN channel TEXT NOT NULL DEFAULT ''`},
	} {
		if columns[column.name] {
			continue
		}
		if _, err := db.Exec(column.ddl); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}

	for _, index := range []string{
		`CREATE INDEX IF NOT EXISTS idx_conversations_folder ON conversations(user_id, folder_id)`,
		`CREATE INDEX IF NOT EXISTS idx_conversations_pinned ON conversations(user_id, pinned_at)`,
		`CREATE INDEX IF NOT EXISTS idx_conversations_scope ON conversations(scope, active)`,
		// At most one active conversation per scope, so two topics can never
		// share a session pointer. Legacy unscoped rows are excluded.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_conversations_active_scope ON conversations(scope) WHERE active = 1 AND scope <> ''`,
	} {
		if _, err := db.Exec(index); err != nil {
			return err
		}
	}
	return nil
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var (
			cid        int
			name       string
			ctype      sql.NullString
			notNull    int
			defaultV   sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultV, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}
