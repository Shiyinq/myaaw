package subagent

import (
	"strings"
	"testing"

	"myaaw/internal/config"
	"myaaw/internal/provider"
)

func TestSubAgentUsesSelectedProviderCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	name, base, key, model, mode, id := config.LLMProviderName, config.LLMProviderBaseURL, config.LLMProviderAPIKey, config.LLMDefaultModel, config.LLMProviderAuthMode, config.LLMProviderCredentialID
	t.Cleanup(func() {
		config.LLMProviderName, config.LLMProviderBaseURL, config.LLMProviderAPIKey = name, base, key
		config.LLMDefaultModel, config.LLMProviderAuthMode, config.LLMProviderCredentialID = model, mode, id
	})
	factory := provider.LLMproviderFactories["openai"]
	t.Cleanup(func() { provider.LLMproviderFactories["openai"] = factory })
	called := false
	provider.LLMproviderFactories["openai"] = func(base, key, model string) provider.LLMProvider {
		called = true
		if base != "https://selected.invalid" || key != "selected-key" || model != "selected-model" {
			t.Fatal("subagent lost selected API-key config")
		}
		return provider.NewOpenAIProvider(base, key, model)
	}
	config.LLMProviderName, config.LLMProviderBaseURL, config.LLMProviderAPIKey = "openai", "https://selected.invalid", "selected-key"
	config.LLMDefaultModel, config.LLMProviderAuthMode, config.LLMProviderCredentialID = "selected-model", "", ""
	if _, err := createSubAgentProvider(); err != nil || !called {
		t.Fatalf("legacy selected provider: %v", err)
	}
	config.LLMProviderName, config.LLMProviderBaseURL, config.LLMProviderAPIKey = "openai_chatgpt", "", ""
	config.LLMProviderAuthMode, config.LLMProviderCredentialID = "oauth", "11111111-1111-4111-8111-111111111111"
	p, err := createSubAgentProvider()
	if err != nil {
		t.Fatal(err)
	}
	if p.ProviderName() != "openai_chatgpt" || p.DefaultModel("") != "selected-model" {
		t.Fatal("wrong selected OAuth provider")
	}
	if _, err := p.Models(); err == nil || !strings.Contains(err.Error(), "credential unavailable") {
		t.Fatalf("selected credential did not reach local lookup: %v", err)
	}
	config.LLMProviderCredentialID = ""
	if _, err := createSubAgentProvider(); err == nil {
		t.Fatal("disconnected subagent reused an old credential")
	}
}
