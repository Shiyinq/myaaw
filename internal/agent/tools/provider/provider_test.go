package provider

import (
	"fmt"
	"strings"
	"testing"

	"myaaw/internal/config"
	baseProvider "myaaw/internal/provider"
)

type modelProvider struct {
	baseProvider.LLMProvider
	models func() ([]string, error)
}

func (p *modelProvider) Models() ([]string, error) { return p.models() }

func TestFetchModelsUsesExplicitConfigWithoutGlobalOverrides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	oldBase, oldModel, oldMode, oldID := config.LLMProviderBaseURL, config.LLMDefaultModel, config.LLMProviderAuthMode, config.LLMProviderCredentialID
	t.Cleanup(func() {
		config.LLMProviderBaseURL, config.LLMDefaultModel, config.LLMProviderAuthMode, config.LLMProviderCredentialID = oldBase, oldModel, oldMode, oldID
	})
	config.LLMProviderBaseURL, config.LLMDefaultModel = "https://active.invalid", "active-model"
	config.LLMProviderAuthMode, config.LLMProviderCredentialID = "oauth", "active-account"
	factory := baseProvider.LLMproviderFactories["openai"]
	t.Cleanup(func() { baseProvider.LLMproviderFactories["openai"] = factory })
	baseProvider.LLMproviderFactories["openai"] = func(base, key, model string) baseProvider.LLMProvider {
		if base != "https://target.invalid" || key != "target-api-key" || model != "target-model" {
			t.Errorf("wrong explicit provider: %q / %q / %q", base, key, model)
		}
		return &modelProvider{models: func() ([]string, error) {
			if config.LLMProviderBaseURL != "https://active.invalid" || config.LLMDefaultModel != "active-model" || config.LLMProviderCredentialID != "active-account" {
				return nil, fmt.Errorf("active globals were temporarily overwritten")
			}
			return []string{"available-model"}, nil
		}}
	}
	cfg := &config.Config{Providers: map[string]config.ProviderConfig{
		"target":                {Type: "openai", BaseURL: "https://target.invalid", APIKey: "target-api-key", DefaultModel: "target-model"},
		"disconnected":          {Type: "openai_chatgpt", AuthMode: "oauth", DefaultModel: "subscription-model"},
		"missing-local-account": {Type: "openai_chatgpt", AuthMode: "oauth", CredentialID: "11111111-1111-4111-8111-111111111111", DefaultModel: "subscription-model"},
	}}
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	tool := NewProviderTool()
	if got := tool.CallTool(`{"action":"fetch_models","name":"target"}`, nil); !strings.Contains(got, "available-model") {
		t.Fatal(got)
	}
	if got := tool.CallTool(`{"action":"fetch_models","name":"disconnected"}`, nil); !strings.Contains(got, "select a credential_id") {
		t.Fatalf("borrowed selected account for disconnected target: %s", got)
	}
	// The target's credential gets as far as local credential lookup, despite
	// the active credential ID being invalid. No HTTP request is possible.
	if got := tool.CallTool(`{"action":"fetch_models","name":"missing-local-account"}`, nil); !strings.Contains(got, "credential unavailable") {
		t.Fatalf("target OAuth credential not selected: %s", got)
	}
	if config.LLMProviderBaseURL != "https://active.invalid" || config.LLMProviderAuthMode != "oauth" || config.LLMProviderCredentialID != "active-account" {
		t.Fatal("fetch_models mutated active globals")
	}
}
