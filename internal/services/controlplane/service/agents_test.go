package service

import (
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

func TestAgentServiceCreateDoesNotInstallSkills(t *testing.T) {
	agents := setupAgentService(t)

	if _, err := agents.Create("agent-noskill"); err != nil {
		t.Fatalf("Create: %v", err)
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

func TestAgentServiceCreateAndDelete(t *testing.T) {
	agents := setupAgentService(t)

	info, err := agents.Create("myfriend")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Name != "myfriend" || info.Port == 0 || info.BaseURL == "" {
		t.Fatalf("agent = %+v", info)
	}
	if !agentdir.AgentInitialized("myfriend") {
		t.Fatal("agent should be initialized")
	}

	if _, err := agents.Create("myfriend"); !errors.Is(err, ErrAgentExists) {
		t.Fatalf("duplicate create err = %v, want ErrAgentExists", err)
	}
	if _, err := agents.Create("../bad"); !errors.Is(err, ErrInvalidAgentName) {
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

	if _, err := agents.Create("default"); err != nil {
		t.Fatalf("Create default: %v", err)
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
