package service

import (
	"encoding/json"
	"path/filepath"
	"reflect"
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
	if !reflect.DeepEqual(got, want) {
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
	if stored["shell_mode"] != "bots" || stored["locale"] != "en" || stored["theme_mode"] != "system" || stored["show_connection_badge"] != false {
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
		ShellMode: strPtr("studio"),
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
	if gotSettings.Locale != "id" || gotSettings.ThemeMode != "dark" || gotSettings.ShellMode != "studio" {
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
		Locale:              strPtr("id"),
		ThemeMode:           strPtr("dark"),
		Theme:               strPtr("obsidian"),
		ShellMode:           strPtr("studio"),
		TextScale:           intPtr(150), // clamped to 125
		QueueMessages:       boolPtr(true),
		ShowConnectionBadge: boolPtr(true),
		SpeechRate:          floatPtr(2.5),  // clamped to 1.5
		NotifyVolume:        floatPtr(-0.5), // clamped to 0
	})
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	_ = auth.Close()

	if updated.Locale != "id" || updated.ThemeMode != "dark" || updated.Theme != "obsidian" || updated.ShellMode != "studio" {
		t.Fatalf("unexpected updated fields: %+v", updated)
	}
	if !updated.ShowConnectionBadge {
		t.Fatal("connection badge was not enabled")
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
	if !reflect.DeepEqual(reloaded.Settings(), updated) {
		t.Fatalf("reloaded settings = %+v, want %+v", reloaded.Settings(), updated)
	}

	// Partial update preserves previous custom fields.
	next, err := reloaded.UpdateSettings(SettingsPatch{
		Density: strPtr("compact"),
	})
	if err != nil {
		t.Fatalf("second UpdateSettings: %v", err)
	}
	if next.Density != "compact" || next.ShellMode != "studio" || next.Locale != "id" || !next.ShowConnectionBadge {
		t.Fatalf("partial patch lost prior fields: %+v", next)
	}
	next, err = reloaded.UpdateSettings(SettingsPatch{ShowConnectionBadge: boolPtr(false)})
	if err != nil || next.ShowConnectionBadge {
		t.Fatalf("disable connection badge: enabled=%v, err=%v", next.ShowConnectionBadge, err)
	}
}

func TestBotsSidebarConfigNormalizationAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controlplane.db")
	auth, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("NewAuthServiceAt: %v", err)
	}
	defer auth.Close()

	// Legacy JSON row without bots_sidebar loads with default section_order.
	if _, err := auth.db.DB().Exec(`UPDATE settings SET data_json = '{"locale":"id"}' WHERE id = 1`); err != nil {
		t.Fatalf("update legacy row: %v", err)
	}
	legacy, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("reload legacy: %v", err)
	}
	if !reflect.DeepEqual(legacy.Settings().BotsSidebar, DefaultBotsSidebarConfig()) {
		t.Fatalf("legacy bots_sidebar = %+v, want default", legacy.Settings().BotsSidebar)
	}
	_ = legacy.Close()

	// Normalization rejects reserved IDs, deduplicates items across sections, and repairs section_order.
	cfg := BotsSidebarConfig{
		SectionOrder: []string{"unknown", "work", "work", "agents"},
		Sections: []BotsSidebarSection{
			{ID: "groups", Name: "Reserved", Items: []BotsSidebarItemRef{{Kind: "group", ID: "g1"}}},
			{ID: "unread", Name: "Reserved2"},
			{ID: "", Name: "EmptyID"},
			{ID: "work", Name: "  Work  ", Items: []BotsSidebarItemRef{
				{Kind: "AGENT", ID: "myaagy"},
				{Kind: "agent", ID: "myaagy"}, // dup in same section
				{Kind: "group", ID: "g1"},
				{Kind: "bogus", ID: "x"},
			}},
			{ID: "personal", Name: "Personal", Items: []BotsSidebarItemRef{
				{Kind: "agent", ID: "myaagy"}, // dup across sections -> dropped
				{Kind: "agent", ID: "oline"},
			}},
		},
	}

	updated, err := auth.UpdateSettings(SettingsPatch{BotsSidebar: &cfg})
	if err != nil {
		t.Fatalf("UpdateSettings(BotsSidebar): %v", err)
	}

	got := updated.BotsSidebar
	wantOrder := []string{"work", "agents", "groups", "personal"}
	if !reflect.DeepEqual(got.SectionOrder, wantOrder) {
		t.Fatalf("section_order = %v, want %v", got.SectionOrder, wantOrder)
	}
	if len(got.Sections) != 2 {
		t.Fatalf("sections len = %d, want 2: %+v", len(got.Sections), got.Sections)
	}
	if got.Sections[0].Name != "Work" || len(got.Sections[0].Items) != 2 {
		t.Fatalf("section[0] = %+v", got.Sections[0])
	}
	if got.Sections[1].ID != "personal" || len(got.Sections[1].Items) != 1 || got.Sections[1].Items[0].ID != "oline" {
		t.Fatalf("section[1] = %+v", got.Sections[1])
	}

	// Unrelated PATCH preserves bots_sidebar intact.
	afterOther, err := auth.UpdateSettings(SettingsPatch{Density: strPtr("compact")})
	if err != nil {
		t.Fatalf("UpdateSettings(Density): %v", err)
	}
	if !reflect.DeepEqual(afterOther.BotsSidebar, got) {
		t.Fatalf("bots_sidebar lost on unrelated patch: %+v", afterOther.BotsSidebar)
	}
}
