package capabilities

import "testing"

func TestParseDropsMetadataAndReadsLimits(t *testing.T) {
	data := []byte(`{
		"_schema": {"version": 3},
		"_comment": "metadata",
		"demo": {
			"provider": "demo",
			"status": "active",
			"capabilities": {"thinking": true, "vision": null},
			"thinking": {"can_disable": true, "effort_values": ["low", "high"]},
			"limits": {"context_window_tokens": 123}
		}
	}`)

	view, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if view.Version != 3 {
		t.Fatalf("version = %d, want 3", view.Version)
	}
	if _, ok := view.Models["_comment"]; ok {
		t.Fatal("metadata key leaked into the model map")
	}
	entry, ok := view.Models["demo"]
	if !ok {
		t.Fatal("demo model missing")
	}
	if !Supports(entry, "thinking") {
		t.Fatal("thinking should be supported")
	}
	if Supports(entry, "vision") {
		t.Fatal("nil capability must not count as support")
	}
	if entry.ContextWindow == nil || *entry.ContextWindow != 123 {
		t.Fatalf("context window = %v", entry.ContextWindow)
	}
	if entry.Thinking == nil || len(entry.Thinking.EffortValues) != 2 {
		t.Fatalf("thinking = %+v", entry.Thinking)
	}
}

func TestLookupToleratesModelsPrefix(t *testing.T) {
	SetForTest(View{Version: 1, Models: map[string]Entry{"gemini-x": {Provider: "gemini"}}})
	t.Cleanup(Reset)

	if _, ok := Lookup("gemini-x"); !ok {
		t.Fatal("exact id not found")
	}
	entry, ok := Lookup("models/gemini-x")
	if !ok || entry.Provider != "gemini" {
		t.Fatalf("prefixed id = %+v, %v", entry, ok)
	}
	if _, ok := Lookup("missing"); ok {
		t.Fatal("unknown id must not resolve")
	}
}
