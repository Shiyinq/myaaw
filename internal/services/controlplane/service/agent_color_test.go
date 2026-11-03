package service

import (
	"errors"
	"testing"

	"myaaw/internal/agentdir"
)

func iconColorPtr(value string) *string {
	return &value
}

func TestOnboardStoresAgentColor(t *testing.T) {
	agents := setupAgentService(t)

	if _, err := agents.Onboard("painted", OnboardRequest{AgentColor: iconColorPtr("brun")}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}

	registry, err := agentdir.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if got := registry.Agents["painted"].AgentColor; got != "brun" {
		t.Fatalf("registry colour = %q, want brun", got)
	}

	info, err := agents.Get("painted")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if info.AgentColor != "brun" {
		t.Fatalf("Get colour = %q, want brun", info.AgentColor)
	}

	list, err := agents.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, agent := range list {
		if agent.Name != "painted" {
			continue
		}
		found = true
		if agent.AgentColor != "brun" {
			t.Fatalf("List colour = %q, want brun", agent.AgentColor)
		}
	}
	if !found {
		t.Fatal("painted agent missing from List")
	}
}

// Omitting the field must not touch what is already stored, and an agent that
// never had one keeps an empty registry value while the API resolves it.
func TestOnboardWithoutAgentColorKeepsLegacyEntry(t *testing.T) {
	agents := setupAgentService(t)

	if _, err := agents.Onboard("plain", OnboardRequest{}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}

	registry, err := agentdir.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if got := registry.Agents["plain"].AgentColor; got != "" {
		t.Fatalf("registry should stay untouched, found %q", got)
	}

	info, err := agents.Get("plain")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if info.AgentColor != agentdir.DefaultAgentColor {
		t.Fatalf("resolved colour = %q, want %q", info.AgentColor, agentdir.DefaultAgentColor)
	}
}

func TestOnboardRejectsUnknownAgentColor(t *testing.T) {
	agents := setupAgentService(t)

	_, err := agents.Onboard("typo", OnboardRequest{AgentColor: iconColorPtr("mauve")})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("want ErrInvalidConfig, got %v", err)
	}
	// The colour is checked before Initialize, so no agent is left behind.
	if agentdir.AgentInitialized("typo") {
		t.Fatal("a rejected colour must not create the agent")
	}
}

func TestOnboardKeepsAgentColorOnReconfigure(t *testing.T) {
	agents := setupAgentService(t)

	if _, err := agents.Onboard("painted", OnboardRequest{AgentColor: iconColorPtr("rose")}); err != nil {
		t.Fatalf("first Onboard: %v", err)
	}
	if _, err := agents.Onboard("painted", OnboardRequest{}); err != nil {
		t.Fatalf("second Onboard: %v", err)
	}

	info, err := agents.Get("painted")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if info.AgentColor != "rose" {
		t.Fatalf("colour after reconfigure = %q, want rose", info.AgentColor)
	}
}
