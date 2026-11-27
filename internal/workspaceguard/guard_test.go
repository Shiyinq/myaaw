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

func TestGitBranch(t *testing.T) {
	root := t.TempDir()

	// Non-git directory returns empty string.
	plainDir := filepath.Join(root, "plain")
	if err := os.MkdirAll(plainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := GitBranch(plainDir); got != "" {
		t.Fatalf("expected empty branch for non-git dir, got %q", got)
	}

	// Standard .git directory with branch ref.
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".git", "HEAD"), []byte("ref: refs/heads/feat/chat-working-dir\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := GitBranch(repoDir); got != "feat/chat-working-dir" {
		t.Fatalf("expected feat/chat-working-dir, got %q", got)
	}

	// Detached HEAD returns 7-char short commit hash.
	if err := os.WriteFile(filepath.Join(repoDir, ".git", "HEAD"), []byte("a1b2c3d4e5f60718293a4b5c6d7e8f9012345678\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := GitBranch(repoDir); got != "a1b2c3d" {
		t.Fatalf("expected short detached hash a1b2c3d, got %q", got)
	}

	// Worktree .git pointer file (relative gitdir).
	wtGitDir := filepath.Join(repoDir, ".git", "worktrees", "wt-rel")
	if err := os.MkdirAll(wtGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtGitDir, "HEAD"), []byte("ref: refs/heads/worktree-rel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wtRelDir := filepath.Join(root, "wt-rel")
	if err := os.MkdirAll(wtRelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtRelDir, ".git"), []byte("gitdir: ../repo/.git/worktrees/wt-rel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := GitBranch(wtRelDir); got != "worktree-rel" {
		t.Fatalf("expected worktree-rel for relative worktree pointer, got %q", got)
	}

	// Worktree .git pointer file (absolute gitdir) with detached HEAD.
	wtAbsGitDir := filepath.Join(repoDir, ".git", "worktrees", "wt-abs")
	if err := os.MkdirAll(wtAbsGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtAbsGitDir, "HEAD"), []byte("deadbeef1234567890\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wtAbsDir := filepath.Join(root, "wt-abs")
	if err := os.MkdirAll(wtAbsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtAbsDir, ".git"), []byte("gitdir: "+wtAbsGitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := GitBranch(wtAbsDir); got != "deadbee" {
		t.Fatalf("expected deadbee for detached absolute worktree pointer, got %q", got)
	}
}
