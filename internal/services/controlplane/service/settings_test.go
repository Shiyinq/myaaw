package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func TestAuthServiceSeedsDefaultSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controlplane.json")
	auth, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("NewAuthServiceAt: %v", err)
	}

	got := auth.Settings()
	want := DefaultSettings()
	if got != want {
		t.Fatalf("seeded settings = %+v, want %+v", got, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read controlplane.json: %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("unmarshal controlplane.json: %v", err)
	}
	rawSettings, ok := stored["settings"].(map[string]any)
	if !ok {
		t.Fatalf("stored settings missing or not an object: %#v", stored["settings"])
	}
	if rawSettings["shell_mode"] != "advanced" || rawSettings["locale"] != "en" || rawSettings["theme_mode"] != "system" {
		t.Fatalf("unexpected persisted settings defaults: %#v", rawSettings)
	}
}

func TestAuthServiceMigratesMissingSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controlplane.json")
	if _, err := NewAuthServiceAt(path); err != nil {
		t.Fatalf("NewAuthServiceAt: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read controlplane.json: %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delete(stored, "settings")
	data, _ = json.Marshal(stored)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	auth, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if auth.Settings() != DefaultSettings() {
		t.Fatalf("migrated settings = %+v, want defaults", auth.Settings())
	}

	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated file: %v", err)
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("unmarshal migrated: %v", err)
	}
	if _, ok := stored["settings"].(map[string]any); !ok {
		t.Fatalf("migrated file is missing settings key: %#v", stored)
	}
}

func TestAuthServiceUpdateSettingsPersistsAndNormalizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controlplane.json")
	auth, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("NewAuthServiceAt: %v", err)
	}

	updated, err := auth.UpdateSettings(SettingsPatch{
		Locale:        strPtr("id"),
		ThemeMode:     strPtr("dark"),
		Theme:         strPtr("obsidian"),
		ShellMode:     strPtr("bots"),
		TextScale:     intPtr(150), // clamped to 125
		QueueMessages: boolPtr(true),
		SpeechRate:    floatPtr(2.5), // clamped to 1.5
		NotifyVolume:  floatPtr(-0.5), // clamped to 0
	})
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	if updated.Locale != "id" || updated.ThemeMode != "dark" || updated.Theme != "obsidian" || updated.ShellMode != "bots" {
		t.Fatalf("unexpected updated fields: %+v", updated)
	}
	if updated.TextScale != 125 {
		t.Fatalf("text_scale = %d, want 125", updated.TextScale)
	}
	if !updated.QueueMessages || updated.InterruptOnSend {
		t.Fatalf("expected queue_messages=true and interrupt_on_send=false, got queue=%v interrupt=%v", updated.QueueMessages, updated.InterruptOnSend)
	}
	if updated.SpeechRate != 1.5 {
		t.Fatalf("speech_rate = %v, want 1.5", updated.SpeechRate)
	}
	if updated.NotifyVolume != 0 {
		t.Fatalf("notify_volume = %v, want 0", updated.NotifyVolume)
	}

	// Reload from disk and verify persistence.
	reloaded, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Settings() != updated {
		t.Fatalf("reloaded settings = %+v, want %+v", reloaded.Settings(), updated)
	}

	// Partial update preserves previous custom fields.
	next, err := reloaded.UpdateSettings(SettingsPatch{
		Density: strPtr("compact"),
	})
	if err != nil {
		t.Fatalf("second UpdateSettings: %v", err)
	}
	if next.Density != "compact" || next.ShellMode != "bots" || next.Locale != "id" {
		t.Fatalf("partial patch lost prior fields: %+v", next)
	}
}
