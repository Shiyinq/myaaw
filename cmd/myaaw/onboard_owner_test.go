package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/agentdir"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOwnerPromptsFollowConfiguredChannels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cases := []struct {
		name     string
		telegram bool
		discord  bool
		want     int
	}{
		{"none", false, false, stepSummary},
		{"telegram", true, false, stepOwnerTelegram},
		{"discord", false, true, stepOwnerDiscord},
		{"both", true, true, stepOwnerTelegram},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := initialOnboardModel("agent-b")
			if tc.telegram {
				m.telegramToken = "123:abc"
			}
			if tc.discord {
				m.discordToken = "discord-token"
			}
			m.setupOwnerInputs()
			if m.step != tc.want {
				t.Fatalf("step = %d, want %d", m.step, tc.want)
			}
		})
	}
}

func TestOwnerInputIsOptionalAndMerged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := initialOnboardModel("agent-b")
	m.telegramToken = "123:abc"
	m.discordToken = "discord-token"
	m.setupOwnerInputs()

	if m.step != stepOwnerTelegram {
		t.Fatalf("step = %d, want stepOwnerTelegram", m.step)
	}
	// Leave the Telegram owner empty on purpose.
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.step != stepOwnerDiscord {
		t.Fatalf("step = %d, want stepOwnerDiscord", m.step)
	}

	m.textInput.SetValue("222")
	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.step != stepSummary {
		t.Fatalf("step = %d, want stepSummary", m.step)
	}

	if got := m.ownerIDsAll(); len(got) != 1 || got[0] != "222" {
		t.Fatalf("owner IDs = %v, want [222]", got)
	}
	if got := m.input().OwnerIDs; len(got) != 1 || got[0] != "222" {
		t.Fatalf("input owners = %v, want [222]", got)
	}
}

func TestOwnerIDsAreMergedPerChannel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := initialOnboardModel("agent-b")
	m.telegramToken = "123:abc"
	m.discordToken = "discord-token"
	m.setupOwnerInputs()

	m.textInput.SetValue("111")
	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	m.textInput.SetValue("222, 111")

	next, _ = m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)

	owners := m.ownerIDsAll()
	if len(owners) != 2 || owners[0] != "111" || owners[1] != "222" {
		t.Fatalf("owner IDs = %v, want [111 222] with duplicates removed", owners)
	}
}

func TestOwnerInputRejectsNonNumeric(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := initialOnboardModel("agent-b")
	m.telegramToken = "123:abc"
	m.setupOwnerInputs()
	m.textInput.SetValue("not-an-id")

	next, _ := m.nextStep(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(onboardModel)
	if m.err == nil {
		t.Fatal("expected an error for a non-numeric owner ID")
	}
	if m.step != stepOwnerTelegram {
		t.Fatalf("step = %d, want to stay on stepOwnerTelegram", m.step)
	}
}

func TestInstallStepOnlyForFirstAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	fresh := initialOnboardModel("")
	if !fresh.firstAgent {
		t.Fatal("a machine without initialized agents should be treated as first")
	}
	if !strings.Contains(fresh.renderTimeline(), "Install") {
		t.Fatal("first agent should show the install step")
	}

	// Simulate an existing initialized agent.
	root, err := agentdir.AgentRoot("existing")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "config.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := agentdir.RegisterAgent("existing"); err != nil {
		t.Fatal(err)
	}

	next := initialOnboardModel("agent-b")
	if next.firstAgent {
		t.Fatal("a second agent must not be treated as the first")
	}
	if strings.Contains(next.renderTimeline(), "Install") {
		t.Fatal("later agents should not show the install step")
	}
	if got := next.afterSaveStep(); got != stepDone {
		t.Fatalf("afterSaveStep = %d, want stepDone", got)
	}
	next.restartPending = true
	if got := next.afterSaveStep(); got != stepRestart {
		t.Fatalf("afterSaveStep = %d, want stepRestart", got)
	}
}
