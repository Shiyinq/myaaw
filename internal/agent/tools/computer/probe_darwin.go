//go:build darwin

package computer

import (
	"context"
	"fmt"
	"os"
)

// Probe checks whether desktop automation can run on this machine. It is
// non-destructive: no pointer or keyboard input is sent, and the single test
// screenshot it takes is deleted immediately.
func Probe(ctx context.Context) []ProbeResult {
	results := make([]ProbeResult, 0, 6)

	results = append(results, ProbeResult{Name: "Operating system", OK: true, Detail: "macOS (darwin backend)"})

	if path, err := resolveCliclick(); err == nil {
		results = append(results, ProbeResult{Name: "cliclick", OK: true, Detail: path})
	} else {
		results = append(results, ProbeResult{
			Name:   "cliclick",
			OK:     false,
			Detail: "not installed",
			Fix:    "brew install cliclick. Without it, move/right_click/double_click/drag fail and click/type/key fall back to AppleScript with reduced coverage.",
		})
	}

	if _, err := lookPath("screencapture"); err != nil {
		results = append(results, ProbeResult{
			Name:   "screencapture",
			OK:     false,
			Detail: fmt.Sprintf("not found: %v", err),
			Fix:    "screencapture ships with macOS; this is unexpected.",
		})
	} else if ok, detail := probeScreenRecording(ctx); ok {
		results = append(results, ProbeResult{Name: "Screen Recording permission", OK: true, Detail: detail})
	} else {
		results = append(results, ProbeResult{
			Name:   "Screen Recording permission",
			OK:     false,
			Detail: detail,
			Fix:    "System Settings → Privacy & Security → Screen Recording → enable the app that launched this command, then quit and reopen that app (macOS applies screen-capture grants on relaunch).",
		})
	}

	if _, err := runCommand(ctx, "osascript", "-e", `tell application "System Events" to get name of first process`); err != nil {
		results = append(results, ProbeResult{
			Name:   "System Events access",
			OK:     false,
			Detail: fmt.Sprintf("System Events query failed: %v", err),
			Fix:    "System Settings → Privacy & Security → Automation → [your terminal] → enable System Events (fixes error -1743), and enable the same app under Accessibility for cliclick input.",
		})
	} else {
		results = append(results, ProbeResult{Name: "System Events access", OK: true, Detail: "Automation to System Events works"})
	}

	if w, h := (&darwinBackend{}).displaySize(ctx); w > 0 && h > 0 {
		results = append(results, ProbeResult{Name: "Desktop size", OK: true, Detail: fmt.Sprintf("%dx%d points", w, h)})
	}

	return results
}

// probeScreenRecording captures one screenshot to a temp file and deletes it.
func probeScreenRecording(ctx context.Context) (bool, string) {
	tmp, err := os.CreateTemp("", "myaaw-doctor-*.png")
	if err != nil {
		return false, fmt.Sprintf("cannot create temp file: %v", err)
	}
	path := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(path)

	if _, err := runCommand(ctx, "screencapture", "-x", "-t", "png", path); err != nil {
		return false, fmt.Sprintf("screencapture failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return false, "screencapture produced an empty image"
	}
	return true, fmt.Sprintf("screencapture works (%d bytes)", info.Size())
}
