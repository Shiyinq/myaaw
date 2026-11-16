package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMCPWorkingDirLivesInAgentWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYAAW_AGENT", "tester")

	dir, err := mcpWorkingDir()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(home, ".myaaw", "agents", "tester", "home", "workspace", "mcp")
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	if resolved, err := filepath.EvalSymlinks(want); err == nil {
		want = resolved
	}
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("working directory was not created: %v", err)
	}
}
