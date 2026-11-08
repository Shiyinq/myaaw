package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/auth/chatgpt"
	"myaaw/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

const cliChatGPTID = "33333333-3333-4333-8333-333333333333"

func setupCLIChatGPT(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("MYAAW_API_KEY", "unrelated-api-key-from-env")
	if _, err := initializeAgent("subscription-test"); err != nil {
		t.Fatal(err)
	}
	if _, err := chatgpt.DefaultManager(); err != nil {
		t.Fatal(err)
	}
	c := chatgpt.Credential{ID: cliChatGPTID, ClientID: "oaiapp_cli", Subject: "test-subject", Email: "cli@example.test",
		AccessToken: "cli-secret-access", RefreshToken: "cli-secret-refresh", IDToken: "cli-secret-id", ExpiresAt: time.Now().Add(time.Hour),
		Scopes: []string{"openid", "offline_access", "resource.invoke", "chatgpt.tokens.use.direct"}}
	data, _ := json.Marshal(c)
	path := filepath.Join(home, ".myaaw", "credentials", "chatgpt", "accounts", cliChatGPTID+".json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestConnectChatGPTHeadlessReferenceAndStatus(t *testing.T) {
	setupCLIChatGPT(t)
	originalBrowser := openChatGPTBrowser
	openChatGPTBrowser = func(string) error { t.Fatal("headless attachment opened a browser"); return nil }
	t.Cleanup(func() { openChatGPTBrowser = originalBrowser })
	cmd := newConnectChatGPTCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	if err := cmd.Flags().Parse([]string{"--credential-id", cliChatGPTID, "--model", "live-model", "--default"}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, []string{"subscription"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadJSONConfigOnly()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Providers["subscription"]
	if p.Type != chatgpt.ProviderType || p.AuthMode != "oauth" || p.CredentialID != cliChatGPTID || p.DefaultModel != "live-model" || p.APIKey != "" || cfg.DefaultProvider != "subscription" {
		t.Fatalf("saved reference = %+v", p)
	}
	status := newChatGPTStatusCmd()
	status.SetOut(&output)
	if err := status.RunE(status, []string{"subscription"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "cli@example.test") {
		t.Fatal("status omitted account label")
	}
	for _, secret := range []string{"cli-secret", "access_token", "refresh_token", "id_token", "oaiapp_cli"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("CLI exposed %q", secret)
		}
	}
	disconnect := newDisconnectChatGPTCmd()
	disconnect.SetOut(&output)
	if err := disconnect.RunE(disconnect, []string{"subscription"}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = config.LoadJSONConfigOnly()
	if cfg.Providers["subscription"].CredentialID != "" {
		t.Fatal("disconnect kept provider reference")
	}
	manager, _ := chatgpt.DefaultManager()
	if _, err := manager.Get(cliChatGPTID); err != nil {
		t.Fatal("disconnect deleted shared credential", err)
	}
}

func TestChatGPTHeadlessOnboardingNoAPIKey(t *testing.T) {
	setupCLIChatGPT(t)
	in, err := headlessInput("new-agent", onboardOptions{provider: chatgpt.ProviderType, credentialID: cliChatGPTID, model: "live-model"})
	if err != nil {
		t.Fatal(err)
	}
	if in.ProviderCredentialID != cliChatGPTID || in.ProviderAuthMode != "oauth" || in.ProviderAPIKey != "" || in.ProviderBaseURL != "" {
		t.Fatalf("input = %+v", in)
	}
	for _, opts := range []onboardOptions{
		{provider: chatgpt.ProviderType, model: "live-model"},
		{provider: chatgpt.ProviderType, credentialID: cliChatGPTID},
		{provider: chatgpt.ProviderType, credentialID: cliChatGPTID, model: "live-model", apiKey: "sk-no"},
		{provider: chatgpt.ProviderType, credentialID: cliChatGPTID, model: "live-model", baseURL: "https://evil.example"},
	} {
		if _, err := headlessInput("new-agent", opts); err == nil {
			t.Fatal("invalid headless OAuth input accepted")
		}
	}
}

func TestChatGPTWizardSkipsKeyAndDoesNotInventModels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newOnboardingModel("new-agent", true)
	m.cursor = indexOf(m.choices, chatgpt.Label)
	if m.cursor < 0 {
		t.Fatal("ChatGPT Subscription missing from choices")
	}
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.providerType != chatgpt.ProviderType || m.step != stepProviderName {
		t.Fatal("wrong provider selection")
	}
	next, cmd := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	defer m.chatGPTCancel()
	if cmd == nil || m.step != stepProviderModel || m.llmKey != "" || m.providerBaseURL != "" || !m.modelsLoading {
		t.Fatal("OAuth wizard asked for API key or URL")
	}
	// Do not execute the browser command; drive its safe result as the TUI does.
	updated, _ := m.Update(chatGPTSignedInMsg{credentialID: cliChatGPTID, err: errors.New("sign-in failed")})
	m = updated.(onboardModel)
	if m.modelManual || m.modelsLoading || m.credentialID != cliChatGPTID {
		t.Fatal("failed registration recovery was lost")
	}
	m = m.handleModels(modelsFetchedMsg{err: errors.New("offline")})
	if m.modelManual || len(m.models) != 0 {
		t.Fatal("OAuth offered unauthenticated manual fallback")
	}
	m = m.handleModels(modelsFetchedMsg{models: []string{"real-model"}})
	if len(m.choices) != 1 || m.choices[0] != "real-model" {
		t.Fatalf("choices = %v", m.choices)
	}
	m.step = stepSkills
	m = m.previousStep()
	if m.step != stepProviderModel || m.modelManual || len(m.choices) != 1 || m.choices[0] != "real-model" {
		t.Fatal("back navigation introduced a manual OAuth model fallback")
	}
}

func TestChatGPTBrowserCommandUsesArgv(t *testing.T) {
	url := "https://auth.openai.com/authorize?state=abc&hint='; echo secret"
	for _, goos := range []string{"darwin", "windows", "linux"} {
		cmd := browserCommand(goos, url)
		if cmd.Args[len(cmd.Args)-1] != url {
			t.Fatal("authorization URL was split or interpreted")
		}
		for _, arg := range cmd.Args[:len(cmd.Args)-1] {
			if arg == "sh" || arg == "-c" || arg == "cmd" || arg == "/c" {
				t.Fatal("browser launch used shell")
			}
		}
	}
}

func TestChatGPTSignInFailureDoesNotExposeURL(t *testing.T) {
	setupCLIChatGPT(t)
	original := openChatGPTBrowser
	openChatGPTBrowser = func(string) error { return errors.New("cannot open system browser") }
	t.Cleanup(func() { openChatGPTBrowser = original })
	id, err := signInChatGPT(context.Background(), "")
	if err == nil || id == "" {
		t.Fatal("failed attempt did not return recovery ID")
	}
	if strings.Contains(err.Error(), "authorize") || strings.Contains(err.Error(), "state=") {
		t.Fatal("error exposed OAuth URL")
	}
	if !agentdir.AgentInitialized("subscription-test") {
		t.Fatal("test agent missing")
	}
}
