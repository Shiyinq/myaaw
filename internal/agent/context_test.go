package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureWorkspaceCreatesDirectoryTree(t *testing.T) {
	homeDir := t.TempDir()

	workspaceDir, err := ensureWorkspace(homeDir)
	if err != nil {
		t.Fatalf("ensureWorkspace() error = %v", err)
	}

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

	if _, err := ensureWorkspace(""); err == nil {
		t.Error("expected an error for an empty home directory")
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

	workspaceDir, err := ensureWorkspace(homeDir)
	if err != nil {
		t.Fatal(err)
	}

	builder := &SystemPromptBuilder{
		userID:       42,
		channelName:  "test",
		homeDir:      homeDir,
		workspaceDir: workspaceDir,
	}

	prompt := builder.Build()

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

func TestSystemPromptIncludesCurrentDirectory(t *testing.T) {
	builder := &SystemPromptBuilder{
		homeDir:    t.TempDir(),
		currentDir: "/Users/example/my-project",
	}

	prompt := builder.Build()
	for _, want := range []string{
		"## Current Directory",
		"Myaaw launch directory: /Users/example/my-project",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("system prompt missing %q\n\n%s", want, prompt)
		}
	}
}

func TestNewSystemPromptBuilderCapturesLocalDirectory(t *testing.T) {
	myaawDir := t.TempDir()

	builder := newSystemPromptBuilder(7, "cli", func() (string, error) {
		return "/Users/example/my-project", nil
	}, func() (string, error) {
		return myaawDir, nil
	})

	if builder.currentDir != "/Users/example/my-project" {
		t.Fatalf("currentDir = %q, want %q", builder.currentDir, "/Users/example/my-project")
	}

	projectsDir := filepath.Join(myaawDir, "home", workspaceDirName, "projects")
	if info, err := os.Stat(projectsDir); err != nil || !info.IsDir() {
		t.Fatalf("expected workspace projects directory at %s: %v", projectsDir, err)
	}
}

func TestNewSystemPromptBuilderOmitsRemoteDirectory(t *testing.T) {
	getwdCalled := false

	builder := newSystemPromptBuilder(7, "telegram", func() (string, error) {
		getwdCalled = true
		return "/server/cwd", nil
	}, func() (string, error) {
		return t.TempDir(), nil
	})

	if getwdCalled {
		t.Error("getwd should not be called for remote channels")
	}
	if builder.currentDir != "" {
		t.Errorf("currentDir = %q, want empty", builder.currentDir)
	}
}

func TestLocalChannelDetection(t *testing.T) {
	cases := map[string]bool{
		"cli":           true,
		"voice-classic": true,
		"telegram":      false,
		"discord":       false,
		"":              false,
	}

	for channel, want := range cases {
		if got := isLocalChannel(channel); got != want {
			t.Errorf("isLocalChannel(%q) = %v, want %v", channel, got, want)
		}
	}
}
