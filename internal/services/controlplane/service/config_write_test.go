package service

import (
	"errors"
	"testing"

	"myaaw/internal/config"
)

func TestConfigServiceProviderWrites(t *testing.T) {
	setupConfigAgent(t, &config.Config{})
	configSvc := NewConfigService()

	created, err := configSvc.CreateProvider("myfriend", ProviderInput{
		Name:         "openai",
		Type:         "openai",
		BaseURL:      "https://api.openai.com",
		APIKey:       "sk-secret",
		DefaultModel: "gpt-4o",
	})
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if !created.APIKey.Configured {
		t.Fatalf("provider = %+v", created)
	}

	cfg, err := loadAgentConfig("myfriend")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DefaultProvider != "openai" {
		t.Fatalf("default = %q", cfg.DefaultProvider)
	}

	model := "gpt-4o-mini"
	updated, err := configSvc.UpdateProvider("myfriend", "openai", ProviderPatch{DefaultModel: &model})
	if err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	if updated.DefaultModel != model {
		t.Fatalf("model = %q", updated.DefaultModel)
	}
	cfg, _ = loadAgentConfig("myfriend")
	if cfg.Providers["openai"].APIKey != "sk-secret" {
		t.Fatal("api key should be preserved when the patch omits it")
	}

	if _, err := configSvc.CreateProvider("myfriend", ProviderInput{Name: "local", Type: "openai"}); err != nil {
		t.Fatalf("CreateProvider local: %v", err)
	}
	if err := configSvc.SetDefaultProvider("myfriend", "local"); err != nil {
		t.Fatalf("SetDefaultProvider: %v", err)
	}
	if err := configSvc.DeleteProvider("myfriend", "local"); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	cfg, _ = loadAgentConfig("myfriend")
	if cfg.DefaultProvider != "openai" {
		t.Fatalf("default after delete = %q", cfg.DefaultProvider)
	}

	if err := configSvc.DeleteProvider("myfriend", "missing"); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("err = %v, want ErrProviderNotFound", err)
	}
}

func TestConfigServiceChannelWrites(t *testing.T) {
	setupConfigAgent(t, &config.Config{})
	configSvc := NewConfigService()

	active := true
	view, err := configSvc.UpsertChannel("myfriend", "telegram", ChannelInput{
		Active: &active,
		Token:  "telegram-token",
		Mode:   "polling",
	})
	if err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
	if !view.Token.Configured || view.Mode != "polling" {
		t.Fatalf("channel = %+v", view)
	}

	inactive := false
	view, err = configSvc.UpsertChannel("myfriend", "telegram", ChannelInput{Active: &inactive})
	if err != nil {
		t.Fatalf("UpsertChannel update: %v", err)
	}
	if view.Active || !view.Token.Configured {
		t.Fatalf("channel = %+v", view)
	}

	if err := configSvc.DisableChannel("myfriend", "telegram"); err != nil {
		t.Fatalf("DisableChannel: %v", err)
	}
	cfg, _ := loadAgentConfig("myfriend")
	if cfg.Channels.Telegram.Active {
		t.Fatal("telegram should be inactive")
	}
	if cfg.Channels.Telegram.Token != "telegram-token" {
		t.Fatal("disabling should keep the token")
	}

	if err := configSvc.DisableChannel("myfriend", "discord"); !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("err = %v, want ErrChannelNotFound", err)
	}
	if _, err := configSvc.UpsertChannel("myfriend", "slack", ChannelInput{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
}
