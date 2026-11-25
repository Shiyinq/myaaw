package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSkillsInstructionForExternalRuntime(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	homeDir := filepath.Join(root, "home")
	calendarDir := filepath.Join(skillsDir, "calendar")
	if err := os.MkdirAll(calendarDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, ".venv", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	venvPy := filepath.Join(homeDir, ".venv", "bin", "python")
	if err := os.WriteFile(venvPy, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: Calendar\ndescription: Manage events\n---\nRun via bash.\n"
	if err := os.WriteFile(filepath.Join(calendarDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	native := buildSkillsInstructionForRuntime(skillsDir, homeDir, nil, false)
	if !strings.Contains(native, "You can use the 'filesystem' to read file skill") {
		t.Fatalf("expected native wording, got:\n%s", native)
	}

	external := buildSkillsInstructionForRuntime(skillsDir, homeDir, nil, true)
	if strings.Contains(external, "You can use the 'filesystem' to read file skill") {
		t.Fatalf("external wording must not instruct calling literal filesystem tool:\n%s", external)
	}
	if !strings.Contains(external, "built-in file-reading tool") {
		t.Fatalf("expected runtime-neutral wording:\n%s", external)
	}
	if !strings.Contains(external, venvPy) {
		t.Fatalf("expected absolute venv python path %q in external skills prompt:\n%s", venvPy, external)
	}
}
