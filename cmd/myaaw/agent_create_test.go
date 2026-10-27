package main

import (
	"errors"
	"os"
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
	m = enter(m) // model -> channels
	m.cursor = len(m.choices) - 1
	m = enter(m) // skip channels
	m.cursor = len(m.choices) - 1
	m = enter(m) // skip heartbeat
	m = enter(m) // owner (empty) -> review
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
