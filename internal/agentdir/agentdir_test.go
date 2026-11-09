package agentdir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateName(t *testing.T) {
	valid := []string{"default", "myfriend", "friend_2", "agent-3", "team1", "123"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) unexpected error: %v", name, err)
		}
	}

	invalid := []string{"", "my friend", "../admin", "my.friend", "my/friend", "-agent", "_agent", "pids", "PIDS", "a" + strings.Repeat("b", MaxNameLength)}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", name)
		}
	}
}

func TestRegistryRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := RegisterAgent("myfriend"); err != nil {
		t.Fatal(err)
	}
	if err := SetDefault("myfriend"); err != nil {
		t.Fatal(err)
	}

	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if registry.Default != "myfriend" {
		t.Fatalf("Default = %q, want myfriend", registry.Default)
	}
	if _, exists := registry.Agents["myfriend"]; !exists {
		t.Fatal("myfriend missing from registry")
	}
}

// A registry that lost entries must not hide agents whose folders still exist:
// that is how four agents became one while every config and database stayed put.
func TestEnsureLayoutRebuildsRegistryFromDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := filepath.Join(home, ".myaaw")

	for _, name := range []string{"main", "alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(root, "agents", name, "config"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "agents", name, "config", "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Shared runtime folders are not agents and must stay out of the registry.
	for _, name := range []string{"pids", "logs", "database"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := SaveRegistry(&Registry{
		Default: "main",
		Agents:  map[string]AgentInfo{"main": {CreatedAt: "2026-01-01T00:00:00Z"}},
	}); err != nil {
		t.Fatal(err)
	}

	if err := EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"main", "alpha", "beta"} {
		if _, exists := registry.Agents[name]; !exists {
			t.Fatalf("%s was not re-registered: %+v", name, registry.Agents)
		}
	}
	for _, name := range []string{"pids", "logs", "database"} {
		if _, exists := registry.Agents[name]; exists {
			t.Fatalf("%s is not an agent but got registered", name)
		}
	}
	if registry.Default != "main" {
		t.Fatalf("default = %q", registry.Default)
	}
	if registry.Agents["main"].CreatedAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("existing entry was rewritten: %+v", registry.Agents["main"])
	}
}

func TestRegisterAgentStoresUTCTimestamp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := RegisterAgent("myfriend"); err != nil {
		t.Fatal(err)
	}
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}

	created := registry.Agents["myfriend"].CreatedAt
	parsed, err := time.Parse(time.RFC3339, created)
	if err != nil {
		t.Fatalf("created_at %q is not RFC3339: %v", created, err)
	}
	if parsed.Location() != time.UTC {
		t.Fatalf("created_at location = %v, want UTC", parsed.Location())
	}
	if time.Since(parsed) > time.Minute {
		t.Fatalf("created_at %v is not recent", parsed)
	}
}

func TestSetActiveExportsAgentEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := SetActive("myfriend"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	t.Cleanup(func() { _ = SetActive(DefaultAgentName) })

	// Tool subprocesses inherit the environment, not the in-process variable. The
	// exported value keeps `myaaw <command>` children on the same agent as the
	// server that spawned them.
	if got := os.Getenv("MYAAW_AGENT"); got != "myfriend" {
		t.Fatalf("MYAAW_AGENT = %q, want myfriend", got)
	}
	if got := Active(); got != "myfriend" {
		t.Fatalf("Active() = %q, want myfriend", got)
	}

	// An in-process selection wins over a stale inherited value.
	t.Setenv("MYAAW_AGENT", "other")
	if got := Active(); got != "myfriend" {
		t.Fatalf("Active() after env change = %q, want myfriend", got)
	}
}

func TestRewriteLegacyPathsForNewAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	agentRoot := filepath.Join(home, ".myaaw", "agents", "myfriend")
	if err := os.MkdirAll(filepath.Join(agentRoot, "home"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(agentRoot, "home", "AGENTS.md")
	if err := os.WriteFile(path, []byte("~/.myaaw/home/MEMORY.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(agentRoot, "skills", "notes", "script.py")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	script := `DATA = os.path.join(os.path.expanduser("~"), ".myaaw", "database", "notes")`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RewriteLegacyPaths(agentRoot, "myfriend"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "~/.myaaw/agents/myfriend/home/MEMORY.md") {
		t.Errorf("path was not rewritten: %s", data)
	}

	scriptData, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scriptData), `".myaaw", "agents", "myfriend", "database"`) {
		t.Errorf("python skill path was not rewritten: %s", scriptData)
	}
}

func TestSuggestedPort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := os.MkdirAll(filepath.Join(home, ".myaaw"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".myaaw", "agents", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".myaaw", "agents", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RegisterAgent(DefaultAgentName); err != nil {
		t.Fatal(err)
	}
	if err := RegisterAgent("alpha"); err != nil {
		t.Fatal(err)
	}
	if err := UpsertEnvValue(filepath.Join(home, ".myaaw", "agents", "default", ".env"), "PORT", "8080"); err != nil {
		t.Fatal(err)
	}
	if err := UpsertEnvValue(filepath.Join(home, ".myaaw", "agents", "alpha", ".env"), "PORT", "8081"); err != nil {
		t.Fatal(err)
	}

	if got := SuggestedPort("beta"); got != 8082 {
		t.Fatalf("SuggestedPort(beta) = %d, want 8082", got)
	}
}

func TestApprovalsPathMigratesLegacyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	agentRoot := filepath.Join(home, ".myaaw", "agents", "myfriend")
	if err := os.MkdirAll(agentRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(agentRoot, "approvals.json")
	content := []byte(`{"bash":{"always":true}}`)
	if err := os.WriteFile(legacy, content, 0o600); err != nil {
		t.Fatal(err)
	}

	path, err := ApprovalsPath("myfriend")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(agentRoot, "config", "approvals.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}

	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("migrated file missing: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("migrated content = %q, want %q", data, content)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy approvals.json should have been moved")
	}

	// Idempotent: resolving again keeps the migrated file untouched.
	if _, err := ApprovalsPath("myfriend"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(want); err != nil || string(data) != string(content) {
		t.Fatalf("second resolve changed the file: %q, err=%v", data, err)
	}
}
