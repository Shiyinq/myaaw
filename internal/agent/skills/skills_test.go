package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSkillEnabled(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	cases := []struct {
		name               string
		skill              string
		filter             map[string]bool
		frontmatterEnabled *bool
		want               bool
	}{
		{"nil filter and frontmatter defaults to enabled", "weather", nil, nil, true},
		{"explicitly enabled in filter", "weather", map[string]bool{"weather": true}, nil, true},
		{"explicitly disabled in filter", "tavily", map[string]bool{"tavily": false}, nil, false},
		{"omitted in filter defaults to enabled", "tavily", map[string]bool{"weather": true}, nil, true},
		{"frontmatter false disables when omitted from filter", "tavily", map[string]bool{"weather": true}, boolPtr(false), false},
		{"frontmatter true enables when omitted from filter", "tavily", map[string]bool{"weather": true}, boolPtr(true), true},
		{"filter true overrides frontmatter false", "tavily", map[string]bool{"tavily": true}, boolPtr(false), true},
		{"filter false overrides frontmatter true", "tavily", map[string]bool{"tavily": false}, boolPtr(true), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSkillEnabled(tc.skill, tc.filter, tc.frontmatterEnabled); got != tc.want {
				t.Errorf("isSkillEnabled(%q) = %v, want %v", tc.skill, got, tc.want)
			}
		})
	}
}

func TestLoadSkillsFilter(t *testing.T) {
	t.Run("missing file returns nil", func(t *testing.T) {
		if got := loadSkillsFilter(t.TempDir()); got != nil {
			t.Errorf("expected nil filter, got %v", got)
		}
	})

	t.Run("valid file returns parsed map", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "skills.json"), []byte(`{"weather": true, "tavily": false}`), 0644); err != nil {
			t.Fatal(err)
		}
		filter := loadSkillsFilter(dir)
		if filter == nil {
			t.Fatal("expected non-nil filter")
		}
		if filter["weather"] != true {
			t.Errorf("weather should be enabled, got %v", filter["weather"])
		}
		if filter["tavily"] != false {
			t.Errorf("tavily should be disabled, got %v", filter["tavily"])
		}
	})

	t.Run("invalid file returns nil", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "skills.json"), []byte(`not valid json`), 0644); err != nil {
			t.Fatal(err)
		}
		if got := loadSkillsFilter(dir); got != nil {
			t.Errorf("expected nil filter, got %v", got)
		}
	})
}

func TestBuildSkillsInstruction(t *testing.T) {
	makeSkill := func(dir, name, description string) {
		frontmatter := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# body\n"
		if err := os.MkdirAll(filepath.Join(dir, name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "SKILL.md"), []byte(frontmatter), 0644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("no filter includes all skills", func(t *testing.T) {
		dir := t.TempDir()
		makeSkill(dir, "weather", "Get the current weather.")
		makeSkill(dir, "tavily", "Search the web.")

		out := buildSkillsInstruction(dir, nil)
		if !strings.Contains(out, "weather") || !strings.Contains(out, "tavily") {
			t.Errorf("expected both skills in output:\n%s", out)
		}
	})

	t.Run("disabled skill is skipped", func(t *testing.T) {
		dir := t.TempDir()
		makeSkill(dir, "weather", "Get the current weather.")
		makeSkill(dir, "tavily", "Search the web.")

		out := buildSkillsInstruction(dir, map[string]bool{"tavily": false})
		if !strings.Contains(out, "weather") {
			t.Errorf("expected weather in output:\n%s", out)
		}
		if strings.Contains(out, "tavily") {
			t.Errorf("tavily should be skipped:\n%s", out)
		}
	})

	t.Run("frontmatter enabled false skips skill", func(t *testing.T) {
		dir := t.TempDir()
		makeSkill(dir, "weather", "Get the current weather.")
		if err := os.MkdirAll(filepath.Join(dir, "tavily"), 0755); err != nil {
			t.Fatal(err)
		}
		frontmatter := "---\nname: Tavily\ndescription: Search the web.\nenabled: false\n---\n\n# body\n"
		if err := os.WriteFile(filepath.Join(dir, "tavily", "SKILL.md"), []byte(frontmatter), 0644); err != nil {
			t.Fatal(err)
		}

		out := buildSkillsInstruction(dir, nil)
		if !strings.Contains(out, "weather") {
			t.Errorf("expected weather in output:\n%s", out)
		}
		if strings.Contains(out, "tavily") {
			t.Errorf("tavily should be skipped:\n%s", out)
		}
	})

	t.Run("filter true overrides frontmatter enabled false", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "tavily"), 0755); err != nil {
			t.Fatal(err)
		}
		frontmatter := "---\nname: Tavily\ndescription: Search the web.\nenabled: false\n---\n\n# body\n"
		if err := os.WriteFile(filepath.Join(dir, "tavily", "SKILL.md"), []byte(frontmatter), 0644); err != nil {
			t.Fatal(err)
		}

		out := buildSkillsInstruction(dir, map[string]bool{"tavily": true})
		if !strings.Contains(out, "tavily") {
			t.Errorf("tavily should be included when explicitly enabled in filter:\n%s", out)
		}
	})

	t.Run("missing directory returns empty", func(t *testing.T) {
		out := buildSkillsInstruction(filepath.Join(t.TempDir(), "nope"), nil)
		if out != "" {
			t.Errorf("expected empty output, got %q", out)
		}
	})
}
