package service

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"myaaw/internal/agentdir"

	_ "github.com/mattn/go-sqlite3"
)

const controlPlaneSchemaVersion = 6

var controlPlaneSchemaV1 = []string{
	`CREATE TABLE IF NOT EXISTS auth_account (
		id               INTEGER PRIMARY KEY CHECK (id = 1),
		username         TEXT    NOT NULL,
		password_hash    TEXT    NOT NULL,
		role             TEXT    NOT NULL DEFAULT 'admin',
		user_id          INTEGER NOT NULL DEFAULT 0,
		password_changed INTEGER NOT NULL DEFAULT 0,
		created_at       TEXT    NOT NULL,
		updated_at       TEXT    NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS settings (
		id         INTEGER PRIMARY KEY CHECK (id = 1),
		data_json  TEXT    NOT NULL,
		updated_at TEXT    NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS groups (
		id                   TEXT    PRIMARY KEY,
		name                 TEXT    NOT NULL,
		description          TEXT    NOT NULL DEFAULT '',
		discussion_mode      TEXT    NOT NULL DEFAULT 'autonomous',
		max_turns            INTEGER NOT NULL DEFAULT 8,
		hide_thinking        INTEGER NOT NULL DEFAULT 0,
		hide_tools           INTEGER NOT NULL DEFAULT 0,
		last_read_message_id TEXT    NOT NULL DEFAULT '',
		read_at              TEXT    NOT NULL DEFAULT '',
		created_at           TEXT    NOT NULL,
		updated_at           TEXT    NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_groups_updated_at
		ON groups(updated_at DESC, id ASC)`,
	`CREATE TABLE IF NOT EXISTS group_members (
		group_id             TEXT    NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
		name                 TEXT    NOT NULL,
		position             INTEGER NOT NULL,
		last_seen_message_id TEXT    NOT NULL DEFAULT '',
		conversation_id      TEXT    NOT NULL DEFAULT '',
		PRIMARY KEY (group_id, name)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_group_members_order
		ON group_members(group_id, position ASC)`,
	`CREATE TABLE IF NOT EXISTS group_messages (
		seq                INTEGER PRIMARY KEY AUTOINCREMENT,
		id                 TEXT    NOT NULL UNIQUE,
		group_id           TEXT    NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
		sender_type        TEXT    NOT NULL,
		sender_name        TEXT    NOT NULL,
		agent_color        TEXT    NOT NULL DEFAULT '',
		content            TEXT    NOT NULL DEFAULT '',
		timeline_json      TEXT    NOT NULL DEFAULT '',
		reply_to_id        TEXT    NOT NULL DEFAULT '',
		reply_preview_json TEXT    NOT NULL DEFAULT '',
		quotes_json        TEXT    NOT NULL DEFAULT '',
		attachments_json   TEXT    NOT NULL DEFAULT '',
		extra_messages_json TEXT   NOT NULL DEFAULT '',
		created_at         TEXT    NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_group_messages_group_seq
		ON group_messages(group_id, seq ASC)`,
	`CREATE TABLE IF NOT EXISTS runtime_defaults (
		preset          TEXT PRIMARY KEY,
		command         TEXT NOT NULL DEFAULT '',
		args_json       TEXT NOT NULL DEFAULT '[]',
		model           TEXT NOT NULL DEFAULT '',
		thinking_level  TEXT NOT NULL DEFAULT '',
		session_mode    TEXT NOT NULL DEFAULT '',
		auth_source     TEXT NOT NULL DEFAULT '',
		api_key_env     TEXT NOT NULL DEFAULT '',
		api_key         TEXT NOT NULL DEFAULT '',
		permission_mode TEXT NOT NULL DEFAULT '',
		mcp_bridge      INTEGER NOT NULL DEFAULT 1,
		updated_at      TEXT NOT NULL
	)`,
}

// ControlPlaneDB owns the shared SQLite connection for ~/.myaaw/controlplane/controlplane.db.
type ControlPlaneDB struct {
	db   *sql.DB
	path string
}

// OpenControlPlaneDB opens ~/.myaaw/controlplane/controlplane.db and initializes its schema.
func OpenControlPlaneDB() (*ControlPlaneDB, error) {
	path, err := agentdir.ControlPlaneDBPath()
	if err != nil {
		return nil, err
	}
	return OpenControlPlaneDBAt(path)
}

// OpenControlPlaneDBAt opens (or creates) a control-plane SQLite database at path.
func OpenControlPlaneDBAt(path string) (*ControlPlaneDB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create controlplane db dir: %w", err)
		}
	}

	dsn := "file:" + path + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on&_synchronous=NORMAL"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open controlplane db: %w", err)
	}
	// Single connection serializes concurrent AuthService and GroupService writes cleanly.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping controlplane db: %w", err)
	}

	for _, pragma := range []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA synchronous = NORMAL`,
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure controlplane db (%s): %w", pragma, err)
		}
	}

	cpdb := &ControlPlaneDB{db: db, path: path}
	if err := cpdb.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}

	// Protect password_hash stored in auth_account.
	_ = os.Chmod(path, 0o600)
	return cpdb, nil
}

// DB returns the underlying *sql.DB handle.
func (c *ControlPlaneDB) DB() *sql.DB { return c.db }

// Path returns the filesystem path of the SQLite database.
func (c *ControlPlaneDB) Path() string { return c.path }

// Close closes the underlying SQLite database handle.
func (c *ControlPlaneDB) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

func (c *ControlPlaneDB) migrate() error {
	var version int
	if err := c.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read controlplane schema version: %w", err)
	}

	if version >= controlPlaneSchemaVersion {
		return nil
	}

	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("begin controlplane schema migration: %w", err)
	}
	if version < 1 {
		for _, stmt := range controlPlaneSchemaV1 {
			if _, err := tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply controlplane schema: %w", err)
			}
		}
	} else {
		if version < 2 {
			if _, err := tx.Exec(`ALTER TABLE group_messages ADD COLUMN attachments_json TEXT NOT NULL DEFAULT ''`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply controlplane schema v2: %w", err)
			}
		}
		if version < 3 {
			for _, stmt := range []string{
				`ALTER TABLE groups ADD COLUMN last_read_message_id TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE groups ADD COLUMN read_at TEXT NOT NULL DEFAULT ''`,
				`UPDATE groups
				 SET last_read_message_id = COALESCE((
				     SELECT id FROM group_messages
				     WHERE group_id = groups.id
				     ORDER BY seq DESC LIMIT 1
				 ), ''),
				 read_at = updated_at
				 WHERE last_read_message_id = '' AND read_at = ''`,
			} {
				if _, err := tx.Exec(stmt); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("apply controlplane schema v3: %w", err)
				}
			}
		}
		if version < 4 {
			for _, stmt := range []string{
				`ALTER TABLE groups ADD COLUMN hide_thinking INTEGER NOT NULL DEFAULT 0`,
				`ALTER TABLE groups ADD COLUMN hide_tools INTEGER NOT NULL DEFAULT 0`,
			} {
				if _, err := tx.Exec(stmt); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("apply controlplane schema v4: %w", err)
				}
			}
		}
		if version < 5 {
			if _, err := tx.Exec(`ALTER TABLE group_messages ADD COLUMN extra_messages_json TEXT NOT NULL DEFAULT ''`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply controlplane schema v5: %w", err)
			}
		}
		if version < 6 {
			if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS runtime_defaults (
				preset          TEXT PRIMARY KEY,
				command         TEXT NOT NULL DEFAULT '',
				args_json       TEXT NOT NULL DEFAULT '[]',
				model           TEXT NOT NULL DEFAULT '',
				thinking_level  TEXT NOT NULL DEFAULT '',
				session_mode    TEXT NOT NULL DEFAULT '',
				auth_source     TEXT NOT NULL DEFAULT '',
				api_key_env     TEXT NOT NULL DEFAULT '',
				api_key         TEXT NOT NULL DEFAULT '',
				permission_mode TEXT NOT NULL DEFAULT '',
				mcp_bridge      INTEGER NOT NULL DEFAULT 1,
				updated_at      TEXT NOT NULL
			)`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply controlplane schema v6: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit controlplane schema: %w", err)
	}

	if _, err := c.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, controlPlaneSchemaVersion)); err != nil {
		return fmt.Errorf("set controlplane schema version: %w", err)
	}
	return nil
}
