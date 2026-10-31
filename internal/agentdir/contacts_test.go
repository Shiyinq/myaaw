package agentdir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupAgent(t *testing.T, name, port, token string) {
	t.Helper()
	root, err := AgentRoot(name)
	if err != nil {
		t.Fatalf("AgentRoot(%q): %v", name, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	// config.json existence marks the agent as initialized.
	if err := os.WriteFile(filepath.Join(root, "config", "config.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	env := "PORT=" + port + "\nMYAAW_BASE_URL=http://localhost:" + port + "\n"
	if token != "" {
		env += "A2A_TOKEN=" + token + "\n"
	}
	envPath, err := EnvPath(name)
	if err != nil {
		t.Fatalf("EnvPath: %v", err)
	}
	if err := os.WriteFile(envPath, []byte(env), 0o644); err != nil {
		t.Fatalf("write env: %v", err)
	}
	if err := RegisterAgent(name); err != nil {
		t.Fatalf("RegisterAgent(%q): %v", name, err)
	}
}

func readContacts(t *testing.T, name string) []map[string]any {
	t.Helper()
	path, err := ContactsPath(name)
	if err != nil {
		t.Fatalf("ContactsPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contacts: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("parse contacts: %v", err)
	}
	return entries
}

func TestSeedA2AContacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	setupAgent(t, "alice", "8081", "alice-secret")
	setupAgent(t, "bob", "8082", "bob-secret")
	setupAgent(t, "carol", "8083", "")

	if err := SeedA2AContacts("carol"); err != nil {
		t.Fatalf("SeedA2AContacts: %v", err)
	}

	// The new agent gets a generated token.
	carolToken := readEnvValue("carol", "A2A_TOKEN")
	if carolToken == "" {
		t.Fatal("carol should have a generated A2A_TOKEN")
	}

	// The new agent has contacts for each other agent, with their tokens.
	contacts := readContacts(t, "carol")
	if len(contacts) != 2 {
		t.Fatalf("carol contacts = %+v, want 2", contacts)
	}
	byName := map[string]map[string]any{}
	for _, entry := range contacts {
		byName[entry["name"].(string)] = entry
	}
	if byName["alice"]["url"] != "http://localhost:8081/a2a" || byName["alice"]["token"] != "alice-secret" {
		t.Fatalf("alice contact = %+v", byName["alice"])
	}
	if byName["bob"]["url"] != "http://localhost:8082/a2a" || byName["bob"]["token"] != "bob-secret" {
		t.Fatalf("bob contact = %+v", byName["bob"])
	}

	// Existing agents get the new agent added, with the generated token.
	for _, other := range []string{"alice", "bob"} {
		entries := readContacts(t, other)
		if len(entries) != 1 {
			t.Fatalf("%s contacts = %+v, want 1", other, entries)
		}
		if entries[0]["name"] != "carol" || entries[0]["url"] != "http://localhost:8083/a2a" || entries[0]["token"] != carolToken || entries[0]["description"] != "Myaaw agent carol" {
			t.Fatalf("%s carol contact = %+v", other, entries[0])
		}
	}
}

func TestRemoveA2AContacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	setupAgent(t, "alice", "8081", "alice-secret")
	setupAgent(t, "bob", "8082", "bob-secret")
	setupAgent(t, "carol", "8083", "")
	if err := SeedA2AContacts("carol"); err != nil {
		t.Fatalf("SeedA2AContacts: %v", err)
	}

	// Alice also keeps a manual external contact that must survive the cleanup.
	alicePath, _ := ContactsPath("alice")
	if err := os.WriteFile(alicePath, []byte(`[{"name":"external","url":"https://x/a2a"},{"name":"carol","url":"http://localhost:8083/a2a"}]`), 0o600); err != nil {
		t.Fatalf("write alice contacts: %v", err)
	}

	if err := RemoveA2AContacts("carol"); err != nil {
		t.Fatalf("RemoveA2AContacts: %v", err)
	}

	for _, other := range []string{"alice", "bob"} {
		entries := readContacts(t, other)
		for _, entry := range entries {
			if name, _ := entry["name"].(string); name == "carol" {
				t.Fatalf("%s still lists carol: %+v", other, entries)
			}
		}
	}
	alice := readContacts(t, "alice")
	if len(alice) != 1 || alice[0]["name"] != "external" {
		t.Fatalf("alice contacts after cleanup = %+v, want only external", alice)
	}
}

func TestSeedA2AContactsPreservesExistingContacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	setupAgent(t, "alice", "8081", "alice-secret")

	// Alice already has a manually-added external contact that must survive.
	alicePath, _ := ContactsPath("alice")
	if err := os.WriteFile(alicePath, []byte(`[{"name":"external","url":"https://x/a2a","token":"t","description":"keep me"}]`), 0o600); err != nil {
		t.Fatalf("write alice contacts: %v", err)
	}

	setupAgent(t, "carol", "8082", "")
	if err := SeedA2AContacts("carol"); err != nil {
		t.Fatalf("SeedA2AContacts: %v", err)
	}

	entries := readContacts(t, "alice")
	if len(entries) != 2 {
		t.Fatalf("alice contacts = %+v, want 2 (external + carol)", entries)
	}
	foundExternal, foundCarol := false, false
	for _, entry := range entries {
		switch entry["name"] {
		case "external":
			foundExternal = true
			if entry["description"] != "keep me" {
				t.Fatalf("external description lost: %+v", entry)
			}
		case "carol":
			foundCarol = true
			if entry["token"] == "" || entry["url"] != "http://localhost:8082/a2a" {
				t.Fatalf("carol contact = %+v", entry)
			}
		}
	}
	if !foundExternal || !foundCarol {
		t.Fatalf("alice contacts = %+v", entries)
	}
}
