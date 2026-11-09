package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseModelCapabilities(t *testing.T) {
	data := []byte(`{
		"_schema": {"version": 2},
		"demo-model": {
			"provider": "demo",
			"status": "preview",
			"capabilities": {"thinking": true, "vision": null, "json_mode": true},
			"thinking": {"can_disable": true, "effort_values": ["none", "high"]},
			"input_modalities": ["text", "image"],
			"limits": {"context_window_tokens": 128000}
		}
	}`)

	view, err := parseModelCapabilities(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if view.Version != 2 {
		t.Fatalf("version = %d", view.Version)
	}
	entry, ok := view.Models["demo-model"]
	if !ok {
		t.Fatal("model missing")
	}
	if entry.Status != "preview" || entry.ContextWindow == nil || *entry.ContextWindow != 128000 {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Capabilities["vision"] != nil {
		t.Fatal("null capability must stay nil/unverified")
	}
	if entry.Thinking == nil || len(entry.Thinking.EffortValues) != 2 {
		t.Fatalf("thinking = %+v", entry.Thinking)
	}
	if _, ok := view.Models["_schema"]; ok {
		t.Fatal("metadata key leaked into the model map")
	}
}

func TestModelCapabilitiesSeedsGlobalFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	view, err := (&ConfigService{}).ModelCapabilities()
	if err != nil {
		t.Fatalf("ModelCapabilities: %v", err)
	}
	if _, ok := view.Models["deepseek-flash"]; !ok {
		t.Fatalf("embedded catalog was not seeded: %d models", len(view.Models))
	}
	path := filepath.Join(home, ".myaaw", "config", "model_capabilities.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("global capabilities file missing: %v", err)
	}
}
