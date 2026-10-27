package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		DefaultProvider: "openai",
		Providers: map[string]config.ProviderConfig{
			"openai": {Type: "openai", BaseURL: "https://api.openai.com", APIKey: "sk-secret", DefaultModel: "gpt-4o"},
		},
		Bot: config.GlobalBotConfig{
			Type:          "private",
			OwnerIDs:      []string{"1", "2"},
			MaxIterations: cpIntPtr(100),
			BlockedTools:  []string{"bash"},
		},
		Heartbeat:   config.HeartbeatConfig{Active: true, Channel: "telegram", To: "42", Every: "1h"},
		Cron:        config.CronConfig{Active: true},
		Transcriber: config.TranscriberConfig{Provider: "openai", APIKey: "sk-transcriber"},
	}
}

func loadTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := loadAgentConfig("myfriend")
	if err != nil {
		t.Fatalf("loadAgentConfig: %v", err)
	}
	return cfg
}

func TestConfigPatchUpdatesOnlyGivenFields(t *testing.T) {
	setupConfigAgent(t, testConfig())
	svc := NewConfigService()

	botType := "public"
	to := "99"
	cronActive := false
	view, err := svc.Patch("myfriend", ConfigPatch{
		Bot:       &BotPatch{Type: &botType},
		Heartbeat: &HeartbeatPatch{To: &to},
		Cron:      &CronPatch{Active: &cronActive},
	})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}

	cfg := loadTestConfig(t)
	if cfg.Bot.Type != "public" {
		t.Fatalf("bot type = %q, want public", cfg.Bot.Type)
	}
	if got := strings.Join(cfg.Bot.OwnerIDs, ","); got != "1,2" {
		t.Fatalf("owner_ids = %q, want unchanged", got)
	}
	if cfg.Bot.MaxIterations == nil || *cfg.Bot.MaxIterations != 100 {
		t.Fatalf("max_iterations changed: %v", cfg.Bot.MaxIterations)
	}
	if cfg.Heartbeat.To != "99" || cfg.Heartbeat.Channel != "telegram" || cfg.Heartbeat.Every != "1h" {
		t.Fatalf("heartbeat = %+v", cfg.Heartbeat)
	}
	if cfg.Cron.Active {
		t.Fatal("cron.active should be false")
	}
	if cfg.Transcriber.Provider != "openai" || cfg.Transcriber.APIKey != "sk-transcriber" {
		t.Fatalf("transcriber changed: %+v", cfg.Transcriber)
	}
	if !strings.Contains(view.Bot.Type, "public") {
		t.Fatalf("view does not reflect the update: %+v", view)
	}
}

func TestConfigPatchDoesNotDeleteKeys(t *testing.T) {
	root := setupConfigAgent(t, testConfig())
	svc := NewConfigService()

	empty := []string{}
	if _, err := svc.Patch("myfriend", ConfigPatch{
		Bot: &BotPatch{OwnerIDs: &empty, BlockedTools: &empty},
	}); err != nil {
		t.Fatalf("Patch: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "config", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"owner_ids"`, `"max_iterations"`, `"providers"`, `"transcriber"`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("key %s was removed from config.json", key)
		}
	}

	cfg := loadTestConfig(t)
	if len(cfg.Bot.OwnerIDs) != 0 || len(cfg.Bot.BlockedTools) != 0 {
		t.Fatalf("slices were not cleared: %+v", cfg.Bot)
	}
}

func TestConfigPatchValidation(t *testing.T) {
	svc := NewConfigService()

	cases := []ConfigPatch{
		{Bot: &BotPatch{Type: cpStrPtr("secret")}},
		{Bot: &BotPatch{MaxIterations: cpIntPtr(0)}},
		{Heartbeat: &HeartbeatPatch{Channel: cpStrPtr("email")}},
		{Heartbeat: &HeartbeatPatch{Every: cpStrPtr("not-a-duration")}},
	}
	for _, patch := range cases {
		setupConfigAgent(t, testConfig())
		if _, err := svc.Patch("myfriend", patch); err == nil {
			t.Fatalf("Patch(%+v) should fail", patch)
		}
	}
}

func TestConfigPatchWarningBelowMax(t *testing.T) {
	setupConfigAgent(t, testConfig())
	svc := NewConfigService()

	if _, err := svc.Patch("myfriend", ConfigPatch{
		Bot: &BotPatch{MaxIterations: cpIntPtr(5), WarningIterations: cpIntPtr(10)},
	}); err == nil {
		t.Fatal("warning_iterations >= max_iterations should fail")
	}
}

func TestConfigResetFields(t *testing.T) {
	setupConfigAgent(t, testConfig())
	svc := NewConfigService()

	view, err := svc.ResetFields("myfriend", ConfigResetRequest{
		Bot:       []string{"type"},
		Heartbeat: cpBoolPtr(true),
	})
	if err != nil {
		t.Fatalf("ResetFields: %v", err)
	}

	cfg := loadTestConfig(t)
	if cfg.Bot.Type != "private" {
		t.Fatalf("bot type = %q, want reset to private", cfg.Bot.Type)
	}
	if got := strings.Join(cfg.Bot.OwnerIDs, ","); got != "1,2" {
		t.Fatalf("owner_ids = %q, want untouched", got)
	}
	if cfg.Heartbeat.Active || cfg.Heartbeat.Every != "30m" || cfg.Heartbeat.To != "" {
		t.Fatalf("heartbeat not reset to template: %+v", cfg.Heartbeat)
	}
	if !strings.Contains(view.Bot.Type, "private") {
		t.Fatalf("view does not reflect reset: %+v", view)
	}
}

func TestConfigResetFieldsWholeBot(t *testing.T) {
	setupConfigAgent(t, testConfig())
	svc := NewConfigService()

	if _, err := svc.ResetFields("myfriend", ConfigResetRequest{Bot: []string{}}); err != nil {
		t.Fatalf("ResetFields: %v", err)
	}
	cfg := loadTestConfig(t)
	if cfg.Bot.Type != "private" || len(cfg.Bot.OwnerIDs) != 0 || len(cfg.Bot.BlockedTools) != 0 {
		t.Fatalf("bot section not reset to template: %+v", cfg.Bot)
	}
}

func TestConfigResetFull(t *testing.T) {
	setupConfigAgent(t, testConfig())
	svc := NewConfigService()

	if _, err := svc.ResetConfig("myfriend"); err != nil {
		t.Fatalf("ResetConfig: %v", err)
	}
	cfg := loadTestConfig(t)
	if cfg.DefaultProvider != "" || len(cfg.Providers) != 0 {
		t.Fatalf("providers not reset: %+v", cfg.Providers)
	}
	if cfg.Bot.Type != "private" {
		t.Fatalf("bot type = %q", cfg.Bot.Type)
	}
	if cfg.Transcriber.Provider != "" || cfg.Synthesizer.Provider != "" {
		t.Fatalf("voice services not reset: %+v %+v", cfg.Transcriber, cfg.Synthesizer)
	}
	if cfg.Heartbeat.Active {
		t.Fatal("heartbeat should be inactive after reset")
	}
}

func cpStrPtr(v string) *string { return &v }
