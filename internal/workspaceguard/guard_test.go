package workspaceguard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDirPolicy(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("MYAAW_ALLOW_HOST_DIRS", "")
	t.Setenv("MYAAW_WORKSPACE_ROOTS", "")

	// Agent A and Agent B roots under temp HOME/.myaaw
	agentAWorkspace := filepath.Join(tmpHome, ".myaaw", "agents", "alpha", "home", "workspace", "projects", "demo")
	agentAConfig := filepath.Join(tmpHome, ".myaaw", "agents", "alpha", "config")
	agentBWorkspace := filepath.Join(tmpHome, ".myaaw", "agents", "beta", "home", "workspace")
	sshDir := filepath.Join(tmpHome, ".ssh")
	hostProject := filepath.Join(tmpHome, "Development", "my-app")

	for _, dir := range []string{agentAWorkspace, agentAConfig, agentBWorkspace, sshDir, hostProject} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Allowed: agent's own workspace project & normal host project
	if _, err := ValidateDir("alpha", agentAWorkspace); err != nil {
		t.Fatalf("expected own workspace project allowed, got %v", err)
	}
	if _, err := ValidateDir("alpha", hostProject); err != nil {
		t.Fatalf("expected host project allowed, got %v", err)
	}

	// Forbidden: /, $HOME root, ~/.ssh, own config dir, another agent's workspace
	for _, bad := range []string{"/", tmpHome, sshDir, agentAConfig, agentBWorkspace} {
		if _, err := ValidateDir("alpha", bad); err == nil {
			t.Fatalf("expected %q to be rejected", bad)
		}
	}

	// MYAAW_ALLOW_HOST_DIRS=0 blocks hostProject but keeps own workspace allowed
	t.Setenv("MYAAW_ALLOW_HOST_DIRS", "0")
	if _, err := ValidateDir("alpha", hostProject); err == nil {
		t.Fatalf("expected host project rejected when MYAAW_ALLOW_HOST_DIRS=0")
	}
	if _, err := ValidateDir("alpha", agentAWorkspace); err != nil {
		t.Fatalf("expected own workspace still allowed when MYAAW_ALLOW_HOST_DIRS=0, got %v", err)
	}

	// MYAAW_WORKSPACE_ROOTS restricts host paths to configured roots
	t.Setenv("MYAAW_ALLOW_HOST_DIRS", "1")
	allowedRoot := filepath.Join(tmpHome, "Development")
	otherDir := filepath.Join(tmpHome, "Other")
	_ = os.MkdirAll(otherDir, 0o755)
	t.Setenv("MYAAW_WORKSPACE_ROOTS", allowedRoot)
	if _, err := ValidateDir("alpha", hostProject); err != nil {
		t.Fatalf("expected path inside MYAAW_WORKSPACE_ROOTS allowed, got %v", err)
	}
	if _, err := ValidateDir("alpha", otherDir); err == nil {
		t.Fatalf("expected path outside MYAAW_WORKSPACE_ROOTS rejected")
	}
}

func TestValidateCreatableDirAndHasGit(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("MYAAW_ALLOW_HOST_DIRS", "")
	t.Setenv("MYAAW_WORKSPACE_ROOTS", "")

	parent := filepath.Join(tmpHome, "Development")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}

	target, err := ValidateCreatableDir("alpha", parent, "new-empty-app")
	if err != nil {
		t.Fatalf("expected creatable dir allowed, got %v", err)
	}
	if filepath.Base(target) != "new-empty-app" {
		t.Fatalf("unexpected target %q", target)
	}

	if _, err := ValidateCreatableDir("alpha", parent, "../escape"); err == nil {
		t.Fatalf("expected traversal name rejected")
	}

	if HasGit(parent) {
		t.Fatalf("expected HasGit=false before .git created")
	}
	if err := os.MkdirAll(filepath.Join(parent, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !HasGit(parent) {
		t.Fatalf("expected HasGit=true after .git created")
	}
}
