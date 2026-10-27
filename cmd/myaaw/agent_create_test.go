package main

import (
	"os"
	"testing"

	"myaaw/internal/agentdir"
)

// agent create must reuse the onboarding implementation: same layout, .env,
// A2A token, registry entry, and port rules.
func TestNewOnboardingModelCreatesAgent(t *testing.T) {
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
	if !agentdir.AgentInitialized("agent-new") {
		t.Fatal("agent layout was not created")
	}

	envPath, err := agentdir.EnvPath("agent-new")
	if err != nil {
		t.Fatal(err)
	}
	if port := readEnvValue(envPath, "PORT"); port != "8080" {
		t.Fatalf("PORT = %q, want 8080", port)
	}
	if token := readEnvValue(envPath, "A2A_TOKEN"); token == "" {
		t.Fatal("A2A token was not generated")
	}
	contactsPath, err := agentdir.ContactsPath("agent-new")
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(contactsPath); statErr != nil {
		t.Fatalf("contacts.json was not created: %v", statErr)
	}

	registry, err := agentdir.LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Agents["agent-new"]; !ok {
		t.Fatal("agent was not registered")
	}
	if registry.Default != "agent-new" {
		t.Fatalf("registry default = %q, want agent-new", registry.Default)
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
