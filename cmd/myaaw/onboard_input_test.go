package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// Regression: text inputs used to swallow j, k, and space because those keys
// were handled as navigation before the input received them.
func TestTextInputAcceptsNavigationLetters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := initialOnboardModel("agent-b")
	m.step = stepProviderName
	m.resetInput("openai", false)

	for _, r := range "deepsek j" {
		next, _ := m.Update(runeKey(r))
		m = next.(onboardModel)
	}

	if got := m.textInput.Value(); got != "deepsek j" {
		t.Fatalf("input = %q, want %q", got, "deepsek j")
	}
	if m.cursor != 0 {
		t.Fatalf("cursor moved to %d while typing", m.cursor)
	}
}

func TestSingleChoiceStepsStillNavigateWithJandK(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := initialOnboardModel("agent-b")
	m.step = stepChannelChoice
	m.choices = []string{"Telegram", "Discord", "Both", "Skip"}
	m.cursor = 0

	next, _ := m.Update(runeKey('j'))
	m = next.(onboardModel)
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 after j", m.cursor)
	}

	next, _ = m.Update(runeKey('k'))
	m = next.(onboardModel)
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 after k", m.cursor)
	}
}

func TestMultiSelectStepsToggleWithSpace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := initialOnboardModel("agent-b")
	m.step = stepSkills
	m.choices = append([]string{}, m.skills...)
	if len(m.choices) == 0 {
		t.Fatal("no skills to toggle")
	}
	m.cursor = 0
	name := m.choices[0]

	next, _ := m.Update(runeKey(' '))
	m = next.(onboardModel)
	if !m.skillsSelected[name] {
		t.Fatalf("skill %q was not toggled on", name)
	}

	next, _ = m.Update(runeKey(' '))
	m = next.(onboardModel)
	if m.skillsSelected[name] {
		t.Fatalf("skill %q was not toggled off", name)
	}
}
