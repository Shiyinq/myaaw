package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"myaaw/internal/agentdir"
)

func TestProviderAuthConfigRoundTripPreservesLegacyAPIKey(t *testing.T) {
	cfg := Config{DefaultProvider: "api", Providers: map[string]ProviderConfig{
		"api":          {Type: "openai", BaseURL: "https://compatible.example", APIKey: "original-key", DefaultModel: "original-model"},
		"subscription": {Type: "openai_chatgpt", AuthMode: "oauth", CredentialID: "local-reference", DefaultModel: "account-model"},
	}}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var restored Config
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	for name, original := range cfg.Providers {
		if restored.Providers[name] != original {
			t.Fatalf("provider %s changed during round trip", name)
		}
	}
}

func TestReloadProviderAuthDoesNotRetainSubscriptionCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LLM_PROVIDER_NAME", "openai")
	t.Setenv("LLM_PROVIDER_API_KEY", "environment-key")
	previous := ProviderConfig{Type: LLMProviderName, BaseURL: LLMProviderBaseURL, APIKey: LLMProviderAPIKey,
		AuthMode: LLMProviderAuthMode, CredentialID: LLMProviderCredentialID, DefaultModel: LLMDefaultModel}
	previousID := CurrentProviderID
	t.Cleanup(func() {
		LLMProviderName, LLMProviderBaseURL, LLMProviderAPIKey = previous.Type, previous.BaseURL, previous.APIKey
		LLMProviderAuthMode, LLMProviderCredentialID, LLMDefaultModel = previous.AuthMode, previous.CredentialID, previous.DefaultModel
		CurrentProviderID = previousID
	})
	path, err := agentdir.ActiveConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	write := func(body string) {
		t.Helper()
		if err := WriteConfigAtomic(path, []byte(body)); err != nil {
			t.Fatal(err)
		}
		LoadBaseConfig()
	}
	write(`{"default_provider":"subscription","providers":{"subscription":{"type":"openai_chatgpt","auth_mode":"oauth","credential_id":"local-account","default_model":"account-model"}}}`)
	if LLMProviderName != "openai_chatgpt" || LLMProviderAuthMode != "oauth" || LLMProviderCredentialID != "local-account" || LLMProviderAPIKey != "" {
		t.Fatal("subscription configuration was not loaded independently of environment API key")
	}
	write(`{"default_provider":"api","providers":{"api":{"type":"openai","api_key":"saved-api-key","default_model":"api-model"}}}`)
	if LLMProviderName != "openai" || LLMProviderAPIKey != "saved-api-key" || LLMProviderAuthMode != "" || LLMProviderCredentialID != "" {
		t.Fatal("switch to API key retained OAuth credentials or changed API key")
	}
	write(`{}`)
	if CurrentProviderID != "" || LLMProviderName != "openai" || LLMProviderAPIKey != "environment-key" || LLMProviderAuthMode != "" || LLMProviderCredentialID != "" {
		t.Fatal("removing configured provider retained a stale selection")
	}
}

func TestWriteConfigAtomicReplacesLegacyPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"old":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"providers":{"api":{"type":"openai","api_key":"private"}}}`)
	if err := WriteConfigAtomic(path, body); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(body) {
		t.Fatalf("config replacement failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %o, want 600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary config files remain: %v", err)
	}
}
