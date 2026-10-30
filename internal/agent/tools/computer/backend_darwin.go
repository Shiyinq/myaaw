//go:build darwin

package computer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// runCommand and lookPath are variables so tests can substitute them.
var (
	runCommand = func(ctx context.Context, name string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
		}
		return string(out), nil
	}
	lookPath = exec.LookPath
)

type darwinBackend struct {
	mu     sync.Mutex
	lastID string
}

func newBackend() Backend { return &darwinBackend{} }

func (b *darwinBackend) Screenshot(ctx context.Context) (Frame, error) {
	tmp, err := os.CreateTemp("", "myaaw-shot-*.png")
	if err != nil {
		return Frame{}, err
	}
	path := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(path)

	if _, err := runCommand(ctx, "screencapture", "-x", "-t", "png", path); err != nil {
		return Frame{}, fmt.Errorf("screencapture failed (grant Screen Recording permission): %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Frame{}, err
	}
	width, height := 0, 0
	if cfg, err := png.DecodeConfig(bytes.NewReader(data)); err == nil {
		width, height = cfg.Width, cfg.Height
	}
	if width > 1568 || height > 1568 {
		if _, err := lookPath("sips"); err == nil {
			if _, err := runCommand(ctx, "sips", "-Z", "1568", path); err == nil {
				if resized, err := os.ReadFile(path); err == nil {
					data = resized
					if cfg, err := png.DecodeConfig(bytes.NewReader(data)); err == nil {
						width, height = cfg.Width, cfg.Height
					}
				}
			}
		}
	}

	hash := sha256.Sum256(data)
	id := hex.EncodeToString(hash[:8])
	b.mu.Lock()
	unchanged := b.lastID == id
	b.lastID = id
	b.mu.Unlock()

	displayWidth, displayHeight := b.displaySize(ctx)
	return Frame{
		ID: id, PNG: data, Width: width, Height: height,
		Unchanged:     unchanged,
		DisplayWidth:  displayWidth,
		DisplayHeight: displayHeight,
	}, nil
}

// displaySize returns the logical desktop size in points via Finder bounds, so
// screenshot pixel coordinates can be mapped back to screen points.
func (b *darwinBackend) displaySize(ctx context.Context) (int, int) {
	out, err := runCommand(ctx, "osascript", "-e", `tell application "Finder" to get bounds of window of desktop`)
	if err != nil {
		return 0, 0
	}
	parts := strings.Split(strings.TrimSpace(out), ", ")
	if len(parts) < 4 {
		return 0, 0
	}
	width, errW := strconv.Atoi(strings.TrimSpace(parts[2]))
	height, errH := strconv.Atoi(strings.TrimSpace(parts[3]))
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return 0, 0
	}
	return width, height
}

// cliclickPaths are checked after PATH. A launchd daemon inherits a minimal
// PATH (/usr/bin:/bin:/usr/sbin:/sbin), so Homebrew's bin dir is not on it.
var cliclickPaths = []string{"/opt/homebrew/bin/cliclick", "/usr/local/bin/cliclick"}

// resolveCliclick finds cliclick through PATH and then the standard Homebrew
// locations, so the daemon can use it without a custom LaunchAgent PATH.
func resolveCliclick() (string, error) {
	if path, err := lookPath("cliclick"); err == nil {
		return path, nil
	}
	for _, candidate := range cliclickPaths {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: cliclick is not installed (brew install cliclick)", ErrUnsupportedAction)
}

func (b *darwinBackend) cliclick(ctx context.Context, args ...string) (string, error) {
	path, err := resolveCliclick()
	if err != nil {
		return "", err
	}
	return runCommand(ctx, path, args...)
}

func (b *darwinBackend) MouseMove(ctx context.Context, x, y int) error {
	_, err := b.cliclick(ctx, fmt.Sprintf("m:%d,%d", x, y))
	return err
}

func (b *darwinBackend) Click(ctx context.Context, x, y int, button string) error {
	switch strings.ToLower(button) {
	case "", "left":
		_, err := b.cliclick(ctx, fmt.Sprintf("c:%d,%d", x, y))
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrUnsupportedAction) {
			return err
		}
		// AppleScript fallback for a left click when cliclick is absent.
		script := fmt.Sprintf(`tell application "System Events" to click at {%d, %d}`, x, y)
		if _, fallbackErr := runCommand(ctx, "osascript", "-e", script); fallbackErr != nil {
			return fmt.Errorf("cliclick unavailable and AppleScript fallback failed: %w", fallbackErr)
		}
		return nil
	case "right":
		_, err := b.cliclick(ctx, fmt.Sprintf("rc:%d,%d", x, y))
		return err
	default:
		return fmt.Errorf("%w: button %q", ErrUnsupportedAction, button)
	}
}

func (b *darwinBackend) DoubleClick(ctx context.Context, x, y int) error {
	_, err := b.cliclick(ctx, fmt.Sprintf("dc:%d,%d", x, y))
	return err
}

func (b *darwinBackend) RightClick(ctx context.Context, x, y int) error {
	_, err := b.cliclick(ctx, fmt.Sprintf("rc:%d,%d", x, y))
	return err
}

func (b *darwinBackend) Drag(ctx context.Context, x1, y1, x2, y2 int) error {
	if _, err := b.cliclick(ctx, fmt.Sprintf("dd:%d,%d", x1, y1)); err != nil {
		return err
	}
	if _, err := b.cliclick(ctx, fmt.Sprintf("dm:%d,%d", x2, y2)); err != nil {
		return err
	}
	_, err := b.cliclick(ctx, fmt.Sprintf("du:%d,%d", x2, y2))
	return err
}

func (b *darwinBackend) Scroll(ctx context.Context, x, y, amount int, direction string) error {
	if amount <= 0 {
		amount = 3
	}
	code := "121" // page down
	if strings.EqualFold(direction, "up") {
		code = "116" // page up
	}
	for i := 0; i < amount; i++ {
		if _, err := runCommand(ctx, "osascript", "-e",
			fmt.Sprintf(`tell application "System Events" to key code %s`, code)); err != nil {
			return fmt.Errorf("scroll: %w", err)
		}
	}
	return nil
}

func (b *darwinBackend) Type(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}
	if _, err := b.cliclick(ctx, "t:"+text); err == nil {
		return nil
	}

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			if _, err := runCommand(ctx, "osascript", "-e", `tell application "System Events" to key code 36`); err != nil {
				return err
			}
		}
		if line == "" {
			continue
		}
		script := fmt.Sprintf(`tell application "System Events" to keystroke %s`, appleScriptString(line))
		if _, err := runCommand(ctx, "osascript", "-e", script); err != nil {
			return err
		}
	}
	return nil
}

func (b *darwinBackend) Key(ctx context.Context, keys string) error {
	if _, err := b.cliclick(ctx, "k:"+keys); err == nil {
		return nil
	}
	script, err := appleScriptKey(keys)
	if err != nil {
		return err
	}
	_, err = runCommand(ctx, "osascript", "-e", script)
	return err
}

func (b *darwinBackend) Wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *darwinBackend) ListWindows(ctx context.Context) ([]Window, error) {
	script := `tell application "System Events"
set out to ""
repeat with p in (every process whose background only is false)
  set appName to name of p
  try
    repeat with w in windows of p
      set wName to ""
      try
        set wName to name of w
      end try
      set out to out & appName & "|" & wName & linefeed
    end repeat
  on error
    set out to out & appName & "|" & linefeed
  end try
end repeat
return out
end tell`
	out, err := runCommand(ctx, "osascript", "-e", script)
	if err != nil {
		return b.listProcesses(ctx)
	}
	windows := parseWindowList(out)
	if len(windows) == 0 {
		return b.listProcesses(ctx)
	}
	return windows, nil
}

func (b *darwinBackend) listProcesses(ctx context.Context) ([]Window, error) {
	out, err := runCommand(ctx, "osascript", "-e",
		`tell application "System Events" to get name of every process whose background only is false`)
	if err != nil {
		return nil, err
	}
	names := strings.Split(strings.TrimSpace(out), ", ")
	windows := make([]Window, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		windows = append(windows, Window{App: name})
	}
	return windows, nil
}

func (b *darwinBackend) FocusApp(ctx context.Context, name string) error {
	script := fmt.Sprintf(`tell application "System Events" to set frontmost of process %s to true`, appleScriptString(name))
	if _, err := runCommand(ctx, "osascript", "-e", script); err == nil {
		return nil
	}
	// Fallback: no process with that exact name, try a window title match.
	titleScript := fmt.Sprintf(`tell application "System Events"
set found to false
repeat with p in (every process whose background only is false)
	try
		repeat with w in windows of p
			if name of w contains %s then
				set frontmost of p to true
				set found to true
				exit repeat
			end if
		end repeat
	end try
	if found then exit repeat
end repeat
return found
end tell`, appleScriptString(name))
	out, err := runCommand(ctx, "osascript", "-e", titleScript)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "true" {
		return nil
	}
	return fmt.Errorf("%w: no app or window matches %q", ErrUnsupportedAction, name)
}

func appleScriptString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

var appleScriptKeyCodes = map[string]string{
	"return": "36", "enter": "36", "tab": "48", "esc": "53", "escape": "53",
	"space": "49", "backspace": "51", "delete": "117",
	"up": "126", "down": "125", "left": "123", "right": "124",
	"home": "115", "end": "119", "pageup": "116", "pagedown": "121",
}

var appleScriptModifiers = map[string]string{
	"cmd": "command down", "command": "command down",
	"shift": "shift down", "ctrl": "control down", "control": "control down",
	"alt": "option down", "opt": "option down", "option": "option down",
}

func appleScriptKey(keys string) (string, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(keys)), "+")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return "", fmt.Errorf("%w: empty key", ErrUnsupportedAction)
	}
	key := parts[len(parts)-1]
	var modifiers []string
	for _, part := range parts[:len(parts)-1] {
		modifier, ok := appleScriptModifiers[strings.TrimSpace(part)]
		if !ok {
			return "", fmt.Errorf("%w: unknown modifier %q", ErrUnsupportedAction, part)
		}
		modifiers = append(modifiers, modifier)
	}

	if len([]rune(key)) == 1 {
		script := fmt.Sprintf(`tell application "System Events" to keystroke %s`, appleScriptString(key))
		if len(modifiers) > 0 {
			script += " using {" + strings.Join(modifiers, ", ") + "}"
		}
		return script, nil
	}
	code, ok := appleScriptKeyCodes[key]
	if !ok {
		return "", fmt.Errorf("%w: unknown key %q", ErrUnsupportedAction, key)
	}
	if len(modifiers) > 0 {
		return "", fmt.Errorf("%w: modifiers on %q require cliclick", ErrUnsupportedAction, key)
	}
	return fmt.Sprintf(`tell application "System Events" to key code %s`, code), nil
}
