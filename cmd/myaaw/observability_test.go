package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"myaaw/internal/obs"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestParseTimeFlag(t *testing.T) {
	start, err := parseTimeFlag("2026-10-28", false)
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	wantStart := time.Date(2026, 10, 28, 0, 0, 0, 0, time.Local).UnixMilli()
	if start != wantStart {
		t.Fatalf("start = %d, want %d", start, wantStart)
	}

	end, err := parseTimeFlag("2026-10-28", true)
	if err != nil {
		t.Fatalf("parse end: %v", err)
	}
	wantEnd := time.Date(2026, 10, 28, 23, 59, 59, int(999*time.Millisecond), time.Local).UnixMilli()
	if end != wantEnd {
		t.Fatalf("end = %d, want the last millisecond of the day (%d)", end, wantEnd)
	}

	millis, err := parseTimeFlag("1700000000000", false)
	if err != nil || millis != 1_700_000_000_000 {
		t.Fatalf("unix millis = %d, err = %v", millis, err)
	}

	if zero, err := parseTimeFlag("", false); err != nil || zero != 0 {
		t.Fatalf("empty = %d, err = %v", zero, err)
	}
	if _, err := parseTimeFlag("28-10-2026", false); err == nil {
		t.Fatal("an ambiguous date must be rejected")
	}
}

func testPicker(runs []obs.Run) runPickerModel {
	items := make([]list.Item, 0, len(runs))
	for _, run := range runs {
		items = append(items, runItem{run: run})
	}
	picker := list.New(items, list.NewDefaultDelegate(), 80, 20)
	picker.SetFilteringEnabled(false)
	return runPickerModel{list: picker, chosen: -1}
}

func TestRunPickerMovesWithArrowKeys(t *testing.T) {
	runs := []obs.Run{{ID: "run_a"}, {ID: "run_b"}, {ID: "run_c"}}
	picker := testPicker(runs)

	updated, _ := picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	picker = updated.(runPickerModel)
	if picker.list.Index() != 1 {
		t.Fatalf("index after down = %d, want 1", picker.list.Index())
	}

	updated, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	picker = updated.(runPickerModel)
	if picker.list.Index() != 0 {
		t.Fatalf("index after up = %d, want 0", picker.list.Index())
	}

	updated, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	picker = updated.(runPickerModel)
	updated, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	picker = updated.(runPickerModel)

	if picker.chosen != 1 {
		t.Fatalf("chosen = %d, want the second run", picker.chosen)
	}
	if cmd == nil {
		t.Fatal("enter must quit the picker")
	}
	if runs[picker.chosen].ID != "run_b" {
		t.Fatalf("selected run = %q", runs[picker.chosen].ID)
	}
}

func TestRunPickerCancelsOnQuitKeys(t *testing.T) {
	for _, key := range []string{"q", "esc", "ctrl+c"} {
		picker := testPicker([]obs.Run{{ID: "run_a"}})
		updated, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		picker = updated.(runPickerModel)
		if key == "esc" {
			updated, cmd = picker.Update(tea.KeyMsg{Type: tea.KeyEsc})
			picker = updated.(runPickerModel)
		}
		if key == "ctrl+c" {
			updated, cmd = picker.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			picker = updated.(runPickerModel)
		}
		if cmd == nil {
			t.Fatalf("%s must quit the picker", key)
		}
		if picker.chosen != -1 {
			t.Fatalf("%s chose run %d, want no selection", key, picker.chosen)
		}
	}
}

func TestRunItemDescribesTheRun(t *testing.T) {
	item := runItem{run: obs.Run{
		ID: "run_a", Channel: "cli", Model: "deepseek-v4-flash",
		Status: obs.StatusCompleted, InputTokens: 9600, OutputTokens: 124,
	}}
	title := item.Title()
	if title == "" || item.Description() == "" {
		t.Fatalf("item = %q / %q", title, item.Description())
	}
	if item.FilterValue() == "" {
		t.Fatal("filter value must not be empty")
	}
}

func TestSessionItemDescribesTheSession(t *testing.T) {
	item := sessionItem{session: obs.Session{
		ConversationID: "4f1c8a2e", Channel: "cli", Turns: 4,
		LastActiveAt: 1_700_000_000_000, InputTokens: 1_000_000, OutputTokens: 7200,
	}}
	if item.Title() == "" || item.Description() == "" || item.FilterValue() != "4f1c8a2e" {
		t.Fatalf("item = %q / %q / %q", item.Title(), item.Description(), item.FilterValue())
	}
}

// Each command binds its own variable; a shared one would let the last
// registration overwrite another command's default.
func TestObservabilityFlagDefaultsAreIndependent(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"summary --days", obsSummaryDays, 7},
		{"prune --days", obsPruneDays, 0},
		{"runs --limit", obsRunsLimit, 20},
		{"sessions --limit", obsSessionsLimit, 50},
	}
	for _, testCase := range cases {
		if testCase.got != testCase.want {
			t.Fatalf("%s default = %d, want %d", testCase.name, testCase.got, testCase.want)
		}
	}
}

func TestFormatTurnRef(t *testing.T) {
	if got := formatTurnRef("4e7371da-c190-434e-90d3-e1902f309c82", "cli", 2); got != "4e7371da.cli.2" {
		t.Fatalf("ref = %q", got)
	}
	if got := formatTurnRef("abc", "", 1); got != "abc.agent.1" {
		t.Fatalf("ref without a channel = %q", got)
	}
}

func TestResolveTurnRef(t *testing.T) {
	store, err := obs.Open(filepath.Join(t.TempDir(), "observability.db"), obs.Options{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	runs := []obs.Run{
		{ID: "run_a1", StartedAt: 1000, ConversationID: "4e7371da-aaaa", Channel: "cli", Status: obs.StatusCompleted},
		{ID: "run_a2", StartedAt: 2000, ConversationID: "4e7371da-aaaa", Channel: "cli", Status: obs.StatusCompleted},
		{ID: "run_b1", StartedAt: 3000, ConversationID: "9f2a0b1c-bbbb", Channel: "telegram", Status: obs.StatusCompleted},
	}
	for _, run := range runs {
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
	}

	resolved, err := resolveTurnRef(store, "4e7371da.cli.2")
	if err != nil {
		t.Fatalf("resolve second turn: %v", err)
	}
	if resolved.ID != "run_a2" {
		t.Fatalf("resolved = %q, want run_a2", resolved.ID)
	}

	first, err := resolveTurnRef(store, "4e7371da.cli.1")
	if err != nil || first.ID != "run_a1" {
		t.Fatalf("first turn = %+v, err = %v", first, err)
	}

	if _, err := resolveTurnRef(store, "4e7371da.cli.3"); err == nil {
		t.Fatal("a turn beyond the session must fail")
	}
	if _, err := resolveTurnRef(store, "deadbeef.cli.1"); err == nil {
		t.Fatal("an unknown session prefix must fail")
	}
	for _, malformed := range []string{"nonsense", "4e7371da.cli", "4e7371da.cli.x", "4e7371da.cli.0"} {
		if _, err := resolveTurnRef(store, malformed); err == nil {
			t.Fatalf("%q must be rejected", malformed)
		}
	}
}

func TestTruncateValueKeepsColumnsAligned(t *testing.T) {
	if got := truncateValue("short", 10); got != "short" {
		t.Fatalf("short value = %q", got)
	}
	got := truncateValue("playwright_browser_take_screenshot", 20)
	if runes := []rune(got); len(runes) != 20 || runes[19] != '…' {
		t.Fatalf("truncated = %q (%d runes)", got, len(runes))
	}
	// A multi-byte name must not be split mid-character.
	if got := truncateValue("pengguna_üñî", 8); !utf8.ValidString(got) {
		t.Fatalf("truncation broke UTF-8: %q", got)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(out)
}

func TestPrintAssistantContentShowsThinkingAndText(t *testing.T) {
	raw := `{"role":"assistant","content":"Final answer.\nSecond line.","thought":"I should inspect the repo.\nThen run the tests."}`
	got := captureStdout(t, func() { printAssistantContent(raw, "    ") })

	if !strings.Contains(got, "thinking:") || !strings.Contains(got, "I should inspect the repo.") {
		t.Fatalf("thinking preview missing:\n%s", got)
	}
	if !strings.Contains(got, "text:") || !strings.Contains(got, "Final answer.") {
		t.Fatalf("text preview missing:\n%s", got)
	}
	if strings.Count(got, "(+1 more)") != 2 {
		t.Fatalf("each two-line block should be marked as truncated:\n%s", got)
	}
}

func TestPrintAssistantContentSkipsEmptyBlocks(t *testing.T) {
	got := captureStdout(t, func() {
		printAssistantContent(`{"role":"assistant","thought":"only thinking"}`, "  ")
	})
	if !strings.Contains(got, "thinking:") {
		t.Fatalf("thinking missing:\n%s", got)
	}
	if strings.Contains(got, "text:") {
		t.Fatalf("empty text block must not be printed:\n%s", got)
	}
}

func TestPrintAssistantContentIgnoresInvalidJSON(t *testing.T) {
	if got := captureStdout(t, func() { printAssistantContent("not json", "  ") }); got != "" {
		t.Fatalf("invalid json printed %q", got)
	}
}
