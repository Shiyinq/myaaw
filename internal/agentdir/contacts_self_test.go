package agentdir

import (
	"encoding/json"
	"os"
	"testing"
)

func assertNoSelfContact(t *testing.T, name string) {
	t.Helper()
	for _, entry := range readContacts(t, name) {
		if got, _ := entry["name"].(string); sameAgentName(got, name) {
			t.Fatalf("%s lists itself as a contact: %+v", name, entry)
		}
	}
}

func TestEnsureA2AContactsDropsSelfEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	setupAgent(t, "alice", "8081", "alice-secret")
	setupAgent(t, "bob", "8082", "bob-secret")

	// Simulate an older or hand-edited contacts.json that includes alice
	// herself plus an unrelated external contact.
	path, err := ContactsPath("alice")
	if err != nil {
		t.Fatal(err)
	}
	entries := []map[string]any{
		{"name": "alice", "url": "http://localhost:8081/a2a"},
		{"name": "external", "url": "https://example.com/a2a", "token": "ext"},
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureA2AContacts("alice"); err != nil {
		t.Fatalf("EnsureA2AContacts: %v", err)
	}

	assertNoSelfContact(t, "alice")

	names := map[string]bool{}
	for _, entry := range readContacts(t, "alice") {
		name, _ := entry["name"].(string)
		names[name] = true
	}
	if names["alice"] {
		t.Fatal("self entry survived")
	}
	if !names["external"] {
		t.Fatal("unrelated external contact was dropped")
	}
	if !names["bob"] {
		t.Fatal("bob was not added")
	}
}

func TestEnsureA2AContactsIgnoresCaseOnlySelf(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	setupAgent(t, "agent-b", "8081", "token-b")
	setupAgent(t, "Agent-B", "8081", "token-b")

	if err := EnsureA2AContacts("agent-b"); err != nil {
		t.Fatalf("EnsureA2AContacts: %v", err)
	}

	assertNoSelfContact(t, "agent-b")
	if got := len(readContacts(t, "agent-b")); got != 0 {
		t.Fatalf("expected no contacts for a case-only duplicate, got %d", got)
	}
}

func TestUpsertContactRejectsSelf(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	setupAgent(t, "alice", "8081", "alice-secret")
	if err := ensureContactsFile("alice"); err != nil {
		t.Fatal(err)
	}

	if err := upsertContact("alice", contactEntry{Name: "ALICE", URL: "http://localhost:8081/a2a"}); err != nil {
		t.Fatalf("upsertContact: %v", err)
	}
	if got := len(readContacts(t, "alice")); got != 0 {
		t.Fatalf("self contact was written (%d entries)", got)
	}
}
