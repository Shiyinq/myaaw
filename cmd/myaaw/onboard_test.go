package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/agentdir"
	"myaaw/internal/config"
)

func TestMergeOnboardConfigFresh(t *testing.T) {
	cfg := mergeOnboardConfig(nil, onboardInput{
		ProviderName:    "work",
		ProviderType:    "openai",
		ProviderBaseURL: "https://api.deepseek.com",
		ProviderAPIKey:  "sk-test",
		ProviderModel:   "deepseek-chat",
	})

	if cfg.DefaultProvider != "work" {
		t.Fatalf("DefaultProvider = %q, want %q", cfg.DefaultProvider, "work")
	}
	p, ok := cfg.Providers["work"]
	if !ok {
		t.Fatal("provider was not written")
	}
	if p.Type != "openai" || p.BaseURL != "https://api.deepseek.com" || p.APIKey != "sk-test" || p.DefaultModel != "deepseek-chat" {
		t.Fatalf("unexpected provider config: %+v", p)
	}
	if cfg.Channels.Telegram != nil || cfg.Channels.Discord != nil {
		t.Fatalf("skipped channels should stay unset, got telegram=%+v discord=%+v", cfg.Channels.Telegram, cfg.Channels.Discord)
	}
	if cfg.Heartbeat.Active {
		t.Fatal("heartbeat should stay disabled when the user skipped it")
	}
	if len(cfg.Bot.OwnerIDs) != 0 {
		t.Fatalf("owner IDs = %v, want empty", cfg.Bot.OwnerIDs)
	}
}

func TestMergeOnboardConfigHonorsHeartbeatChannel(t *testing.T) {
	cfg := mergeOnboardConfig(nil, onboardInput{
		ProviderName:     "openai",
		ProviderType:     "openai",
		TelegramToken:    "123:abc",
		TelegramMode:     "polling",
		DiscordToken:     "discord-token",
		Heartbeat:        true,
		HeartbeatSet:     true,
		HeartbeatChannel: "discord",
		HeartbeatTo:      "999",
		HeartbeatEvery:   "1h",
	})

	if cfg.Heartbeat.Channel != "discord" {
		t.Fatalf("heartbeat channel = %q, want discord (the user's choice)", cfg.Heartbeat.Channel)
	}
	if cfg.Heartbeat.To != "999" || cfg.Heartbeat.Every != "1h" {
		t.Fatalf("unexpected heartbeat config: %+v", cfg.Heartbeat)
	}
	if cfg.Channels.Telegram == nil || cfg.Channels.Telegram.Mode != "polling" {
		t.Fatalf("telegram config not written: %+v", cfg.Channels.Telegram)
	}
	if cfg.Channels.Discord == nil {
		t.Fatal("discord config not written")
	}
}

func TestMergeOnboardConfigKeepsSkippedChannels(t *testing.T) {
	base := &config.Config{
		Channels: config.ChannelsConfig{
			Telegram: &config.ChannelConfig{Active: true, Token: "existing", Mode: "webhook"},
		},
	}

	cfg := mergeOnboardConfig(base, onboardInput{ProviderName: "openai", ProviderType: "openai"})

	if cfg.Channels.Telegram == nil {
		t.Fatal("existing telegram config was dropped")
	}
	if !cfg.Channels.Telegram.Active || cfg.Channels.Telegram.Token != "existing" || cfg.Channels.Telegram.Mode != "webhook" {
		t.Fatalf("existing telegram config was modified: %+v", cfg.Channels.Telegram)
	}
	if base.Channels.Telegram.Token != "existing" {
		t.Fatal("merge mutated the input config")
	}
}

func TestMergeOnboardConfigReplacesOwners(t *testing.T) {
	base := &config.Config{Bot: config.GlobalBotConfig{OwnerIDs: []string{"1"}}}
	cfg := mergeOnboardConfig(base, onboardInput{OwnerIDs: []string{"2", "3"}, OwnersSet: true})

	if got := strings.Join(cfg.Bot.OwnerIDs, ","); got != "2,3" {
		t.Fatalf("owner IDs = %q, want 2,3", got)
	}
}

func TestSanitizeOnboardConfigRemovesTemplateValues(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "gemini",
		Providers: map[string]config.ProviderConfig{
			"gemini": {Type: "gemini", APIKey: "YOUR_GEMINI_API_KEY"},
			"real":   {Type: "openai", APIKey: "sk-real"},
		},
		Channels: config.ChannelsConfig{
			Telegram: &config.ChannelConfig{Active: true, Token: "YOUR_TELEGRAM_BOT_TOKEN", Mode: "polling"},
			Discord:  &config.ChannelConfig{Active: true, Token: "YOUR_DISCORD_BOT_TOKEN"},
		},
		Heartbeat: config.HeartbeatConfig{Active: true, Channel: "telegram", To: "123456789", Every: "30m"},
		Bot:       config.GlobalBotConfig{OwnerIDs: []string{"YOUR_OWNER_ID_1"}},
		Transcriber: config.TranscriberConfig{
			Provider: "groq",
			APIKey:   "YOUR_GROQ_API_KEY",
		},
	}

	sanitizeOnboardConfig(cfg)

	if _, ok := cfg.Providers["gemini"]; ok {
		t.Fatal("placeholder provider was not removed")
	}
	if _, ok := cfg.Providers["real"]; !ok {
		t.Fatal("real provider was removed")
	}
	if cfg.DefaultProvider != "" {
		t.Fatalf("DefaultProvider = %q, want empty", cfg.DefaultProvider)
	}
	if cfg.Channels.Telegram != nil {
		t.Fatalf("telegram placeholder channel should be removed, got %+v", cfg.Channels.Telegram)
	}
	if cfg.Channels.Discord != nil {
		t.Fatalf("discord placeholder channel should be removed, got %+v", cfg.Channels.Discord)
	}
	if cfg.Heartbeat.Active {
		t.Fatal("placeholder heartbeat still active")
	}
	if len(cfg.Bot.OwnerIDs) != 0 {
		t.Fatalf("placeholder owners = %v, want empty", cfg.Bot.OwnerIDs)
	}
	if cfg.Transcriber.Provider != "" || cfg.Transcriber.APIKey != "" {
		t.Fatalf("placeholder transcriber not cleared: %+v", cfg.Transcriber)
	}
}

func TestSanitizeOnboardConfigKeepsRealValues(t *testing.T) {
	cfg := &config.Config{
		DefaultProvider: "openai",
		Providers:       map[string]config.ProviderConfig{"openai": {Type: "openai", APIKey: "sk-real"}},
		Channels:        config.ChannelsConfig{Telegram: &config.ChannelConfig{Active: true, Token: "123:abc"}},
		Heartbeat:       config.HeartbeatConfig{Active: true, Channel: "telegram", To: "42", Every: "30m"},
		Bot:             config.GlobalBotConfig{OwnerIDs: []string{"42"}},
	}

	sanitizeOnboardConfig(cfg)

	if cfg.DefaultProvider != "openai" || cfg.Providers["openai"].APIKey != "sk-real" {
		t.Fatal("real provider was modified")
	}
	if !cfg.Channels.Telegram.Active {
		t.Fatal("real channel was disabled")
	}
	if !cfg.Heartbeat.Active || cfg.Heartbeat.To != "42" {
		t.Fatalf("real heartbeat was modified: %+v", cfg.Heartbeat)
	}
	if len(cfg.Bot.OwnerIDs) != 1 || cfg.Bot.OwnerIDs[0] != "42" {
		t.Fatalf("real owner was dropped: %v", cfg.Bot.OwnerIDs)
	}
}

func TestValidateAgentNameInput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := validateAgentNameInput("agent-b"); err != nil {
		t.Fatalf("valid name rejected: %v", err)
	}
	if err := validateAgentNameInput("agent_working"); err != nil {
		t.Fatalf("underscore name rejected: %v", err)
	}
	if err := validateAgentNameInput("agent-personal-working"); err != nil {
		t.Fatalf("multi-segment name rejected: %v", err)
	}

	for _, name := range []string{"", "-bad", "bad name", "pids", strings.Repeat("a", 65)} {
		if err := validateAgentNameInput(name); err == nil {
			t.Fatalf("name %q should be rejected", name)
		}
	}

	root, err := agentdir.AgentRoot("agent-b")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateAgentNameInput("agent-b"); err == nil {
		t.Fatal("already initialized agent should be rejected")
	}

	// A leftover directory from an interrupted run also blocks the name.
	leftoverRoot, err := agentdir.AgentRoot("leftover")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(leftoverRoot, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateAgentNameInput("leftover"); err == nil {
		t.Fatal("leftover directory should be rejected")
	}
}

func TestInitialOnboardModelAgentName(t *testing.T) {
	t.Setenv("HOME", "/tmp/onboard-home")

	m := initialOnboardModel("agent-b")
	if m.agentName != "agent-b" {
		t.Fatalf("agentName = %q", m.agentName)
	}
	want := filepath.Join("/tmp/onboard-home", ".myaaw", "agent-b")
	if m.myaawHome != want {
		t.Fatalf("myaawHome = %q, want %q", m.myaawHome, want)
	}
	if m.envFile != filepath.Join(want, ".env") {
		t.Fatalf("envFile = %q", m.envFile)
	}
	if m.configFile != filepath.Join(want, "config", "config.json") {
		t.Fatalf("configFile = %q", m.configFile)
	}

	empty := initialOnboardModel("")
	if empty.agentName != "" {
		t.Fatalf("agentName = %q, want empty so the wizard asks for it", empty.agentName)
	}
}

func TestParseOwnerIDs(t *testing.T) {
	ids, err := parseOwnerIDs(" 42, -100200, 42 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "42" || ids[1] != "-100200" {
		t.Fatalf("ids = %v", ids)
	}

	ids, err = parseOwnerIDs("   ")
	if err != nil || ids != nil {
		t.Fatalf("empty input should yield no IDs, got %v err=%v", ids, err)
	}

	if _, err := parseOwnerIDs("abc"); err == nil {
		t.Fatal("non-numeric owner should be rejected")
	}
}

func TestValidateChatIDAndTelegramToken(t *testing.T) {
	for _, id := range []string{"42", "-100200"} {
		if err := validateChatID(id); err != nil {
			t.Fatalf("chat ID %q rejected: %v", id, err)
		}
	}
	for _, id := range []string{"", "-", "12a"} {
		if err := validateChatID(id); err == nil {
			t.Fatalf("chat ID %q should be rejected", id)
		}
	}

	if err := validateTelegramToken("123456:ABC-def"); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	for _, token := range []string{"", "not-a-token"} {
		if err := validateTelegramToken(token); err == nil {
			t.Fatalf("token %q should be rejected", token)
		}
	}
}
