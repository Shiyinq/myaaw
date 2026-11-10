package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPingSQLiteAtCreatesAndPings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "myaaw.db")
	if err := PingSQLiteAt(path); err != nil {
		t.Fatalf("PingSQLiteAt: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database file was not created: %v", err)
	}
}

func TestPingSQLiteAtReportsMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "myaaw.db")
	if err := PingSQLiteAt(path); err == nil {
		t.Fatal("expected an error for a database in a missing directory")
	}
}

func TestLoadConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{
		"default_provider": "personal",
		"providers": {"personal": {"type": "openai"}},
		"heartbeat": {"active": true, "every": "20m"},
		"channels": {"telegram": {"active": true, "token": "t"}}
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if cfg.DefaultProvider != "personal" {
		t.Fatalf("default provider = %q", cfg.DefaultProvider)
	}
	if cfg.Providers["personal"].Type != "openai" {
		t.Fatalf("provider type = %q", cfg.Providers["personal"].Type)
	}
	if !cfg.Heartbeat.Active || cfg.Heartbeat.Every != "20m" {
		t.Fatalf("heartbeat = %+v", cfg.Heartbeat)
	}
	if cfg.Channels.Telegram == nil || !cfg.Channels.Telegram.Active {
		t.Fatalf("telegram channel = %+v", cfg.Channels.Telegram)
	}
}

func TestLoadConfigFileRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadConfigFile(path); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}
