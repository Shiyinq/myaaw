package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/agentdir"
)

func setupOnboardHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
}

func TestAgentOnboardCreatesAgent(t *testing.T) {
	setupOnboardHome(t)
	svc := NewAgentService()
	disabled := []string{"bash"}

	result, err := svc.Onboard("newbie", OnboardRequest{
		Provider: &OnboardProviderInput{
			Name:         "deepseek",
			Type:         "openai",
			BaseURL:      "https://api.deepseek.com",
			APIKey:       "sk-x",
			DefaultModel: "deepseek-chat",
		},
		Skills:       []string{"time"},
		DisableTools: &disabled,
		Channels: &OnboardChannelsInput{
			TelegramToken: "123:abc",
			TelegramMode:  "polling",
		},
		Heartbeat: &OnboardHeartbeatInput{Active: true, Channel: "telegram", To: "42", Every: "30m"},
		Bot:       &OnboardBotInput{Type: "public", OwnerIDs: []string{"42"}},
	})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if !result.Created {
		t.Fatal("expected Created = true")
	}

	cfg, err := loadAgentConfig("newbie")
	if err != nil {
		t.Fatalf("loadAgentConfig: %v", err)
	}
	if cfg.DefaultProvider != "deepseek" {
		t.Fatalf("default provider = %q", cfg.DefaultProvider)
	}
	provider := cfg.Providers["deepseek"]
	if provider.APIKey != "sk-x" || provider.BaseURL != "https://api.deepseek.com" || provider.DefaultModel != "deepseek-chat" {
		t.Fatalf("provider = %+v", provider)
	}
	if cfg.Channels.Telegram == nil || !cfg.Channels.Telegram.Active || cfg.Channels.Telegram.Token != "123:abc" {
		t.Fatalf("telegram = %+v", cfg.Channels.Telegram)
	}
	if !cfg.Heartbeat.Active || cfg.Heartbeat.Channel != "telegram" || cfg.Heartbeat.To != "42" {
		t.Fatalf("heartbeat = %+v", cfg.Heartbeat)
	}
	if cfg.Bot.Type != "public" {
		t.Fatalf("bot type = %q", cfg.Bot.Type)
	}
	if len(cfg.Bot.OwnerIDs) != 1 || cfg.Bot.OwnerIDs[0] != "42" {
		t.Fatalf("owner ids = %v", cfg.Bot.OwnerIDs)
	}

	skillsDir, err := agentdir.SkillsDir("newbie")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "time")); err != nil {
		t.Fatalf("time skill not installed: %v", err)
	}

	toolsDir, err := agentdir.ToolsDir("newbie")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(toolsDir, "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tools struct {
		Builtin map[string]bool `json:"builtin_tools"`
	}
	if err := json.Unmarshal(data, &tools); err != nil {
		t.Fatal(err)
	}
	if tools.Builtin["bash"] {
		t.Fatal("bash should be disabled")
	}
	if !tools.Builtin["filesystem"] {
		t.Fatal("filesystem should stay enabled")
	}
}

func TestAgentOnboardReconfiguresWithoutTouchingOtherSections(t *testing.T) {
	setupOnboardHome(t)
	svc := NewAgentService()

	first, err := svc.Onboard("agent-up", OnboardRequest{
		Provider: &OnboardProviderInput{Type: "openai", APIKey: "sk-orig", DefaultModel: "gpt-4o"},
		Channels: &OnboardChannelsInput{TelegramToken: "111:aaa"},
		Bot:      &OnboardBotInput{Type: "private", OwnerIDs: []string{"7"}},
	})
	if err != nil {
		t.Fatalf("first Onboard: %v", err)
	}
	if !first.Created {
		t.Fatal("first run should create")
	}

	second, err := svc.Onboard("agent-up", OnboardRequest{
		Bot: &OnboardBotInput{Type: "public"},
	})
	if err != nil {
		t.Fatalf("second Onboard: %v", err)
	}
	if second.Created {
		t.Fatal("second run should reconfigure, not create")
	}

	cfg, err := loadAgentConfig("agent-up")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bot.Type != "public" {
		t.Fatalf("bot type = %q, want public", cfg.Bot.Type)
	}
	if cfg.Providers["openai"].APIKey != "sk-orig" {
		t.Fatalf("provider changed: %+v", cfg.Providers["openai"])
	}
	if cfg.Channels.Telegram == nil || cfg.Channels.Telegram.Token != "111:aaa" {
		t.Fatalf("telegram changed: %+v", cfg.Channels.Telegram)
	}
	if len(cfg.Bot.OwnerIDs) != 1 || cfg.Bot.OwnerIDs[0] != "7" {
		t.Fatalf("owner ids changed: %v", cfg.Bot.OwnerIDs)
	}
}

func TestAgentOnboardValidation(t *testing.T) {
	setupOnboardHome(t)
	svc := NewAgentService()

	cases := []OnboardRequest{
		{Provider: &OnboardProviderInput{Type: "groq", APIKey: "x"}},
		{Provider: &OnboardProviderInput{Type: "openai"}},
		{Heartbeat: &OnboardHeartbeatInput{Active: true, Channel: "email", To: "1", Every: "30m"}},
		{Heartbeat: &OnboardHeartbeatInput{Active: true, Channel: "telegram", To: "1", Every: "nope"}},
		{Bot: &OnboardBotInput{Type: "secret"}},
		{Skills: []string{"does-not-exist"}},
	}
	for _, req := range cases {
		if _, err := svc.Onboard("agent-bad", req); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("Onboard(%+v) err = %v, want ErrInvalidConfig", req, err)
		}
	}
}
