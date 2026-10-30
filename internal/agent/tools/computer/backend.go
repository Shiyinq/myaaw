package computer

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	// ErrUnsupportedPlatform is returned when desktop automation is not available
	// on the current OS.
	ErrUnsupportedPlatform = errors.New("COMPUTER_UNSUPPORTED_PLATFORM")
	// ErrUnsupportedAction is returned when the backend cannot perform an action.
	ErrUnsupportedAction = errors.New("COMPUTER_UNSUPPORTED_ACTION")
)

// Frame is one screen capture.
type Frame struct {
	ID        string
	PNG       []byte
	Width     int
	Height    int
	Unchanged bool
	// DisplayWidth/DisplayHeight are the logical (point) screen size. They
	// differ from Width/Height on Retina displays and after downscaling.
	DisplayWidth  int
	DisplayHeight int
}

// Window describes one visible application window.
type Window struct {
	App   string
	Title string
}

// parseWindowList parses the AppleScript output, one "app|title" per line.
func parseWindowList(output string) []Window {
	var windows []Window
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		app, title, found := strings.Cut(line, "|")
		if !found {
			app = line
		}
		windows = append(windows, Window{App: strings.TrimSpace(app), Title: strings.TrimSpace(title)})
	}
	return windows
}

// Backend performs desktop actions. The macOS implementation is the reference;
// other platforms return ErrUnsupportedPlatform.
type Backend interface {
	Screenshot(ctx context.Context) (Frame, error)
	MouseMove(ctx context.Context, x, y int) error
	Click(ctx context.Context, x, y int, button string) error
	DoubleClick(ctx context.Context, x, y int) error
	RightClick(ctx context.Context, x, y int) error
	Drag(ctx context.Context, x1, y1, x2, y2 int) error
	Scroll(ctx context.Context, x, y, amount int, direction string) error
	Type(ctx context.Context, text string) error
	Key(ctx context.Context, keys string) error
	Wait(ctx context.Context, d time.Duration) error
	ListWindows(ctx context.Context) ([]Window, error)
	FocusApp(ctx context.Context, name string) error
}
