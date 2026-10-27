package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/agentdir"

	tea "github.com/charmbracelet/bubbletea"
)

// agent create must reuse the onboarding implementation but must not touch the
// filesystem before the review step is saved.
func TestNewOnboardingModelStartsAtProviderWithoutWriting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	m := newOnboardingModel("agent-new", true)
	if m.err != nil {
		t.Fatalf("unexpected error: %v", m.err)
	}
	if m.step != stepProviderType {
		t.Fatalf("step = %d, want stepProviderType", m.step)
	}

	root, err := agentdir.AgentRoot("agent-new")
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("agent directory exists before saving: %v", statErr)
	}
	registry, err := agentdir.LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Agents["agent-new"]; ok {
		t.Fatal("agent was registered before saving")
	}
}

func TestOnboardCancelLeavesNothing(t *testing.T) {
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

	// Walk all the way to the review step without saving, then "cancel".
	m := newOnboardingModel("agent-cancel", true)
	m = enter(m) // provider type
	m = enter(m) // provider name
	m = enter(m) // base URL
	m.textInput.SetValue("sk-test")
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	m = m.handleModels(modelsFetchedMsg{err: errors.New("offline")})
	m.textInput.SetValue("gpt-4o")
	m = enter(m) // model -> skills
	m = enter(m) // skills (none) -> tools
	m = enter(m) // tools (all) -> channels
	m.cursor = len(m.choices) - 1
	m = enter(m) // skip channels
	m.cursor = len(m.choices) - 1
	m = enter(m) // skip heartbeat -> review (no owner prompt without a channel)
	if m.step != stepSummary {
		t.Fatalf("step = %d, want stepSummary", m.step)
	}

	// Cancel: no persist() call.
	root, err := agentdir.AgentRoot("agent-cancel")
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("cancelled onboarding left a directory: %v", statErr)
	}
	registry, err := agentdir.LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Agents["agent-cancel"]; ok {
		t.Fatal("cancelled onboarding registered the agent")
	}
}

func TestUpdateCancelKeepsExistingConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MYAAW_API_KEY", "sk-original")
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	defer func() { onboardOpts = onboardOptions{} }()
	onboardOpts = onboardOptions{yes: true, provider: "openai", model: "gpt-4o", channel: "skip", owner: "42"}
	if code := runOnboardHeadless("agent-update"); code != 0 {
		t.Fatalf("setup exit code = %d", code)
	}

	root, err := agentdir.AgentRoot("agent-update")
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config", "config.json")
	envPath := filepath.Join(root, ".env")
	beforeConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeEnv, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}

	// Reconfigure in the TUI, change everything, then cancel at Review.
	m := newOnboardingModel("agent-update", true)
	enter := func(m onboardModel) onboardModel {
		t.Helper()
		next, cmd := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
		if m.step != stepProviderKey && cmd != nil {
			t.Fatalf("unexpected async command at step %d", m.step)
		}
		return next.(onboardModel)
	}

	m.cursor = 1
	m = enter(m) // provider type -> name
	m = enter(m) // name -> base URL
	m = enter(m) // base URL -> API key
	m.textInput.SetValue("sk-changed")
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	m = m.handleModels(modelsFetchedMsg{err: errors.New("offline")})
	m.textInput.SetValue("gemini-2.5-flash")
	m = enter(m) // model -> skills
	m = enter(m) // skills -> tools
	m = enter(m) // tools -> channels
	m.cursor = len(m.choices) - 1
	m = enter(m) // skip channels
	m.cursor = len(m.choices) - 1
	m = enter(m) // skip heartbeat -> review (no owner prompt without a channel)
	if m.step != stepSummary {
		t.Fatalf("step = %d, want stepSummary", m.step)
	}

	// Cancel instead of saving.
	afterConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	afterEnv, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeConfig) != string(afterConfig) {
		t.Fatal("config.json changed even though the wizard was cancelled")
	}
	if string(beforeEnv) != string(afterEnv) {
		t.Fatal(".env changed even though the wizard was cancelled")
	}
}

func TestNewOnboardingModelKeepsOrderForOnboard(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	// `myaaw onboard` (createNow=false) keeps the normal step order.
	m := newOnboardingModel("agent-new", false)
	if m.step != stepWelcome {
		t.Fatalf("step = %d, want stepWelcome", m.step)
	}
	if agentdir.AgentInitialized("agent-new") {
		t.Fatal("onboard must not create the agent before the wizard starts")
	}
}
