package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/agentdir"

	tea "github.com/charmbracelet/bubbletea"
)

// seedExistingAgent creates an initialized agent with a provider, owner, and
// tools.json so update-mode behaviour can be tested.
func seedExistingAgent(t *testing.T, name string) {
	t.Helper()
	root, err := agentdir.AgentRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{
  "default_provider": "openai",
  "providers": {"openai": {"type": "openai", "api_key": "sk-existing", "default_model": "gpt-4o"}},
  "bot": {"type": "private", "owner_ids": ["42"]}
}
`
	if err := os.WriteFile(filepath.Join(root, "config", "config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := agentdir.RegisterAgent(name); err != nil {
		t.Fatal(err)
	}
	if err := saveBuiltinToolSelection(name, []string{"bash"}); err != nil {
		t.Fatal(err)
	}
}

func TestSkipOptionsKeepExistingData(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	seedExistingAgent(t, "agent-skip")

	m := initialOnboardModel("agent-skip")
	if !m.agentExistedAtStart {
		t.Fatal("agent should be detected as existing")
	}

	// Owner IDs from config are pre-filled so Enter keeps them.
	if len(m.telegramOwnerIDs) != 1 || m.telegramOwnerIDs[0] != "42" {
		t.Fatalf("owner IDs were not pre-filled: %v", m.telegramOwnerIDs)
	}

	m.step = stepProviderType
	m.choices = m.providerTypeChoices()
	m.cursor = 0
	if m.choices[0] != skipProviderOption {
		t.Fatalf("provider choices = %v, want a keep-current option first", m.choices)
	}
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if !m.skipProvider || m.step != stepSkills {
		t.Fatalf("step = %d skipProvider = %v, want skills with provider skipped", m.step, m.skipProvider)
	}

	if len(m.choices) == 0 || m.choices[0] != skipSectionOption {
		t.Fatalf("skill choices = %v, want a skip option first", m.choices)
	}
	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if !m.skipSkills || m.step != stepTools {
		t.Fatalf("step = %d skipSkills = %v, want tools with skills skipped", m.step, m.skipSkills)
	}

	if len(m.choices) == 0 || m.choices[0] != skipSectionOption {
		t.Fatalf("tool choices = %v, want a skip option first", m.choices)
	}
	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if !m.skipTools || m.step != stepChannelChoice {
		t.Fatalf("step = %d skipTools = %v, want channels with tools skipped", m.step, m.skipTools)
	}

	in := m.input()
	if in.ProviderName != "" {
		t.Fatalf("provider name = %q, want empty so the provider is untouched", in.ProviderName)
	}
	if in.Skills != nil {
		t.Fatalf("skills = %v, want nil so no skill is installed", in.Skills)
	}
	if in.UpdateTools {
		t.Fatal("tools.json should not be rewritten when the tools step is skipped")
	}

	summary := m.renderSummary()
	for _, want := range []string{"unchanged (current)"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary is missing %q:\n%s", want, summary)
		}
	}
}

func TestApplyOnboardingSkipsToolRewrite(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	seedExistingAgent(t, "agent-keep-tools")

	// A skipped tools step must leave tools.json byte-for-byte untouched.
	toolsDir, err := agentdir.ToolsDir("agent-keep-tools")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(toolsDir, "tools.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := applyOnboarding("agent-keep-tools", onboardInput{UpdateTools: false}, 0); err != nil {
		t.Fatalf("applyOnboarding: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("tools.json changed even though the tools step was skipped")
	}

	var parsed struct {
		BuiltinTools map[string]bool `json:"builtin_tools"`
	}
	if err := json.Unmarshal(after, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.BuiltinTools["bash"] {
		t.Fatal("bash should still be disabled")
	}
}

func TestProviderSkipKeepsConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	seedExistingAgent(t, "agent-provider")

	base := loadAgentConfig("agent-provider")
	merged := mergeOnboardConfig(base, onboardInput{ProviderName: ""})
	if merged.DefaultProvider != "openai" {
		t.Fatalf("default provider = %q, want openai", merged.DefaultProvider)
	}
	if merged.Providers["openai"].APIKey != "sk-existing" {
		t.Fatalf("provider was overwritten: %+v", merged.Providers["openai"])
	}
}
