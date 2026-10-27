package agentdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentNameTaken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}

	// The registry placeholder for "default" has no directory, so the name is
	// still available on a fresh install.
	if taken, err := AgentNameTaken("default"); err != nil || taken {
		t.Fatalf("AgentNameTaken(default) = %v, %v; want false, nil", taken, err)
	}

	setupAgent(t, "alice", "8081", "alice-secret")

	if taken, err := AgentNameTaken("alice"); err != nil || !taken {
		t.Fatalf("AgentNameTaken(alice) = %v, %v; want true, nil", taken, err)
	}
	// Case-only duplicates are rejected so the registry stays consistent with
	// the case-insensitive A2A contact matching.
	if taken, err := AgentNameTaken("ALICE"); err != nil || !taken {
		t.Fatalf("AgentNameTaken(ALICE) = %v, %v; want true, nil", taken, err)
	}
	if taken, err := AgentNameTaken("bob"); err != nil || taken {
		t.Fatalf("AgentNameTaken(bob) = %v, %v; want false, nil", taken, err)
	}

	// A leftover directory from a failed run also blocks the name.
	root, err := AgentRoot("leftover")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if taken, err := AgentNameTaken("leftover"); err != nil || !taken {
		t.Fatalf("AgentNameTaken(leftover) = %v, %v; want true, nil", taken, err)
	}

	// Invalid names surface the validator error.
	if _, err := AgentNameTaken("-bad"); err == nil {
		t.Fatal("expected an error for an invalid name")
	}
}
