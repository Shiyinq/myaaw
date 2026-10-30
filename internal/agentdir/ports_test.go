package agentdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSuggestedPortUsesLowestFreePort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}

	// A fresh install whose first agent is not called "default" still gets 8080.
	if got := SuggestedPort("personal"); got != 8080 {
		t.Fatalf("SuggestedPort(personal) = %d, want 8080", got)
	}

	setupAgent(t, "first", "8080", "")
	if got := SuggestedPort("personal"); got != 8081 {
		t.Fatalf("SuggestedPort(personal) = %d, want 8081", got)
	}

	setupAgent(t, "second", "8081", "")
	if got := SuggestedPort("personal"); got != 8082 {
		t.Fatalf("SuggestedPort(personal) = %d, want 8082", got)
	}

	// "default" is not pinned to 8080 when another agent already owns it.
	if got := SuggestedPort(DefaultAgentName); got != 8082 {
		t.Fatalf("SuggestedPort(default) = %d, want 8082", got)
	}

	// An agent's own assignment is not treated as a conflict.
	if got := SuggestedPort("first"); got != 8080 {
		t.Fatalf("SuggestedPort(first) = %d, want its own 8080", got)
	}

	// A registered agent without PORT does not reserve a number.
	root, err := AgentRoot("unassigned")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	envPath, err := EnvPath("unassigned")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envPath, []byte("A2A_TOKEN=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RegisterAgent("unassigned"); err != nil {
		t.Fatal(err)
	}
	if got := SuggestedPort("personal"); got != 8082 {
		t.Fatalf("SuggestedPort(personal) = %d, want 8082", got)
	}
}

func TestReadPort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	setupAgent(t, "alice", "8090", "")

	if got := readPort("alice"); got != 8090 {
		t.Fatalf("readPort(alice) = %d, want 8090", got)
	}
	if got := readPort("missing"); got != 0 {
		t.Fatalf("readPort(missing) = %d, want 0", got)
	}
}
