package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/agentdir"

	tea "github.com/charmbracelet/bubbletea"
)

// writeRawConfig stores a config.json for an agent that is then registered, so
// the wizard sees it as an existing agent.
func writeRawConfig(t *testing.T, name, contents string) {
	t.Helper()
	root, err := agentdir.AgentRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "config.json"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := agentdir.RegisterAgent(name); err != nil {
		t.Fatal(err)
	}
}

// TestSkillsAndToolsPrefilledWhenAgentPickedInWizard covers the update flow
// where the agent is chosen from the welcome list instead of --agent. The
// skip options and the pre-filled selections must both appear there too.
func TestSkillsAndToolsPrefilledWhenAgentPickedInWizard(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	seedExistingAgent(t, "agent-pick")

	skillsDir, err := agentdir.SkillsDir("agent-pick")
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range []string{"calendar", "skill-creator"} {
		if err := os.MkdirAll(filepath.Join(skillsDir, skill), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	m := initialOnboardModel("")
	if m.agentExistedAtStart {
		t.Fatal("a wizard without --agent must not assume an existing agent")
	}
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.step != stepAgentChoice {
		t.Fatalf("step = %d, want stepAgentChoice", m.step)
	}
	cursor := -1
	for i, choice := range m.choices {
		if choice == "agent-pick" {
			cursor = i
			break
		}
	}
	if cursor < 0 {
		t.Fatalf("existing agent missing from %v", m.choices)
	}
	m.cursor = cursor
	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)

	if !m.agentExistedAtStart {
		t.Fatal("picking an existing agent should switch to update mode")
	}
	if !m.skillsSelected["skill-creator"] || !m.skillsSelected["calendar"] {
		t.Fatalf("installed skills are not pre-selected: %v", m.skillsSelected)
	}
	if m.skillsSelected["weather"] {
		t.Fatalf("skills that are not installed must stay unchecked: %v", m.skillsSelected)
	}
	if m.toolsSelected["bash"] {
		t.Fatalf("a tool disabled in tools.json was selected again: %v", m.toolsSelected)
	}
	if !m.toolsSelected["filesystem"] {
		t.Fatalf("enabled tools must stay selected: %v", m.toolsSelected)
	}
	if len(m.skillChoices()) == 0 || m.skillChoices()[0] != keepSkillsOption {
		t.Fatalf("skill choices = %v, want a keep-current option first", m.skillChoices())
	}
	if len(m.toolChoices()) == 0 || m.toolChoices()[0] != keepToolsOption {
		t.Fatalf("tool choices = %v, want a keep-current option first", m.toolChoices())
	}
}

const savedChannelConfig = `{
  "default_provider": "openai",
  "providers": {"openai": {"type": "openai", "api_key": "sk-existing", "default_model": "gpt-4o"}},
  "channels": {"telegram": {"active": true, "token": "123:abc"}},
  "bot": {"type": "private", "owner_ids": ["42", "43"]}
}
`

// TestOwnerStepUsesSavedChannels ensures an update that skips the channel
// section still offers the owner step, because the channel is already wired.
func TestOwnerStepUsesSavedChannels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-saved-channel", savedChannelConfig)

	m := initialOnboardModel("agent-saved-channel")
	if !m.agentExistedAtStart {
		t.Fatal("agent should be detected as existing")
	}
	// The channel section was skipped, so no token was typed in this run.
	m.setupOwnerInputs()
	if m.step != stepOwnerTelegram {
		t.Fatalf("step = %d, want stepOwnerTelegram for the saved Telegram channel", m.step)
	}
	if got := m.textInput.Value(); got != "42, 43" {
		t.Fatalf("owner input = %q, want the saved IDs", got)
	}

	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.step != stepSummary {
		t.Fatalf("step = %d, want stepSummary when only Telegram is configured", m.step)
	}
	in := m.input()
	if !in.OwnersSet || len(in.OwnerIDs) != 2 {
		t.Fatalf("owners = %v (set=%v), want the saved IDs", in.OwnerIDs, in.OwnersSet)
	}
	summary := m.renderSummary()
	if !strings.Contains(summary, "Telegram") || !strings.Contains(summary, "42, 43") {
		t.Fatalf("summary does not show the saved owners:\n%s", summary)
	}
}

// TestOwnerStepsCoverBothSavedChannels checks the per-channel order when both
// channels were already configured and the user answers both owner prompts.
func TestOwnerStepsCoverBothSavedChannels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-both-channels", `{
  "channels": {
    "telegram": {"active": true, "token": "123:abc"},
    "discord": {"active": true, "token": "dc-token"}
  },
  "bot": {"type": "private", "owner_ids": ["42"]}
}
`)

	m := initialOnboardModel("agent-both-channels")
	m.setupOwnerInputs()
	if m.step != stepOwnerTelegram {
		t.Fatalf("step = %d, want stepOwnerTelegram first", m.step)
	}
	m.textInput.SetValue("42, 99")
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.step != stepOwnerDiscord {
		t.Fatalf("step = %d, want stepOwnerDiscord for the saved Discord channel", m.step)
	}
	m.textInput.SetValue("")
	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.step != stepSummary {
		t.Fatalf("step = %d, want stepSummary", m.step)
	}
	in := m.input()
	if got := strings.Join(in.OwnerIDs, ","); got != "42,99" {
		t.Fatalf("owners = %q, want the Telegram answer to survive the empty Discord prompt", got)
	}
}

// TestOwnerWarningOnlyAppliesToChannels keeps the review warning scoped: it
// must mention Telegram/Discord only and stay hidden when no channel is set up.
func TestOwnerWarningOnlyAppliesToChannels(t *testing.T) {
	cases := []struct {
		name     string
		config   string
		wantWarn bool
	}{
		{
			name:     "no channel",
			config:   `{"bot": {"type": "private"}}`,
			wantWarn: false,
		},
		{
			name:     "telegram without owners",
			config:   `{"channels": {"telegram": {"active": true, "token": "123:abc"}}, "bot": {"type": "private"}}`,
			wantWarn: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			if err := agentdir.EnsureLayout(); err != nil {
				t.Fatal(err)
			}
			writeRawConfig(t, "agent-warn", tc.config)

			summary := initialOnboardModel("agent-warn").renderSummary()
			if got := strings.Contains(summary, "No owner IDs"); got != tc.wantWarn {
				t.Fatalf("warning shown = %v, want %v:\n%s", got, tc.wantWarn, summary)
			}
			if !tc.wantWarn {
				return
			}
			for _, want := range []string{"Telegram and Discord", "Terminal", "REST API", "A2A"} {
				if !strings.Contains(summary, want) {
					t.Fatalf("warning does not mention %q:\n%s", want, summary)
				}
			}
		})
	}
}

// TestOwnerIDsCanBeCleared makes sure deleting every ID in the owner step
// actually removes the saved list, while an untouched run keeps it.
func TestOwnerIDsCanBeCleared(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	writeRawConfig(t, "agent-clear-owner", savedChannelConfig)

	base := loadAgentConfig("agent-clear-owner")
	if base == nil {
		t.Fatal("config was not loaded")
	}

	cleared := mergeOnboardConfig(base, onboardInput{OwnersSet: true})
	if len(cleared.Bot.OwnerIDs) != 0 {
		t.Fatalf("owner IDs = %v, want them cleared", cleared.Bot.OwnerIDs)
	}

	kept := mergeOnboardConfig(base, onboardInput{})
	if len(kept.Bot.OwnerIDs) != 2 {
		t.Fatalf("owner IDs = %v, want the saved list kept", kept.Bot.OwnerIDs)
	}
	if len(base.Bot.OwnerIDs) != 2 {
		t.Fatalf("merge mutated the input config: %v", base.Bot.OwnerIDs)
	}
}
