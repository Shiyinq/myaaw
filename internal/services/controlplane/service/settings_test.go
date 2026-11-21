package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func TestAuthServiceSeedsDefaultSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controlplane.db")
	auth, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("NewAuthServiceAt: %v", err)
	}
	defer auth.Close()

	got := auth.Settings()
	want := DefaultSettings()
	if got != want {
		t.Fatalf("seeded settings = %+v, want %+v", got, want)
	}

	var rawJSON string
	if err := auth.db.DB().QueryRow(`SELECT data_json FROM settings WHERE id = 1`).Scan(&rawJSON); err != nil {
		t.Fatalf("query settings: %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &stored); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if stored["shell_mode"] != "studio" || stored["locale"] != "en" || stored["theme_mode"] != "system" {
		t.Fatalf("unexpected persisted settings defaults: %#v", stored)
	}
}

func TestAuthServiceReseedsAccountWhilePreservingSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controlplane.db")
	first, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("NewAuthServiceAt: %v", err)
	}
	if err := first.ChangePassword(DefaultPassword, "custom-secret-99"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := first.UpdateSettings(SettingsPatch{
		Locale:    strPtr("id"),
		ThemeMode: strPtr("dark"),
		ShellMode: strPtr("bots"),
	}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	// Simulate restoring a --no-secrets snapshot where auth_account was stripped.
	if _, err := first.db.DB().Exec(`DELETE FROM auth_account`); err != nil {
		t.Fatalf("delete auth_account: %v", err)
	}
	_ = first.Close()

	reloaded, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("reload after auth strip: %v", err)
	}
	defer reloaded.Close()

	if reloaded.Username() != DefaultUsername || reloaded.Role() != DefaultRole || !reloaded.MustChangePassword() {
		t.Fatalf("expected re-seeded default admin requiring password change, got user=%q role=%q mustChange=%v",
			reloaded.Username(), reloaded.Role(), reloaded.MustChangePassword())
	}
	if _, _, err := reloaded.Login(DefaultUsername, DefaultPassword); err != nil {
		t.Fatalf("expected default login to succeed after re-seed: %v", err)
	}
	gotSettings := reloaded.Settings()
	if gotSettings.Locale != "id" || gotSettings.ThemeMode != "dark" || gotSettings.ShellMode != "bots" {
		t.Fatalf("restored settings were lost during auth re-seed: %+v", gotSettings)
	}
}

func TestAuthServiceUpdateSettingsPersistsAndNormalizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controlplane.db")
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
		SpeechRate:    floatPtr(2.5),  // clamped to 1.5
		NotifyVolume:  floatPtr(-0.5), // clamped to 0
	})
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	_ = auth.Close()

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
	defer reloaded.Close()
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
