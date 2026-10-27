package main

import (
	"errors"
	"testing"

	"myaaw/internal/agentdir"

	tea "github.com/charmbracelet/bubbletea"
)

// TestOnboardFlow drives the wizard without the TUI: first run asks for the
// agent name, then the answers are written to .env and config.json.
func TestOnboardFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	enter := func(m onboardModel) onboardModel {
		t.Helper()
		next, cmd := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
		if m.step != stepProviderKey && cmd != nil {
			t.Fatalf("unexpected async command at step %d", m.step)
		}
		return next.(onboardModel)
	}

	m := initialOnboardModel("")

	// Welcome -> agent name prompt (fresh install).
	m = enter(m)
	if m.step != stepAgentName {
		t.Fatalf("step = %d, want stepAgentName", m.step)
	}
	m.textInput.SetValue("agent-b")
	m = enter(m)
	if m.step != stepInitHome {
		t.Fatalf("step = %d, want stepInitHome", m.step)
	}
	if m.myaawHome == "" || m.agentName != "agent-b" {
		t.Fatalf("agent not resolved: name=%q home=%q", m.agentName, m.myaawHome)
	}
	m = enter(m)
	if m.step != stepProviderType {
		t.Fatalf("step = %d, want stepProviderType", m.step)
	}

	// Provider type -> name -> base URL -> API key.
	m = enter(m)
	if m.step != stepProviderName {
		t.Fatalf("step = %d, want stepProviderName", m.step)
	}
	m.textInput.SetValue("deepseek")
	m = enter(m)
	if m.providerBaseURL != "https://api.openai.com" {
		t.Fatalf("base URL default = %q", m.providerBaseURL)
	}
	m.textInput.SetValue("https://api.deepseek.com/")
	m = enter(m)
	if m.providerBaseURL != "https://api.deepseek.com" {
		t.Fatalf("base URL = %q, want trailing slash trimmed", m.providerBaseURL)
	}
	m.textInput.SetValue("sk-test")
	next, cmd := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a model fetch command")
	}
	m = next.(onboardModel)

	// The fetch failed; the wizard must fall back to manual input.
	m = m.handleModels(modelsFetchedMsg{err: errors.New("offline")})
	if !m.modelManual {
		t.Fatal("expected manual model input after a failed fetch")
	}
	m.textInput.SetValue("deepseek-chat")
	m = enter(m)
	if m.step != stepChannelChoice {
		t.Fatalf("step = %d, want stepChannelChoice", m.step)
	}

	// Skip channels -> skip heartbeat -> owner.
	m.cursor = len(m.choices) - 1
	m = enter(m)
	if m.step != stepHeartbeatChannel {
		t.Fatalf("step = %d, want stepHeartbeatChannel", m.step)
	}
	m.cursor = len(m.choices) - 1
	m = enter(m)
	if m.step != stepOwner {
		t.Fatalf("step = %d, want stepOwner", m.step)
	}
	m.textInput.SetValue("42, -100200")
	m = enter(m)
	if m.step != stepSummary {
		t.Fatalf("step = %d, want stepSummary", m.step)
	}

	// Save.
	m.cursor = 0
	m = enter(m)
	if m.err != nil {
		t.Fatalf("persist failed: %v", m.err)
	}
	if m.step != stepInstallGlobal {
		t.Fatalf("step = %d, want stepInstallGlobal", m.step)
	}

	cfg, err := loadConfigFromFile(m.configFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultProvider != "deepseek" {
		t.Fatalf("default provider = %q", cfg.DefaultProvider)
	}
	p := cfg.Providers["deepseek"]
	if p.Type != "openai" || p.BaseURL != "https://api.deepseek.com" || p.APIKey != "sk-test" || p.DefaultModel != "deepseek-chat" {
		t.Fatalf("unexpected provider: %+v", p)
	}
	if cfg.Channels.Telegram != nil || cfg.Channels.Discord != nil {
		t.Fatal("skipped channels should stay unset on a fresh agent")
	}
	if cfg.Heartbeat.Active {
		t.Fatal("skipped heartbeat should stay disabled")
	}
	if len(cfg.Bot.OwnerIDs) != 2 || cfg.Bot.OwnerIDs[0] != "42" || cfg.Bot.OwnerIDs[1] != "-100200" {
		t.Fatalf("owner IDs = %v", cfg.Bot.OwnerIDs)
	}

	if port := readEnvValue(m.envFile, "PORT"); port != "8081" {
		t.Fatalf("PORT = %q, want 8081", port)
	}
	if base := readEnvValue(m.envFile, "MYAAW_BASE_URL"); base != "http://localhost:8081" {
		t.Fatalf("MYAAW_BASE_URL = %q", base)
	}

	registry, err := agentdir.LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if registry.Default != "agent-b" {
		t.Fatalf("registry default = %q, want agent-b", registry.Default)
	}
}
