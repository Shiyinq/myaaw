package agentdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentColorOrDefault(t *testing.T) {
	cases := map[string]string{
		"":        DefaultAgentColor,
		"brun":    "brun",
		"creme":   "creme",
		"mauve":   DefaultAgentColor,
		"BRUN":    DefaultAgentColor,
		"encre  ": DefaultAgentColor,
	}
	for raw, want := range cases {
		if got := AgentColorOrDefault(raw); got != want {
			t.Fatalf("AgentColorOrDefault(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestValidAgentColorCoversThePalette(t *testing.T) {
	if len(AgentColors) != 12 {
		t.Fatalf("palette should hold 12 colours, found %d", len(AgentColors))
	}
	seen := map[string]bool{}
	for _, color := range AgentColors {
		if seen[color] {
			t.Fatalf("duplicate colour %q", color)
		}
		seen[color] = true
		if !ValidAgentColor(color) {
			t.Fatalf("%q should be valid", color)
		}
	}
	if !ValidAgentColor(DefaultAgentColor) {
		t.Fatalf("default colour %q must be part of the palette", DefaultAgentColor)
	}
	if ValidAgentColor("") || ValidAgentColor("mauve") {
		t.Fatal("empty and unknown keys must be rejected")
	}
}

func TestSetAgentColorPersists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := RegisterAgent("painted"); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if err := SetAgentColor("painted", "violet"); err != nil {
		t.Fatalf("SetAgentColor: %v", err)
	}

	registry, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if got := registry.Agents["painted"].AgentColor; got != "violet" {
		t.Fatalf("stored colour = %q, want violet", got)
	}
	// The entry keeps its creation time: setting a colour is not a re-register.
	if registry.Agents["painted"].CreatedAt == "" {
		t.Fatal("CreatedAt was dropped")
	}
}

func TestSetAgentColorRejectsUnknownAndUnregistered(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := RegisterAgent("painted"); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if err := SetAgentColor("painted", "mauve"); err == nil {
		t.Fatal("unknown colour should be rejected")
	}
	if err := SetAgentColor("ghost", "brun"); err == nil {
		t.Fatal("unregistered agent should be rejected")
	}

	registry, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if got := registry.Agents["painted"].AgentColor; got != "" {
		t.Fatalf("rejected write must not persist, found %q", got)
	}
}

// A registry written before colours existed has no agent_color key at all; it
// must still load and read back as the default.
func TestLoadRegistryWithoutAgentColor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path, err := RegistryPath()
	if err != nil {
		t.Fatalf("RegistryPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"default":"default","agents":{"default":{"created_at":"2026-10-01T08:00:00Z"}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	registry, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if got := registry.Agents["default"].AgentColor; got != "" {
		t.Fatalf("legacy entry gained a colour: %q", got)
	}
	if got := AgentColorOrDefault(registry.Agents["default"].AgentColor); got != DefaultAgentColor {
		t.Fatalf("resolved colour = %q, want %q", got, DefaultAgentColor)
	}
}
