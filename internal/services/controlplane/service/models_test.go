package service

import (
	"errors"
	"testing"

	"myaaw/internal/config"
)

func TestModelsForCredentialsValidation(t *testing.T) {
	svc := NewConfigService()
	if _, err := svc.ModelsForCredentials(ModelLookupInput{Type: "does-not-exist"}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
}

func TestModelsForCredentialsFallsBackToDefault(t *testing.T) {
	svc := NewConfigService()
	view, err := svc.ModelsForCredentials(ModelLookupInput{
		Type:    "openai",
		BaseURL: "http://127.0.0.1:1",
		APIKey:  "sk-x",
	})
	if err != nil {
		t.Fatalf("ModelsForCredentials: %v", err)
	}
	if view.Source != "default" {
		t.Fatalf("source = %q, want default", view.Source)
	}
	if len(view.Models) == 0 || view.Models[0] != "gpt-4o" {
		t.Fatalf("models = %v, want the built-in default", view.Models)
	}
	if view.Warning == "" {
		t.Fatal("expected a warning describing the failed live fetch")
	}
}

func TestProviderModelsUsesSavedProvider(t *testing.T) {
	setupConfigAgent(t, &config.Config{
		DefaultProvider: "openai",
		Providers: map[string]config.ProviderConfig{
			"openai": {Type: "openai", BaseURL: "http://127.0.0.1:1", APIKey: "sk-x", DefaultModel: "gpt-4o"},
		},
	})
	svc := NewConfigService()

	view, err := svc.ProviderModels("myfriend", "openai")
	if err != nil {
		t.Fatalf("ProviderModels: %v", err)
	}
	if view.Source != "default" || len(view.Models) == 0 {
		t.Fatalf("view = %+v", view)
	}

	if _, err := svc.ProviderModels("myfriend", "missing"); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("err = %v, want ErrProviderNotFound", err)
	}
}
