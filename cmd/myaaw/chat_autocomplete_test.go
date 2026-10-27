package main

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
)

func newChatAutocompleteModel(value string, cursor int) *model {
	ti := textinput.New()
	ti.SetValue(value)
	ti.SetCursor(cursor)
	return &model{textInput: ti}
}

// A path typed into chat starts with "/", so moving the cursor to position 0
// used to slice val[1:0] and panic, killing the whole TUI.
func TestUpdateAutocompleteAtStartOfAbsolutePathDoesNotPanic(t *testing.T) {
	m := newChatAutocompleteModel("/Users/shiyinq/.myaaw/default/home/workspace/downloads", 0)

	m.updateAutocomplete()

	if m.isAutocompleting || len(m.suggestions) != 0 {
		t.Fatalf("cursor at the start should not autocomplete: %v", m.suggestions)
	}
}

func TestUpdateAutocompleteSlashCommand(t *testing.T) {
	m := newChatAutocompleteModel("/", 1)

	m.updateAutocomplete()

	if !m.isAutocompleting || len(m.suggestions) == 0 {
		t.Fatalf("expected slash command suggestions, got %v", m.suggestions)
	}
}

func TestUpdateAutocompleteHandlesMultibyteInput(t *testing.T) {
	m := newChatAutocompleteModel("🐾 @dir/fi", 12)

	m.updateAutocomplete()

	if !m.isAutocompleting {
		t.Fatal("expected file autocomplete to stay active")
	}
}

func TestApplySuggestionKeepsCursorOnRuneBoundary(t *testing.T) {
	m := newChatAutocompleteModel("🐾 @fi", 8)
	m.suggestions = []string{"file.txt"}
	m.suggestionIndex = 0
	m.isAutocompleting = true

	m.applySuggestion()

	if got := m.textInput.Value(); got != "🐾 @file.txt " {
		t.Fatalf("value = %q", got)
	}
	if got := m.textInput.Position(); got != 12 {
		t.Fatalf("cursor = %d, want 12", got)
	}
}

func TestCursorByteOffset(t *testing.T) {
	cases := []struct {
		val  string
		pos  int
		want int
	}{
		{"", 0, 0},
		{"", 5, 0},
		{"abc", 0, 0},
		{"abc", 2, 2},
		{"abc", 3, 3},
		{"abc", 99, 3},
		{"🐾ab", 0, 0},
		{"🐾ab", 1, 4},
		{"🐾ab", 3, 6},
		{"🐾ab", 99, 6},
	}
	for _, c := range cases {
		if got := cursorByteOffset(c.val, c.pos); got != c.want {
			t.Errorf("cursorByteOffset(%q, %d) = %d, want %d", c.val, c.pos, got, c.want)
		}
	}
}

func TestRuneIndex(t *testing.T) {
	cases := []struct {
		val    string
		offset int
		want   int
	}{
		{"abc", -1, 0},
		{"abc", 0, 0},
		{"abc", 2, 2},
		{"abc", 99, 3},
		{"🐾ab", 4, 1},
		{"🐾ab", 6, 3},
		{"🐾ab", 99, 3},
	}
	for _, c := range cases {
		if got := runeIndex(c.val, c.offset); got != c.want {
			t.Errorf("runeIndex(%q, %d) = %d, want %d", c.val, c.offset, got, c.want)
		}
	}
}
