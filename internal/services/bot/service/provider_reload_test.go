package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myaaw/internal/auth/chatgpt"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
)

func setRuntimeProvider(cfg config.ProviderConfig) {
	config.LLMProviderName, config.LLMProviderBaseURL = cfg.Type, cfg.BaseURL
	config.LLMProviderAPIKey, config.LLMDefaultModel = cfg.APIKey, cfg.DefaultModel
	config.LLMProviderAuthMode, config.LLMProviderCredentialID = cfg.AuthMode, cfg.CredentialID
}

func preserveRuntimeProvider(t *testing.T) {
	t.Helper()
	previous, id := selectedProviderConfig(), config.CurrentProviderID
	t.Cleanup(func() { setRuntimeProvider(previous); config.CurrentProviderID = id })
}

type runtimeKeyProvider struct {
	provider.LLMProvider
	base, key, model string
}

func (*runtimeKeyProvider) ProviderName() string { return "openai" }
func (p *runtimeKeyProvider) DefaultModel(model string) string {
	if model == "" {
		return p.model
	}
	return model
}
func (p *runtimeKeyProvider) Chat(string, []provider.Message, []map[string]interface{}) (provider.Message, error) {
	return provider.Message{Role: "assistant", Content: p.key}, nil
}

func TestLegacyAPIKeyRuntimeReloadsEffectiveConfig(t *testing.T) {
	preserveRuntimeProvider(t)
	factory := provider.LLMproviderFactories["openai"]
	t.Cleanup(func() { provider.LLMproviderFactories["openai"] = factory })
	provider.LLMproviderFactories["openai"] = func(base, key, model string) provider.LLMProvider {
		return &runtimeKeyProvider{base: base, key: key, model: model}
	}
	// Environment-only selection must reload too (there may be no provider ID).
	config.CurrentProviderID = ""
	cfg := config.ProviderConfig{Type: "openai", BaseURL: "https://legacy.invalid", APIKey: "api-key-1", DefaultModel: "legacy-model"}
	setRuntimeProvider(cfg)
	svc := NewBotService(nil, nil).(*BotServiceImpl)
	assertKey := func(want string) {
		t.Helper()
		if err := svc.requireProvider(); err != nil {
			t.Fatal(err)
		}
		message, err := svc.llmProvider.Chat("", nil, nil)
		if err != nil || provider.MessageText(message) != want {
			t.Fatalf("legacy runtime: %v / %q", err, provider.MessageText(message))
		}
	}
	assertKey("api-key-1")
	before := svc.llmProvider
	svc.checkAndReloadProvider()
	if svc.llmProvider != before {
		t.Fatal("unchanged config recreated provider")
	}
	cfg.APIKey = "api-key-2"
	setRuntimeProvider(cfg)
	svc.checkAndReloadProvider()
	assertKey("api-key-2")
	cfg.BaseURL = "https://other.invalid"
	setRuntimeProvider(cfg)
	svc.checkAndReloadProvider()
	if svc.llmProvider.(*runtimeKeyProvider).base != cfg.BaseURL {
		t.Fatal("endpoint change ignored")
	}
	cfg.DefaultModel = "another-model"
	setRuntimeProvider(cfg)
	svc.checkAndReloadProvider()
	if svc.llmProvider.DefaultModel("") != cfg.DefaultModel {
		t.Fatal("model change ignored")
	}
	cfg.AuthMode = "oauth"
	setRuntimeProvider(cfg)
	svc.checkAndReloadProvider()
	if svc.requireProvider() == nil || svc.llmProvider != nil || svc.agent != nil {
		t.Fatal("invalid auth-mode switch retained API-key client")
	}
}

type providerSwitchUserRepo struct {
	user model.User
}

func (r *providerSwitchUserRepo) CreateUser(*model.User) (*model.User, error) {
	return nil, fmt.Errorf("unexpected user creation")
}
func (r *providerSwitchUserRepo) GetUserById(int) (*model.User, error) {
	user := r.user
	return &user, nil
}
func (r *providerSwitchUserRepo) UpdateModel(_ int, name string) error {
	r.user.Model = name
	return nil
}
func (r *providerSwitchUserRepo) UpdateProvider(_ int, name string) error {
	r.user.Provider = name
	return nil
}

func TestTelegramProviderSwitchSynchronizesStoredUserModel(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			preserveRuntimeProvider(t)
			config.CurrentProviderID = "subscription"
			setRuntimeProvider(config.ProviderConfig{
				Type: "openai_chatgpt", AuthMode: "oauth", CredentialID: "selected-account",
				DefaultModel: "selected-gpt-model",
			})
			repo := &providerSwitchUserRepo{user: model.User{
				UserId: 42, Name: "Telegram user", Provider: "openai", Model: "deepseek-v4-flash",
			}}
			svc := NewBotService(repo, nil).(*BotServiceImpl)
			// /me follows the normal user/provider synchronization path without
			// contacting OpenAI or changing a real Telegram conversation.
			msg := &channel.IncomingMessage{UserID: 42, Channel: "telegram", ChatID: "42", Text: "/me"}
			var err error
			if stream {
				_, err = svc.BotStreamContext(context.Background(), msg, nil)
			} else {
				_, err = svc.BotContext(context.Background(), msg)
			}
			if err != nil {
				t.Fatal(err)
			}
			if repo.user.Provider != "openai_chatgpt" || repo.user.Model != "selected-gpt-model" {
				t.Fatalf("Telegram retained previous provider/model: %s / %s", repo.user.Provider, repo.user.Model)
			}
		})
	}
}

type runtimeTransport func(*http.Request) (*http.Response, error)

func (f runtimeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func seedRuntimeCredential(t *testing.T, home, id, token string) {
	t.Helper()
	dir := filepath.Join(home, ".myaaw", "credentials", "chatgpt", "accounts")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(chatgpt.Credential{
		ID: id, ClientID: "oaiapp_runtime", Subject: "test-subject", AccessToken: token,
		RefreshToken: "unused-refresh", ExpiresAt: time.Now().Add(time.Hour),
		Scopes: []string{"openid", "offline_access", "resource.invoke", "chatgpt.tokens.use.direct"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestOAuthRuntimeReconnectDisconnectAndTypeSwitch(t *testing.T) {
	preserveRuntimeProvider(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	const firstID = "11111111-1111-4111-8111-111111111111"
	const secondID = "22222222-2222-4222-8222-222222222222"
	seedRuntimeCredential(t, home, firstID, "first-token")
	seedRuntimeCredential(t, home, secondID, "second-token")
	oldTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	var tokens []string
	http.DefaultTransport = runtimeTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.openai.com/v1/responses" {
			return nil, fmt.Errorf("unexpected request: %s", r.URL)
		}
		tokens = append(tokens, r.Header.Get("Authorization"))
		body := `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}]}}` + "\n\n"
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	config.CurrentProviderID = "same-provider"
	cfg := config.ProviderConfig{Type: "openai_chatgpt", AuthMode: "oauth", CredentialID: firstID, DefaultModel: "account-model"}
	setRuntimeProvider(cfg)
	svc := NewBotService(nil, nil).(*BotServiceImpl)
	chat := func(wantToken string) {
		t.Helper()
		if err := svc.requireProvider(); err != nil {
			t.Fatal(err)
		}
		_, err := svc.llmProvider.Chat("", []provider.Message{{Role: "user", Content: "hello"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if tokens[len(tokens)-1] != "Bearer "+wantToken {
			t.Fatalf("wrong selected credential: %q", tokens[len(tokens)-1])
		}
	}
	chat("first-token")
	cfg.CredentialID = secondID
	setRuntimeProvider(cfg)
	svc.checkAndReloadProvider()
	chat("second-token")
	// Reauthorization may rotate the stored token without changing the reference.
	seedRuntimeCredential(t, home, secondID, "reconnected-token")
	svc.checkAndReloadProvider()
	chat("reconnected-token")
	cfg.CredentialID = ""
	setRuntimeProvider(cfg)
	svc.checkAndReloadProvider()
	if svc.requireProvider() == nil || svc.llmProvider != nil || svc.agent != nil {
		t.Fatal("disconnect retained the previous OAuth client")
	}
	requests := len(tokens)
	if _, err := svc.BotContext(context.Background(), &channel.IncomingMessage{}); err == nil {
		t.Fatal("disconnected BotContext accepted a turn")
	}
	if _, err := svc.BotStreamContext(context.Background(), &channel.IncomingMessage{}, nil); err == nil {
		t.Fatal("disconnected stream accepted a turn")
	}
	if _, _, err := svc.ProcessHeartbeat("hello", HeartbeatTarget{}, "cron"); err == nil {
		t.Fatal("disconnected heartbeat accepted a turn")
	}
	if len(tokens) != requests {
		t.Fatal("disconnected request reached transport")
	}
	cfg.CredentialID = firstID
	setRuntimeProvider(cfg)
	svc.checkAndReloadProvider()
	chat("first-token")
	// Same ID and model, different billing/auth path: no OAuth fallback or reuse.
	cfg.Type, cfg.AuthMode, cfg.CredentialID, cfg.APIKey = "openai", "", "", "legacy-api-key"
	setRuntimeProvider(cfg)
	svc.checkAndReloadProvider()
	if err := svc.requireProvider(); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.llmProvider.(*provider.OpenAIProvider); !ok {
		t.Fatalf("type switch retained %T", svc.llmProvider)
	}
	cfg.Type, cfg.AuthMode, cfg.APIKey = "openai_chatgpt", "oauth", ""
	setRuntimeProvider(cfg)
	svc.checkAndReloadProvider()
	if svc.requireProvider() == nil || svc.llmProvider != nil {
		t.Fatal("switch to disconnected OAuth retained API-key client")
	}
	cfg.CredentialID = secondID
	setRuntimeProvider(cfg)
	svc.checkAndReloadProvider()
	chat("reconnected-token")
}
