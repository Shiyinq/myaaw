package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myaaw/internal/auth/chatgpt"
	"myaaw/internal/config"
	"myaaw/internal/onboard"
)

const testChatGPTID = "11111111-1111-4111-8111-111111111111"

func writeChatGPTCredential(t *testing.T, expires time.Time) {
	t.Helper()
	if _, err := chatgpt.DefaultManager(); err != nil {
		t.Fatal(err)
	}
	credential := chatgpt.Credential{ID: testChatGPTID, ClientID: "oaiapp_test", Subject: "subject", Email: "account@example.test",
		AccessToken: "secret-access", RefreshToken: "secret-refresh", IDToken: "secret-identity", ExpiresAt: expires,
		Scopes: []string{"openid", "offline_access", "resource.invoke", "chatgpt.tokens.use.direct"}}
	data, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(os.Getenv("HOME"), ".myaaw", "credentials", "chatgpt", "accounts", testChatGPTID+".json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestChatGPTProviderValidationAndSwitching(t *testing.T) {
	setupConfigAgent(t, &config.Config{})
	writeChatGPTCredential(t, time.Now().Add(time.Hour))
	svc := NewConfigService()
	input := ProviderInput{Name: "subscription", Type: chatgpt.ProviderType, CredentialID: testChatGPTID, DefaultModel: "discovered-model"}
	for _, bad := range []ProviderInput{
		{Name: "bad", Type: chatgpt.ProviderType},
		{Name: "bad", Type: "OPENAI_CHATGPT"},
		{Name: "bad", Type: chatgpt.ProviderType, CredentialID: "22222222-2222-4222-8222-222222222222"},
		{Name: "bad", Type: chatgpt.ProviderType, CredentialID: testChatGPTID, APIKey: "sk-secret"},
		{Name: "bad", Type: chatgpt.ProviderType, CredentialID: testChatGPTID, BaseURL: "https://evil.example"},
		{Name: "bad", Type: chatgpt.ProviderType, CredentialID: testChatGPTID, AuthMode: "api_key"},
	} {
		if _, err := svc.CreateProvider("myfriend", bad); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("invalid create err = %v", err)
		}
	}
	created, err := svc.CreateProvider("myfriend", input)
	if err != nil {
		t.Fatal(err)
	}
	if created.AuthMode != "oauth" || created.CredentialID != testChatGPTID || created.APIKey.Configured || created.Connection == nil || created.Connection.Status != "connected" || created.Connection.AccountLabel != "account@example.test" {
		t.Fatalf("metadata = %+v", created)
	}
	body, _ := json.Marshal(created)
	for _, secret := range []string{"secret-access", "secret-refresh", "secret-identity", "access_token", "refresh_token", "id_token", "oaiapp_test"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("secret exposed: %s", body)
		}
	}
	badKey, badURL, badID := "new-secret", "https://evil.example", "missing"
	for _, patch := range []ProviderPatch{{APIKey: &badKey}, {BaseURL: &badURL}, {CredentialID: &badID}} {
		if _, err := svc.UpdateProvider("myfriend", "subscription", patch); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("invalid patch err = %v", err)
		}
	}
	openAI, key := "openai", "sk-original"
	if _, err := svc.UpdateProvider("myfriend", "subscription", ProviderPatch{Type: &openAI, APIKey: &key}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := loadAgentConfig("myfriend")
	p := cfg.Providers["subscription"]
	if p.AuthMode != "" || p.CredentialID != "" || p.APIKey != key {
		t.Fatalf("type switch retained OAuth: %+v", p)
	}
	blank := ""
	if _, err := svc.UpdateProvider("myfriend", "subscription", ProviderPatch{APIKey: &blank}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = loadAgentConfig("myfriend")
	if cfg.Providers["subscription"].APIKey != key {
		t.Fatal("empty key patch must preserve existing API-key behavior")
	}
	oauth := chatgpt.ProviderType
	id := testChatGPTID
	if _, err := svc.UpdateProvider("myfriend", "subscription", ProviderPatch{Type: &oauth, CredentialID: &id}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = loadAgentConfig("myfriend")
	if cfg.Providers["subscription"].APIKey != "" {
		t.Fatal("switch to OAuth retained API key")
	}
}

func TestChatGPTDisconnectPreservesSharedCredential(t *testing.T) {
	p := config.ProviderConfig{Type: chatgpt.ProviderType, AuthMode: "oauth", CredentialID: testChatGPTID}
	setupConfigAgent(t, &config.Config{Providers: map[string]config.ProviderConfig{"one": p, "two": p}})
	writeChatGPTCredential(t, time.Now().Add(time.Hour))
	svc := NewConfigService()
	if err := svc.DisconnectProvider("myfriend", "one"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := loadAgentConfig("myfriend")
	if cfg.Providers["one"].CredentialID != "" || cfg.Providers["two"].CredentialID != testChatGPTID {
		t.Fatal("disconnect changed another reference")
	}
	manager, _ := chatgpt.DefaultManager()
	if _, err := manager.Get(testChatGPTID); err != nil {
		t.Fatal("disconnect deleted shared credential", err)
	}
	view := providerView("one", cfg.Providers["one"])
	if view.Connection.Configured || view.Connection.Status != "disconnected" {
		t.Fatalf("connection = %+v", view.Connection)
	}
	if _, err := svc.ProviderModels("myfriend", "one"); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("disconnected models err = %v", err)
	}
}

func TestChatGPTCredentialValidationErrorsAreRedacted(t *testing.T) {
	setupConfigAgent(t, &config.Config{Providers: map[string]config.ProviderConfig{"subscription": {Type: chatgpt.ProviderType, AuthMode: "oauth", CredentialID: testChatGPTID}}})
	writeChatGPTCredential(t, time.Now().Add(time.Hour))
	credentialPath := filepath.Join(os.Getenv("HOME"), ".myaaw", "credentials", "chatgpt", "accounts", testChatGPTID+".json")
	if err := os.WriteFile(credentialPath, []byte(`{"access_token":"credential-error-secret","id":`), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := NewConfigService()
	for _, id := range []string{testChatGPTID, "credential-error-secret/invalid-id"} {
		_, createErr := svc.CreateProvider("myfriend", ProviderInput{Name: "bad", Type: chatgpt.ProviderType, CredentialID: id})
		_, patchErr := svc.UpdateProvider("myfriend", "subscription", ProviderPatch{CredentialID: &id})
		_, modelsErr := svc.ModelsForCredentials(ModelLookupInput{Type: chatgpt.ProviderType, CredentialID: id})
		_, connectErr := svc.StartChatGPT("myfriend", id)
		for _, err := range []error{createErr, patchErr, modelsErr, connectErr} {
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("credential validation err = %v", err)
			}
			for _, forbidden := range []string{"credential-error-secret", credentialPath, os.Getenv("HOME"), "access_token", "refresh_token"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("credential error disclosed private data: %v", err)
				}
			}
		}
	}
	view, err := svc.Providers("myfriend")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "credential-error-secret") || view.Providers[0].Connection.Status != "unavailable" {
		t.Fatal("unreadable credential exposed in provider status")
	}
}

func TestChatGPTConnectionLocalStates(t *testing.T) {
	setupConfigAgent(t, &config.Config{})
	p := config.ProviderConfig{Type: chatgpt.ProviderType, CredentialID: testChatGPTID}
	if got := providerView("test", p).Connection; got.Configured || got.Status != "unavailable" {
		t.Fatalf("missing = %+v", got)
	}
	writeChatGPTCredential(t, time.Now().Add(-time.Hour))
	if got := providerView("test", p).Connection; got.Status != "refresh_required" {
		t.Fatalf("expired = %+v", got)
	}
	path := filepath.Join(os.Getenv("HOME"), ".myaaw", "credentials", "chatgpt", "accounts", testChatGPTID+".json")
	data, _ := os.ReadFile(path)
	var c chatgpt.Credential
	_ = json.Unmarshal(data, &c)
	c.Scopes = []string{"openid"}
	data, _ = json.Marshal(c)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if got := providerView("test", p).Connection; got.Status != "reauthorization_required" {
		t.Fatalf("missing plan scopes = %+v", got)
	}
	c.AccessToken, c.RefreshToken = "", ""
	data, _ = json.Marshal(c)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if got := providerView("test", p).Connection; got.Status != "reauthorization_required" {
		t.Fatalf("revoked = %+v", got)
	}
}

func TestChatGPTSessionsScopedBoundedAndRedacted(t *testing.T) {
	setupConfigAgent(t, &config.Config{})
	if _, err := onboard.Initialize("other", false); err != nil {
		t.Fatal(err)
	}
	svc := NewConfigService()
	view, err := svc.StartChatGPT("myfriend", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = svc.ChatGPTStatus("myfriend", view.SessionID, true) })
	if view.AuthorizationURL == "" || !view.ExpiresAt.After(time.Now()) {
		t.Fatal("missing login metadata")
	}
	if _, err := svc.StartChatGPT("myfriend", ""); !errors.Is(err, ErrChatGPTSessionLimit) {
		t.Fatalf("parallel attempt err = %v", err)
	}
	if _, err := svc.ChatGPTStatus("other", view.SessionID, false); !errors.Is(err, ErrChatGPTSessionNotFound) {
		t.Fatal("cross-agent status allowed", err)
	}
	if _, err := svc.ChatGPTStatus("other", view.SessionID, true); !errors.Is(err, ErrChatGPTSessionNotFound) {
		t.Fatal("cross-agent cancellation allowed", err)
	}
	status, err := svc.ChatGPTStatus("myfriend", view.SessionID, false)
	if err != nil || status.Status != "pending" || status.CredentialID == "" {
		t.Fatalf("pending = %+v, %v", status, err)
	}
	body, _ := json.Marshal(status)
	for _, secret := range []string{"authorization_url", "code_challenge", "state", "token", "verifier"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("status exposed %s", secret)
		}
	}
	status, err = svc.ChatGPTStatus("myfriend", view.SessionID, true)
	if err != nil || status.Status != "failed" {
		t.Fatalf("cancel = %+v, %v", status, err)
	}
	// Expired retained sessions are unavailable even before the timer sweeps.
	svc.chatGPT.mu.Lock()
	svc.chatGPT.entries[view.SessionID] = chatGPTSession{agent: "myfriend", attempt: &chatgpt.Attempt{ExpiresAt: time.Now().Add(-3 * time.Minute)}}
	svc.chatGPT.mu.Unlock()
	if _, err := svc.ChatGPTStatus("myfriend", view.SessionID, false); !errors.Is(err, ErrChatGPTSessionNotFound) {
		t.Fatalf("expired session err = %v", err)
	}
}

func TestChatGPTSessionGlobalLimit(t *testing.T) {
	svc := NewConfigService()
	svc.chatGPT.entries = make(map[string]chatGPTSession)
	for i := 0; i < 32; i++ {
		id := string(rune('a' + i))
		svc.chatGPT.entries[id] = chatGPTSession{agent: "other", attempt: &chatgpt.Attempt{ExpiresAt: time.Now().Add(time.Minute)}}
	}
	if _, err := svc.StartChatGPT("new-agent", ""); !errors.Is(err, ErrChatGPTSessionLimit) {
		t.Fatalf("global session limit err = %v", err)
	}
}

func TestChatGPTPendingRegistrationRecovery(t *testing.T) {
	setupConfigAgent(t, &config.Config{})
	if _, err := chatgpt.DefaultManager(); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(os.Getenv("HOME"), ".myaaw", "credentials", "chatgpt")
	hostData, err := os.ReadFile(filepath.Join(root, "host.json"))
	if err != nil {
		t.Fatal(err)
	}
	var host struct {
		ID string `json:"ext_agent_host_id"`
	}
	if err := json.Unmarshal(hostData, &host); err != nil {
		t.Fatal(err)
	}
	registration := map[string]string{"id": testChatGPTID, "client_id": "oaiapp_recovery", "issuer": "https://auth.openai.com", "ext_agent_host_id": host.ID}
	data, _ := json.Marshal(registration)
	if err := os.WriteFile(filepath.Join(root, "registrations", testChatGPTID+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	svc := NewConfigService()
	if _, err := svc.CreateProvider("myfriend", ProviderInput{Name: "pending", Type: chatgpt.ProviderType, CredentialID: testChatGPTID}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal("pending registration activated", err)
	}
	view, err := svc.StartChatGPT("new-agent", testChatGPTID)
	if err != nil {
		t.Fatal("cannot resume pending registration", err)
	}
	defer func() { _, _ = svc.ChatGPTStatus("new-agent", view.SessionID, true) }()
	status, err := svc.ChatGPTStatus("new-agent", view.SessionID, false)
	if err != nil || status.CredentialID != testChatGPTID || status.Status != "pending" {
		t.Fatalf("recovery = %+v %v", status, err)
	}
	if !strings.Contains(view.AuthorizationURL, "client_id=oaiapp_recovery") {
		t.Fatal("recovery created a new dynamic client")
	}
	if _, err := loadAgentConfig("new-agent"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatal("browser login persisted an agent", err)
	}
}

func TestChatGPTModelsNeverInventUnauthenticatedFallback(t *testing.T) {
	setupConfigAgent(t, &config.Config{})
	view, err := NewConfigService().ModelsForCredentials(ModelLookupInput{Type: chatgpt.ProviderType})
	if !errors.Is(err, ErrInvalidConfig) || len(view.Models) != 0 {
		t.Fatalf("unauthenticated models = %+v, %v", view, err)
	}
}
