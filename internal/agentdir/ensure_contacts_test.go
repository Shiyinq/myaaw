package agentdir

import "testing"

func TestEnsureA2AContacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	setupAgent(t, "alice", "8081", "alice-secret")
	setupAgent(t, "bob", "8082", "")

	if err := EnsureA2AContacts("bob"); err != nil {
		t.Fatalf("EnsureA2AContacts(bob): %v", err)
	}

	bobToken := readEnvValue("bob", "A2A_TOKEN")
	if bobToken == "" {
		t.Fatal("bob should have a generated A2A_TOKEN")
	}

	bobContacts := readContacts(t, "bob")
	if len(bobContacts) != 1 || bobContacts[0]["name"] != "alice" {
		t.Fatalf("bob contacts = %+v, want alice", bobContacts)
	}
	if bobContacts[0]["token"] != "alice-secret" {
		t.Fatalf("bob contact token = %v, want alice-secret", bobContacts[0]["token"])
	}

	aliceContacts := readContacts(t, "alice")
	if len(aliceContacts) != 1 || aliceContacts[0]["name"] != "bob" {
		t.Fatalf("alice contacts = %+v, want bob", aliceContacts)
	}
	if aliceContacts[0]["token"] != bobToken {
		t.Fatalf("alice sees bob token %v, want %s", aliceContacts[0]["token"], bobToken)
	}

	// Re-running must not rotate bob's token or duplicate contacts.
	if err := EnsureA2AContacts("bob"); err != nil {
		t.Fatalf("EnsureA2AContacts(bob) second run: %v", err)
	}
	if got := readEnvValue("bob", "A2A_TOKEN"); got != bobToken {
		t.Fatalf("bob token rotated on re-run: %q != %q", got, bobToken)
	}
	if got := len(readContacts(t, "bob")); got != 1 {
		t.Fatalf("bob has %d contacts after re-run, want 1", got)
	}
	if got := len(readContacts(t, "alice")); got != 1 {
		t.Fatalf("alice has %d contacts after re-run, want 1", got)
	}

	// A second agent can join later and both directions are filled in.
	if err := EnsureA2AContacts("alice"); err != nil {
		t.Fatalf("EnsureA2AContacts(alice): %v", err)
	}
	if got := readEnvValue("alice", "A2A_TOKEN"); got != "alice-secret" {
		t.Fatalf("alice token rotated: %q", got)
	}
	if got := len(readContacts(t, "alice")); got != 1 {
		t.Fatalf("alice has %d contacts, want 1", got)
	}
}
