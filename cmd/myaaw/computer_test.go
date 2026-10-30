package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/config"
)

func TestSetComputerUseInConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	original := `{
    "default_provider": "deepseek",
    "bot": {
        "owner_ids": ["1"],
        "type": "private"
    },
    "approvals": {
        "mode": "manual"
    }
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := setComputerUseInConfig(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true on enable")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.ComputerUse == nil || !cfg.Bot.ComputerUse.Enabled {
		t.Fatalf("computer_use = %+v, want enabled", cfg.Bot.ComputerUse)
	}
	if cfg.DefaultProvider != "deepseek" {
		t.Fatalf("default_provider = %q", cfg.DefaultProvider)
	}
	if len(cfg.Bot.OwnerIDs) != 1 || cfg.Bot.OwnerIDs[0] != "1" {
		t.Fatalf("owner_ids = %v", cfg.Bot.OwnerIDs)
	}
	if cfg.Approvals == nil || cfg.Approvals.Mode != "manual" {
		t.Fatalf("approvals = %+v", cfg.Approvals)
	}

	changed, err = setComputerUseInConfig(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false when already enabled")
	}

	changed, err = setComputerUseInConfig(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true on disable")
	}
	enabled, err := readComputerUseEnabled(path)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("computer_use still enabled after disable")
	}
}

func TestToolsJSONGate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".myaaw", "agent-x", "tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	toolsPath := filepath.Join(dir, "tools.json")

	// Missing file: implicit enabled.
	if ok, _ := toolsJSONGate("agent-x"); !ok {
		t.Fatal("missing tools.json should be enabled")
	}

	// Missing key: implicit enabled.
	if err := os.WriteFile(toolsPath, []byte(`{"builtin_tools":{"bash":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := toolsJSONGate("agent-x"); !ok {
		t.Fatal("absent computer key should be enabled")
	}

	// Explicit false: disabled.
	if err := os.WriteFile(toolsPath, []byte(`{"builtin_tools":{"computer":false}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, state := toolsJSONGate("agent-x")
	if ok {
		t.Fatalf("explicit false = %v (%q)", ok, state)
	}
}

func TestReadComputerUseEnabledMissingFile(t *testing.T) {
	if _, err := readComputerUseEnabled(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected an error for a missing config")
	}
}
