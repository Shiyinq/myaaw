package config

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// PingSQLite checks if the SQLite database is reachable and valid.
func PingSQLite() error {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			dbPath = filepath.Join(homeDir, ".myaaw", "myaaw.db")
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
