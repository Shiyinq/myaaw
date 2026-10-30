package computer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeBackend struct {
	frame    Frame
	err      error
	moves    []string
	clicks   []string
	clickPos []int
	typed    []string
	keys     []string
	focused  []string
	windows  []Window
	waited   time.Duration
	scrolled string
	drag     string
}

func (f *fakeBackend) Screenshot(context.Context) (Frame, error) { return f.frame, f.err }
func (f *fakeBackend) MouseMove(_ context.Context, x, y int) error {
	f.moves = append(f.moves, "move")
	return f.err
}
func (f *fakeBackend) Click(_ context.Context, x, y int, button string) error {
	f.clicks = append(f.clicks, button)
	f.clickPos = append(f.clickPos, x, y)
	return f.err
}
func (f *fakeBackend) DoubleClick(context.Context, int, int) error { return f.err }
func (f *fakeBackend) RightClick(context.Context, int, int) error  { return f.err }
func (f *fakeBackend) Drag(context.Context, int, int, int, int) error {
	f.drag = "drag"
	return f.err
}
func (f *fakeBackend) Scroll(_ context.Context, _, _, _ int, direction string) error {
	f.scrolled = direction
	return f.err
}
func (f *fakeBackend) Type(_ context.Context, text string) error {
	f.typed = append(f.typed, text)
	return f.err
}
func (f *fakeBackend) Key(_ context.Context, keys string) error {
	f.keys = append(f.keys, keys)
	return f.err
}
func (f *fakeBackend) Wait(_ context.Context, d time.Duration) error {
	f.waited = d
	return f.err
}
func (f *fakeBackend) ListWindows(context.Context) ([]Window, error) {
	return f.windows, f.err
}
func (f *fakeBackend) FocusApp(_ context.Context, name string) error {
	f.focused = append(f.focused, name)
	return f.err
}

func TestScreenshotReturnsImage(t *testing.T) {
	backend := &fakeBackend{frame: Frame{ID: "abc123", PNG: []byte("PNGDATA"), Width: 800, Height: 600}}
	tool := &ComputerTool{Backend: backend}

	result := tool.CallToolStructured(context.Background(), `{"action":"screenshot"}`, nil)
	if len(result.Images) != 1 || result.Images[0].MimeType != "image/png" {
		t.Fatalf("images = %+v", result.Images)
	}
	if !strings.Contains(result.Text, "800x600") || !strings.Contains(result.Text, "abc123") {
		t.Fatalf("text = %q", result.Text)
	}
	if len(result.Attachments) != 0 {
		t.Fatalf("attachments = %+v", result.Attachments)
	}
}

func TestScreenshotUnchangedHasNoImage(t *testing.T) {
	backend := &fakeBackend{frame: Frame{ID: "same", PNG: []byte("PNG"), Width: 1, Height: 1, Unchanged: true}}
	tool := &ComputerTool{Backend: backend}

	result := tool.CallToolStructured(context.Background(), `{"action":"screenshot"}`, nil)
	if len(result.Images) != 0 {
		t.Fatalf("images = %+v", result.Images)
	}
	if !strings.Contains(result.Text, "unchanged") {
		t.Fatalf("text = %q", result.Text)
	}
}

func TestScreenshotDeliverAddsAttachment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	backend := &fakeBackend{frame: Frame{ID: "frame01", PNG: []byte("PNGDATA"), Width: 10, Height: 10}}
	tool := &ComputerTool{Backend: backend}

	result := tool.CallToolStructured(context.Background(), `{"action":"screenshot","deliver":true}`, nil)
	if len(result.Attachments) != 1 || result.Attachments[0].FileName != "screenshot-frame01.png" {
		t.Fatalf("attachments = %+v", result.Attachments)
	}
	path := filepath.Join(home, ".myaaw", "default", "jobs", "captures", "screenshot-frame01.png")
	if data, err := os.ReadFile(path); err != nil || string(data) != "PNGDATA" {
		t.Fatalf("saved capture = %q, err=%v", data, err)
	}
}

func TestActionsDispatch(t *testing.T) {
	backend := &fakeBackend{windows: []Window{{App: "Safari", Title: "Start"}}}
	tool := &ComputerTool{Backend: backend}

	cases := []struct {
		args string
		want string
	}{
		{`{"action":"move","x":10,"y":20}`, "Moved"},
		{`{"action":"click","x":5,"y":6,"button":"right"}`, "Clicked right"},
		{`{"action":"double_click","x":1,"y":2}`, "Double-clicked"},
		{`{"action":"right_click","x":1,"y":2}`, "Right-clicked"},
		{`{"action":"drag","x":1,"y":2,"x2":3,"y2":4}`, "Dragged"},
		{`{"action":"scroll","x":1,"y":2,"amount":2,"direction":"up"}`, "Scrolled up"},
		{`{"action":"type","text":"hello"}`, "Typed 5"},
		{`{"action":"key","keys":"cmd+t"}`, "Pressed cmd+t"},
		{`{"action":"wait","duration":50}`, "Waited 50"},
		{`{"action":"list_windows"}`, "Safari"},
		{`{"action":"focus_app","window":"Safari"}`, "Focused Safari"},
	}
	for _, tc := range cases {
		result := tool.CallToolStructured(context.Background(), tc.args, nil)
		if !strings.Contains(result.Text, tc.want) {
			t.Errorf("%s → %q; want %q", tc.args, result.Text, tc.want)
		}
	}
	if len(backend.clicks) != 1 || backend.clicks[0] != "right" {
		t.Fatalf("clicks = %v", backend.clicks)
	}
	if len(backend.typed) != 1 || backend.typed[0] != "hello" {
		t.Fatalf("typed = %v", backend.typed)
	}
	if backend.waited != 50*time.Millisecond {
		t.Fatalf("waited = %v", backend.waited)
	}
}

func TestInvalidActions(t *testing.T) {
	tool := &ComputerTool{Backend: &fakeBackend{}}

	unknown := tool.CallToolStructured(context.Background(), `{"action":"explode"}`, nil)
	if !strings.Contains(unknown.Text, "COMPUTER_UNSUPPORTED_ACTION") {
		t.Fatalf("text = %q", unknown.Text)
	}
	missingKeys := tool.CallToolStructured(context.Background(), `{"action":"key"}`, nil)
	if !strings.Contains(missingKeys.Text, "COMPUTER_BAD_ARGUMENTS") {
		t.Fatalf("text = %q", missingKeys.Text)
	}
	missingWindow := tool.CallToolStructured(context.Background(), `{"action":"focus_app"}`, nil)
	if !strings.Contains(missingWindow.Text, "COMPUTER_BAD_ARGUMENTS") {
		t.Fatalf("text = %q", missingWindow.Text)
	}
}

func TestClickCoordinatesMapToScreenPoints(t *testing.T) {
	backend := &fakeBackend{frame: Frame{
		ID: "retina", PNG: []byte("PNG"), Width: 2880, Height: 1800,
		DisplayWidth: 1440, DisplayHeight: 900,
	}}
	tool := &ComputerTool{Backend: backend}

	if result := tool.CallToolStructured(context.Background(), `{"action":"screenshot"}`, nil); len(result.Images) != 1 {
		t.Fatalf("screenshot = %+v", result)
	}
	if result := tool.CallToolStructured(context.Background(), `{"action":"click","x":200,"y":100}`, nil); !strings.Contains(result.Text, "Clicked left at (100, 50)") {
		t.Fatalf("click text = %q", result.Text)
	}
	if len(backend.clickPos) != 2 || backend.clickPos[0] != 100 || backend.clickPos[1] != 50 {
		t.Fatalf("click position = %v", backend.clickPos)
	}
}

func TestParseWindowList(t *testing.T) {
	windows := parseWindowList("Safari|Start Page\nFinder|Downloads\nCalculator|\n")
	if len(windows) != 3 {
		t.Fatalf("windows = %+v", windows)
	}
	if windows[0].App != "Safari" || windows[0].Title != "Start Page" {
		t.Fatalf("windows[0] = %+v", windows[0])
	}
	if windows[2].App != "Calculator" || windows[2].Title != "" {
		t.Fatalf("windows[2] = %+v", windows[2])
	}
}

func TestFormatWindows(t *testing.T) {
	if got := formatWindows(nil); got != "No visible windows." {
		t.Fatalf("empty = %q", got)
	}
	got := formatWindows([]Window{{App: "Safari", Title: "Docs"}, {App: "Finder"}})
	if !strings.Contains(got, "Safari — Docs") || !strings.Contains(got, "- Finder") {
		t.Fatalf("windows = %q", got)
	}
}
