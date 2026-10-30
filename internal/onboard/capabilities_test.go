package onboard

import "testing"

func TestBuiltinCapabilitiesDescribeEveryEntry(t *testing.T) {
	caps := BuiltinCapabilities()

	if len(caps.Providers) != 3 {
		t.Fatalf("providers = %d, want 3", len(caps.Providers))
	}
	wantPresets := []struct{ ID, Type string }{
		{"openai", "openai"},
		{"deepseek", "openai"},
		{"gemini", "gemini"},
	}
	for i, want := range wantPresets {
		preset := caps.Providers[i]
		if preset.ID != want.ID || preset.Type != want.Type {
			t.Fatalf("provider[%d] = %+v, want id %q type %q", i, preset, want.ID, want.Type)
		}
		if preset.Label == "" {
			t.Fatalf("provider[%d] has no label", i)
		}
		if preset.DefaultBaseURL == "" {
			t.Fatalf("provider[%d] has no default base URL", i)
		}
		if preset.DefaultModel == "" {
			t.Fatalf("provider[%d] has no default model", i)
		}
	}

	// DeepSeek is an OpenAI-compatible preset: it must keep the wire type but
	// point at its own endpoint.
	deepseek := caps.Providers[1]
	if deepseek.Type != "openai" || deepseek.DefaultBaseURL != "https://api.deepseek.com" {
		t.Fatalf("deepseek preset = %+v", deepseek)
	}

	if len(caps.Skills) < 9 {
		t.Fatalf("skills = %d, want at least 9", len(caps.Skills))
	}
	if len(caps.Tools) < 9 {
		t.Fatalf("tools = %d, want at least 9", len(caps.Tools))
	}

	for _, entry := range append(append([]Capability{}, caps.Skills...), caps.Tools...) {
		if entry.Name == "" {
			t.Fatalf("capability without a name: %+v", entry)
		}
	}
	if caps.Skills[0].Description == "" {
		t.Fatal("skill descriptions were not loaded from SKILL.md")
	}

	// Wiring the catalog to the real registries keeps the wizard from offering
	// a name that onboarding would later reject.
	knownSkills := make(map[string]bool)
	for _, name := range BuiltinSkillNames() {
		knownSkills[name] = true
	}
	for _, entry := range caps.Skills {
		if !knownSkills[entry.Name] {
			t.Fatalf("unknown skill %q", entry.Name)
		}
	}
	knownTools := make(map[string]bool)
	for _, name := range BuiltinToolNames() {
		knownTools[name] = true
	}
	for _, entry := range caps.Tools {
		if !knownTools[entry.Name] {
			t.Fatalf("unknown tool %q", entry.Name)
		}
	}
}

func TestFrontmatterDescriptionReadsSingleLine(t *testing.T) {
	content := "---\nname: demo\ndescription: >\n  Multi line\n  description here\n---\n\n# Demo\n"
	if got := FrontmatterDescription(content); got != "Multi line description here" {
		t.Fatalf("description = %q", got)
	}
	if got := FrontmatterDescription("no frontmatter"); got != "" {
		t.Fatalf("description = %q, want empty", got)
	}
}
