package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/config"
)

func setupConfigAgent(t *testing.T, cfg *config.Config) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	agentRoot := filepath.Join(home, ".myaaw", "myfriend")
	if err := os.MkdirAll(filepath.Join(agentRoot, "home"), 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentRoot, "home", "AGENTS.md"), []byte("test"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if cfg != nil {
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatalf("marshal config: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(agentRoot, "config"), 0o755); err != nil {
			t.Fatalf("mkdir config: %v", err)
		}
		if err := os.WriteFile(filepath.Join(agentRoot, "config", "config.json"), data, 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	return agentRoot
}

func TestConfigServiceProvidersAndChannels(t *testing.T) {
	setupConfigAgent(t, &config.Config{
		DefaultProvider: "openai",
		Providers: map[string]config.ProviderConfig{
			"openai": {Type: "openai", BaseURL: "https://api.openai.com", APIKey: "sk-secret", DefaultModel: "gpt-4o"},
			"local":  {Type: "openai", BaseURL: "http://localhost:11434", DefaultModel: "llama3"},
		},
		Channels: config.ChannelsConfig{
			Telegram: &config.ChannelConfig{Active: true, Token: "telegram-secret", Mode: "polling"},
			Discord:  &config.ChannelConfig{Active: false, Token: ""},
		},
	})

	configSvc := NewConfigService()
	providers, err := configSvc.Providers("myfriend")
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	if providers.Default != "openai" {
		t.Fatalf("default = %q", providers.Default)
	}
	if len(providers.Providers) != 2 {
		t.Fatalf("providers = %+v", providers.Providers)
	}
	if providers.Providers[0].Name != "local" || providers.Providers[1].Name != "openai" {
		t.Fatalf("providers not sorted: %+v", providers.Providers)
	}
	if !providers.Providers[1].APIKey.Configured || providers.Providers[1].APIKey.Masked != "********" {
		t.Fatalf("api key not masked: %+v", providers.Providers[1])
	}
	if providers.Providers[1].BaseURL != "https://api.openai.com" {
		t.Fatalf("base url = %q", providers.Providers[1].BaseURL)
	}

	channels, err := configSvc.Channels("myfriend")
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	if len(channels.Channels) != 2 {
		t.Fatalf("channels = %+v", channels.Channels)
	}
	if channels.Channels[0].Name != "telegram" || !channels.Channels[0].Token.Configured {
		t.Fatalf("telegram = %+v", channels.Channels[0])
	}
	if channels.Channels[1].Token.Configured {
		t.Fatalf("discord token should not be configured: %+v", channels.Channels[1])
	}
}

func TestConfigServiceViewMasksAllSecrets(t *testing.T) {
	setupConfigAgent(t, &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {Type: "openai", APIKey: "sk-secret"},
		},
		Channels: config.ChannelsConfig{
			Telegram: &config.ChannelConfig{Token: "telegram-secret"},
		},
		Transcriber: config.TranscriberConfig{Provider: "gemini", APIKey: "transcriber-secret"},
		Synthesizer: config.SynthesizerConfig{Provider: "gemini", APIKey: "synthesizer-secret"},
	})

	view, err := NewConfigService().Config("myfriend")
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if !view.Transcriber.APIKey.Configured || !view.Synthesizer.APIKey.Configured {
		t.Fatalf("service keys not masked: %+v / %+v", view.Transcriber, view.Synthesizer)
	}
	if view.Providers[0].APIKey.Configured != true {
		t.Fatalf("provider key not masked: %+v", view.Providers[0])
	}
}

func TestConfigServiceMissingConfig(t *testing.T) {
	setupConfigAgent(t, nil)

	if _, err := NewConfigService().Providers("myfriend"); err != ErrConfigNotFound {
		t.Fatalf("err = %v, want ErrConfigNotFound", err)
	}
	if _, err := NewConfigService().Providers("missing"); err != ErrAgentNotFound {
		t.Fatalf("err = %v, want ErrAgentNotFound", err)
	}
}
