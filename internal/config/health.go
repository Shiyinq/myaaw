package config

import (
	"database/sql"
	"fmt"
	"os"

	"myaaw/internal/agentdir"

	_ "github.com/mattn/go-sqlite3"
)

// PingSQLite checks if the SQLite database is reachable and valid.
func PingSQLite() error {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		if path, err := agentdir.ActiveDatabasePath(); err == nil {
			dbPath = path
		} else {
			dbPath = "myaaw.db"
		}
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	return nil
}
