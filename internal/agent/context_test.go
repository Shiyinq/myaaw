package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureWorkspaceCreatesDirectoryTree(t *testing.T) {
	homeDir := t.TempDir()

	workspaceDir := ensureWorkspace(homeDir)

	if want := filepath.Join(homeDir, workspaceDirName); workspaceDir != want {
		t.Fatalf("ensureWorkspace() = %q, want %q", workspaceDir, want)
	}

	for _, dir := range []string{
		workspaceDir,
		filepath.Join(workspaceDir, "projects"),
		filepath.Join(workspaceDir, "downloads"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected directory %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", dir)
		}
	}
}

func TestSystemPromptIncludesWorkspacePaths(t *testing.T) {
	homeDir := t.TempDir()
	const agentsRule = "All generated output MUST stay inside the workspace."
	if err := os.WriteFile(filepath.Join(homeDir, "AGENTS.md"), []byte(agentsRule), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "SOUL.md"), []byte("soul rules"), 0o644); err != nil {
		t.Fatal(err)
	}

	builder := &SystemPromptBuilder{
		userID:       42,
		channelName:  "test",
		homeDir:      homeDir,
		workspaceDir: ensureWorkspace(homeDir),
	}

	prompt := builder.Build()

	workspaceDir := filepath.Join(homeDir, workspaceDirName)
	required := []string{
		"## Workspace",
		"Workspace root: " + workspaceDir,
		filepath.Join(workspaceDir, "projects"),
		filepath.Join(workspaceDir, "downloads"),
		agentsRule,
	}
	for _, want := range required {
		if !strings.Contains(prompt, want) {
			t.Errorf("system prompt missing %q\n\n%s", want, prompt)
		}
	}

	if count := strings.Count(prompt, agentsRule); count != 1 {
		t.Errorf("workspace rule from AGENTS.md should appear once, got %d", count)
	}

	workspaceIndex := strings.Index(prompt, "## Workspace")
	soulIndex := strings.Index(prompt, "## SOUL.md")
	if workspaceIndex == -1 || soulIndex == -1 {
		t.Fatalf("expected both workspace and SOUL sections in prompt:\n\n%s", prompt)
	}
	if workspaceIndex > soulIndex {
		t.Errorf("workspace section should be injected before SOUL.md")
	}
}

func TestSystemPromptSkipsWorkspaceWithoutHome(t *testing.T) {
	builder := &SystemPromptBuilder{}

	if prompt := builder.Build(); strings.Contains(prompt, "## Workspace") {
		t.Fatalf("expected no workspace section when home is unavailable:\n\n%s", prompt)
	}
}
