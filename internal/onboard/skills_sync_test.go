package onboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/agentdir"
)

func setupSkillsHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	if err := agentdir.RegisterAgent("agent-x"); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
}

func readSkillsJSON(t *testing.T, agent string) map[string]bool {
	t.Helper()
	dir, err := agentdir.SkillsDir(agent)
	if err != nil {
		t.Fatalf("SkillsDir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "skills.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read skills.json: %v", err)
	}
	var filter map[string]bool
	if err := json.Unmarshal(data, &filter); err != nil {
		t.Fatalf("parse skills.json: %v", err)
	}
	return filter
}

func TestInstallSkillsRecordsEnabledFlag(t *testing.T) {
	setupSkillsHome(t)
	names := BuiltinSkillNames()
	if len(names) == 0 {
		t.Skip("no built-in skills embedded")
	}
	name := names[0]

	if err := InstallSkills("agent-x", []string{name}); err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}

	dir, err := agentdir.SkillsDir("agent-x")
	if err != nil {
		t.Fatalf("SkillsDir: %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, name)); err != nil || !info.IsDir() {
		t.Fatalf("skill folder missing: %v", err)
	}
	filter := readSkillsJSON(t, "agent-x")
	if enabled, ok := filter[name]; !ok || !enabled {
		t.Fatalf("skills.json = %+v, want %q enabled", filter, name)
	}
}

func TestSyncSkillsDisablesUnselectedWithoutDeleting(t *testing.T) {
	setupSkillsHome(t)
	names := BuiltinSkillNames()
	if len(names) < 2 {
		t.Skip("need at least two built-in skills")
	}
	first, second := names[0], names[1]

	if err := InstallSkills("agent-x", []string{first, second}); err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}
	if err := SyncSkills("agent-x", []string{first}); err != nil {
		t.Fatalf("SyncSkills: %v", err)
	}

	filter := readSkillsJSON(t, "agent-x")
	if enabled, ok := filter[first]; !ok || !enabled {
		t.Fatalf("skills.json = %+v, want %q enabled", filter, first)
	}
	if enabled, ok := filter[second]; !ok || enabled {
		t.Fatalf("skills.json = %+v, want %q disabled", filter, second)
	}

	dir, err := agentdir.SkillsDir("agent-x")
	if err != nil {
		t.Fatalf("SkillsDir: %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, second)); err != nil || !info.IsDir() {
		t.Fatalf("unselected skill folder must survive: %v", err)
	}
}

func TestSyncSkillsLeavesUserSkillsAlone(t *testing.T) {
	setupSkillsHome(t)
	dir, err := agentdir.SkillsDir("agent-x")
	if err != nil {
		t.Fatalf("SkillsDir: %v", err)
	}
	custom := filepath.Join(dir, "my-custom-skill")
	if err := os.MkdirAll(custom, 0o755); err != nil {
		t.Fatalf("mkdir custom skill: %v", err)
	}

	if err := SyncSkills("agent-x", nil); err != nil {
		t.Fatalf("SyncSkills: %v", err)
	}

	if _, ok := readSkillsJSON(t, "agent-x")["my-custom-skill"]; ok {
		t.Fatal("user-authored skill must not be touched")
	}
	if info, err := os.Stat(custom); err != nil || !info.IsDir() {
		t.Fatalf("user-authored skill folder must survive: %v", err)
	}
}
