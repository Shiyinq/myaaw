package service

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"myaaw/internal/agentdir"
)

func setupAgentService(t *testing.T) *AgentService {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	return NewAgentService()
}

func TestAgentServiceOnboardDoesNotInstallSkills(t *testing.T) {
	agents := setupAgentService(t)

	if _, err := agents.Onboard("agent-noskill", OnboardRequest{}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	skillsDir, err := agentdir.SkillsDir("agent-noskill")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("agent created by the control plane should start with no skills, found %d", len(entries))
	}
}

func TestAgentServiceOnboardAndDelete(t *testing.T) {
	agents := setupAgentService(t)

	result, err := agents.Onboard("myfriend", OnboardRequest{})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if !result.Created {
		t.Fatal("first onboarding should report created")
	}
	if !agentdir.AgentInitialized("myfriend") {
		t.Fatal("agent should be initialized")
	}

	info, err := agents.Get("myfriend")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if info.Name != "myfriend" || info.Port == 0 || info.BaseURL == "" {
		t.Fatalf("agent = %+v", info)
	}

	// Re-onboarding an existing agent reconfigures it instead of failing.
	again, err := agents.Onboard("myfriend", OnboardRequest{})
	if err != nil {
		t.Fatalf("second Onboard: %v", err)
	}
	if again.Created {
		t.Fatal("second onboarding should report reconfigured, not created")
	}
	if _, err := agents.Onboard("../bad", OnboardRequest{}); !errors.Is(err, ErrInvalidAgentName) {
		t.Fatalf("invalid name err = %v, want ErrInvalidAgentName", err)
	}

	if err := agents.SetDefault("myfriend"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	registry, err := agentdir.LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if registry.Default != "myfriend" {
		t.Fatalf("default = %q", registry.Default)
	}

	if err := agents.Delete("myfriend"); !errors.Is(err, ErrDefaultAgent) {
		t.Fatalf("delete default err = %v, want ErrDefaultAgent", err)
	}

	if _, err := agents.Onboard("default", OnboardRequest{}); err != nil {
		t.Fatalf("Onboard default: %v", err)
	}
	if err := agents.SetDefault("default"); err != nil {
		t.Fatalf("SetDefault default: %v", err)
	}
	if err := agents.Delete("myfriend"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if agentdir.AgentInitialized("myfriend") {
		t.Fatal("agent directory should be removed")
	}

	if err := agents.Delete("missing"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("err = %v, want ErrAgentNotFound", err)
	}
	if err := agents.Delete("default"); !errors.Is(err, ErrDefaultAgent) {
		t.Fatalf("err = %v, want ErrDefaultAgent", err)
	}
}

func TestAgentServiceDeleteRemovesContacts(t *testing.T) {
	agents := setupAgentService(t)

	if _, err := agents.Onboard("alpha", OnboardRequest{}); err != nil {
		t.Fatalf("Onboard alpha: %v", err)
	}
	if _, err := agents.Onboard("beta", OnboardRequest{}); err != nil {
		t.Fatalf("Onboard beta: %v", err)
	}

	alphaPath, err := agentdir.ContactsPath("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !contactsContain(t, alphaPath, "beta") {
		t.Fatal("alpha should reference beta before the delete")
	}

	if err := agents.Delete("beta"); err != nil {
		t.Fatalf("Delete beta: %v", err)
	}
	if contactsContain(t, alphaPath, "beta") {
		t.Fatal("alpha still references beta after the delete")
	}
}

func contactsContain(t *testing.T, path, name string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var entries []map[string]any
	if err := json.Unmarshal(data, &entries); err != nil {
		return false
	}
	for _, entry := range entries {
		if entry["name"] == name {
			return true
		}
	}
	return false
}

func TestCreatedAtRFC3339NormalizesDates(t *testing.T) {
	registry := &agentdir.Registry{Agents: map[string]agentdir.AgentInfo{
		"old": {CreatedAt: "2026-01-02"},
		"new": {CreatedAt: "2026-01-02T03:04:05+07:00"},
		"utc": {CreatedAt: "2026-01-02T03:04:05Z"},
	}}

	cases := map[string]string{
		"old": "2026-01-02T00:00:00Z",
		"new": "2026-01-01T20:04:05Z",
		"utc": "2026-01-02T03:04:05Z",
	}
	for name, want := range cases {
		if got := createdAtRFC3339(registry, name); got != want {
			t.Errorf("createdAtRFC3339(%q) = %q, want %q", name, got, want)
		}
	}
}
