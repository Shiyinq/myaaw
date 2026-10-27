package main

import (
	"strings"
	"testing"
)

func TestBuiltinDescriptions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := initialOnboardModel("")
	if len(m.skillDescriptions) == 0 {
		t.Fatal("no skill descriptions were loaded")
	}
	if len(m.toolDescriptions) == 0 {
		t.Fatal("no tool descriptions were loaded")
	}

	for _, name := range m.skills {
		if strings.TrimSpace(m.skillDescriptions[name]) == "" {
			t.Fatalf("skill %q has no description", name)
		}
	}
	for _, name := range m.tools {
		if strings.TrimSpace(m.toolDescriptions[name]) == "" {
			t.Fatalf("tool %q has no description", name)
		}
	}

	if !strings.Contains(strings.ToLower(m.skillDescriptions["time"]), "time") {
		t.Fatalf("time description looks wrong: %q", m.skillDescriptions["time"])
	}
	if !strings.Contains(strings.ToLower(m.toolDescriptions["bash"]), "bash") {
		t.Fatalf("bash description looks wrong: %q", m.toolDescriptions["bash"])
	}
}

func TestRenderMultiSelectShowsDescriptions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := initialOnboardModel("")
	m.cursor = 0

	m.step = stepSkills
	m.choices = []string{"time"}
	skillsView := m.renderMultiSelect(m.skillsSelected)
	if !strings.Contains(skillsView, "time") || !strings.Contains(strings.ToLower(skillsView), "timezone") {
		t.Fatalf("skills view is missing the description:\n%s", skillsView)
	}

	m.step = stepTools
	m.choices = []string{"bash"}
	toolsView := m.renderMultiSelect(m.toolsSelected)
	if !strings.Contains(toolsView, "bash") || !strings.Contains(strings.ToLower(toolsView), "bash command") {
		t.Fatalf("tools view is missing the description:\n%s", toolsView)
	}
}

func TestTruncateOneLine(t *testing.T) {
	long := "This is a deliberately long description that should be shortened with an ellipsis."
	got := truncateOneLine(long, 20)
	if len([]rune(got)) != 20 {
		t.Fatalf("length = %d, want 20 (%q)", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated text should end with an ellipsis: %q", got)
	}
	if short := truncateOneLine("short", 20); short != "short" {
		t.Fatalf("short text changed: %q", short)
	}
	if collapsed := truncateOneLine("two   spaces\nand a newline", 100); collapsed != "two spaces and a newline" {
		t.Fatalf("whitespace was not collapsed: %q", collapsed)
	}
}
