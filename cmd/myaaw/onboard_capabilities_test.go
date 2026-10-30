package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/agentdir"
)

func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}

func TestCapabilityDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := initialOnboardModel("")
	if len(m.skills) == 0 {
		t.Fatal("no built-in skills found")
	}
	if len(m.tools) == 0 {
		t.Fatal("no built-in tools found")
	}
	for name, selected := range m.skillsSelected {
		if selected {
			t.Fatalf("skill %q should be unselected by default", name)
		}
	}
	for name, enabled := range m.toolsSelected {
		if !enabled {
			t.Fatalf("tool %q should be enabled by default", name)
		}
	}
}

func TestToggleCapabilities(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := initialOnboardModel("")
	if indexOf(m.skills, "time") < 0 {
		t.Fatalf("expected the time skill, got %v", m.skills)
	}

	m.step = stepSkills
	m.choices = append([]string{}, m.skills...)
	m.cursor = indexOf(m.choices, "time")
	m.toggleCurrent()
	if got := m.selectedSkills(); len(got) != 1 || got[0] != "time" {
		t.Fatalf("selected skills = %v, want [time]", got)
	}

	m.step = stepTools
	m.choices = append([]string{}, m.tools...)
	m.cursor = indexOf(m.choices, "bash")
	m.toggleCurrent()
	if got := m.disabledTools(); len(got) != 1 || got[0] != "bash" {
		t.Fatalf("disabled tools = %v, want [bash]", got)
	}
}

func TestInstallSelectedSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := installSelectedSkills("agent-x", []string{"time"}); err != nil {
		t.Fatalf("installSelectedSkills: %v", err)
	}
	skillsDir, err := agentdir.SkillsDir("agent-x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "time", "SKILL.md")); err != nil {
		t.Fatalf("time skill was not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "weather")); !os.IsNotExist(err) {
		t.Fatalf("unselected skill was created: %v", err)
	}

	// Running again is safe and never errors.
	if err := installSelectedSkills("agent-x", []string{"time"}); err != nil {
		t.Fatalf("second install: %v", err)
	}
	if err := installSelectedSkills("agent-x", []string{"does-not-exist"}); err == nil {
		t.Fatal("expected an error for an unknown skill")
	}
}

func TestSaveBuiltinToolSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	toolsDir, err := agentdir.ToolsDir("agent-x")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(toolsDir, "tools.json")
	existing := `{"custom_tools":[{"name":"hello","command":"/bin/true"}]}`
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := saveBuiltinToolSelection("agent-x", []string{"bash"}); err != nil {
		t.Fatalf("saveBuiltinToolSelection: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		BuiltinTools map[string]bool  `json:"builtin_tools"`
		CustomTools  []map[string]any `json:"custom_tools"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.BuiltinTools["bash"] {
		t.Fatal("bash should be disabled")
	}
	if !parsed.BuiltinTools["filesystem"] {
		t.Fatal("filesystem should stay enabled")
	}
	if len(parsed.CustomTools) != 1 {
		t.Fatalf("custom tools were not preserved: %+v", parsed.CustomTools)
	}

	if err := saveBuiltinToolSelection("agent-x", []string{"nope"}); err == nil {
		t.Fatal("expected an error for an unknown tool")
	}
}

func TestParseOnboardSelection(t *testing.T) {
	skills := builtinSkillNames()
	tools := builtinToolNames()

	got, err := parseOnboardSelection("Time, NOTES", skills, "--skills")
	if err != nil {
		t.Fatalf("case-insensitive selection failed: %v", err)
	}
	if strings.Join(got, ",") != "notes,time" {
		t.Fatalf("selected = %v, want canonical order [notes time]", got)
	}

	got, err = parseOnboardSelection("python", tools, "--disable-tools")
	if err != nil {
		t.Fatalf("python alias rejected: %v", err)
	}
	if len(got) != 1 || got[0] != "execute_python" {
		t.Fatalf("python alias = %v, want [execute_python]", got)
	}

	got, err = parseOnboardSelection("all", skills, "--skills")
	if err != nil || len(got) != len(skills) {
		t.Fatalf("all = %v (%v), want %d skills", got, err, len(skills))
	}

	_, err = parseOnboardSelection("nope", skills, "--skills")
	if err == nil {
		t.Fatal("expected an error for an unknown skill")
	}
	if !strings.Contains(err.Error(), "unknown name") || !strings.Contains(err.Error(), "valid:") {
		t.Fatalf("error should name the problem and list valid values: %v", err)
	}
}

func TestExistingDisabledToolsAlias(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	toolsDir, err := agentdir.ToolsDir("agent-z")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(toolsDir, "tools.json")
	data := `{"builtin_tools":{"python":false,"bash":true}}`
	if err := os.WriteFile(configPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	got := existingDisabledTools("agent-z")
	if len(got) != 1 || got[0] != "execute_python" {
		t.Fatalf("disabled tools = %v, want [execute_python]", got)
	}
}

func TestExistingCapabilitiesArePreselected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	if err := installSelectedSkills("agent-y", []string{"notes"}); err != nil {
		t.Fatal(err)
	}
	if err := saveBuiltinToolSelection("agent-y", []string{"bash"}); err != nil {
		t.Fatal(err)
	}

	m := initialOnboardModel("agent-y")
	if !m.skillsSelected["notes"] {
		t.Fatal("installed skill was not pre-selected")
	}
	if m.toolsSelected["bash"] {
		t.Fatal("disabled tool should be pre-deselected")
	}
	if !m.toolsSelected["filesystem"] {
		t.Fatal("enabled tool should be pre-selected")
	}
}
