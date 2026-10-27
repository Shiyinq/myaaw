package main

import (
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/agentdir"
	"myaaw/internal/config"
)

func TestHeadlessInputValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MYAAW_API_KEY", "")
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("DISCORD_BOT_TOKEN", "")
	t.Setenv("MYAAW_WEBHOOK_URL", "")

	base := onboardOptions{
		provider: "openai",
		apiKey:   "sk-test",
		model:    "gpt-4o",
	}

	cases := []struct {
		name    string
		opts    onboardOptions
		wantErr bool
	}{
		{name: "minimal", opts: base},
		{name: "missing provider", opts: onboardOptions{apiKey: "sk-test"}, wantErr: true},
		{name: "unknown provider", opts: onboardOptions{provider: "anthropic", apiKey: "sk-test"}, wantErr: true},
		{name: "missing api key", opts: onboardOptions{provider: "openai"}, wantErr: true},
		{name: "telegram without token", opts: onboardOptions{provider: "openai", apiKey: "sk", channel: "telegram"}, wantErr: true},
		{name: "webhook without url", opts: onboardOptions{provider: "openai", apiKey: "sk", channel: "telegram", tgToken: "1:abc", tgMode: "webhook"}, wantErr: true},
		{name: "webhook with http url", opts: onboardOptions{provider: "openai", apiKey: "sk", channel: "telegram", tgToken: "1:abc", tgMode: "webhook", webhook: "http://example.com"}, wantErr: true},
		{name: "webhook ok", opts: onboardOptions{provider: "openai", apiKey: "sk", channel: "telegram", tgToken: "1:abc", tgMode: "webhook", webhook: "https://example.com/"}},
		{name: "heartbeat without id", opts: onboardOptions{provider: "openai", apiKey: "sk", channel: "telegram", tgToken: "1:abc", hbChannel: "telegram"}, wantErr: true},
		{name: "heartbeat bad interval", opts: onboardOptions{provider: "openai", apiKey: "sk", channel: "telegram", tgToken: "1:abc", hbChannel: "telegram", hbTo: "42", hbEvery: "soon"}, wantErr: true},
		{name: "heartbeat ok", opts: onboardOptions{provider: "openai", apiKey: "sk", channel: "telegram", tgToken: "1:abc", hbChannel: "telegram", hbTo: "42"}},
		{name: "bad owner", opts: onboardOptions{provider: "openai", apiKey: "sk", owner: "abc"}, wantErr: true},
		{name: "bad port", opts: onboardOptions{provider: "openai", apiKey: "sk", port: 70000}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, err := headlessInput("agent-headless", tc.opts)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if in.ProviderName == "" || in.ProviderModel == "" {
				t.Fatalf("incomplete input: %+v", in)
			}
		})
	}
}

func TestHeadlessRunsEndToEnd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MYAAW_API_KEY", "sk-from-env")
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	defer func() { onboardOpts = onboardOptions{} }()
	onboardOpts = onboardOptions{
		yes:      true,
		provider: "gemini",
		model:    "gemini-2.5-flash",
		channel:  "skip",
		owner:    "42, -100200",
		port:     8123,
	}

	if code := runOnboardHeadless("agent-ci"); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	cfg := loadAgentConfig("agent-ci")
	if cfg == nil {
		t.Fatal("config.json was not written")
	}
	if cfg.DefaultProvider != "gemini" || cfg.Providers["gemini"].APIKey != "sk-from-env" {
		t.Fatalf("unexpected provider config: %+v", cfg.Providers)
	}
	if len(cfg.Bot.OwnerIDs) != 2 {
		t.Fatalf("owner IDs = %v", cfg.Bot.OwnerIDs)
	}
	if cfg.Channels.Telegram != nil || cfg.Channels.Discord != nil {
		t.Fatal("channels should stay unset when --channel skip is used")
	}

	root, err := agentdir.AgentRoot("agent-ci")
	if err != nil {
		t.Fatal(err)
	}
	if port := readEnvValue(filepath.Join(root, ".env"), "PORT"); port != "8123" {
		t.Fatalf("PORT = %q, want 8123", port)
	}
	if base := readEnvValue(filepath.Join(root, ".env"), "MYAAW_BASE_URL"); base != "http://localhost:8123" {
		t.Fatalf("MYAAW_BASE_URL = %q", base)
	}

	// Re-running without --force must refuse to overwrite.
	if code := runOnboardHeadless("agent-ci"); code == 0 {
		t.Fatal("expected a non-zero exit code without --force")
	}
	onboardOpts.force = true
	if code := runOnboardHeadless("agent-ci"); code != 0 {
		t.Fatal("expected --force to allow reconfiguration")
	}
}

func TestHeadlessRefusesLeftoverDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MYAAW_API_KEY", "sk-from-env")
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	root, err := agentdir.AgentRoot("leftover")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}

	defer func() { onboardOpts = onboardOptions{} }()
	onboardOpts = onboardOptions{yes: true, provider: "openai", model: "gpt-4o", channel: "skip"}

	if code := runOnboardHeadless("leftover"); code == 0 {
		t.Fatal("a leftover directory should require --force")
	}
	onboardOpts.force = true
	if code := runOnboardHeadless("leftover"); code != 0 {
		t.Fatal("expected --force to reuse the leftover directory")
	}
}

func TestPreviousStep(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := initialOnboardModel("agent-b")
	m.providerType = "openai"
	m.providerName = "deepseek"
	m.providerBaseURL = "https://api.deepseek.com"
	m.llmKey = "sk-test"
	m.providerModel = "deepseek-chat"
	m.modelManual = true

	m.step = stepChannelChoice
	back := m.previousStep()
	if back.step != stepProviderModel || !back.modelManual {
		t.Fatalf("step = %d manual=%v, want manual model step", back.step, back.modelManual)
	}
	if back.textInput.Value() != "deepseek-chat" {
		t.Fatalf("model input = %q", back.textInput.Value())
	}

	m.step = stepProviderModel
	back = m.previousStep()
	if back.step != stepProviderKey || back.textInput.Value() != "sk-test" {
		t.Fatalf("step = %d key=%q, want provider key step", back.step, back.textInput.Value())
	}

	m.telegramMode = "webhook"
	m.step = stepTelegramToken
	back = m.previousStep()
	if back.step != stepTelegramMode || back.cursor != 1 {
		t.Fatalf("step = %d cursor = %d, want webhook mode selected", back.step, back.cursor)
	}

	m.step = stepTelegramWebhookURL
	back = m.previousStep()
	if back.step != stepTelegramToken {
		t.Fatalf("step = %d, want telegram token", back.step)
	}
}

func TestValidateWebhookURL(t *testing.T) {
	if err := validateWebhookURL("https://agent-b.example.com"); err != nil {
		t.Fatalf("valid URL rejected: %v", err)
	}
	for _, value := range []string{
		"",
		"http://agent-b.example.com",
		"https://agent-b.example.com/webhook/telegram",
		"agent-b.example.com",
	} {
		if err := validateWebhookURL(value); err == nil {
			t.Fatalf("URL %q should be rejected", value)
		}
	}
}

func TestMergeOnboardConfigWebhookFields(t *testing.T) {
	cfg := mergeOnboardConfig(&config.Config{}, onboardInput{
		ProviderName:       "openai",
		ProviderType:       "openai",
		TelegramToken:      "123:abc",
		TelegramMode:       "webhook",
		TelegramWebhookURL: "https://agent-b.example.com",
	})

	if cfg.Channels.Telegram == nil || cfg.Channels.Telegram.Mode != "webhook" {
		t.Fatalf("telegram webhook mode not written: %+v", cfg.Channels.Telegram)
	}
}
