package service

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func waitForOutput(t *testing.T, stream *TerminalStream, marker string) string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	var seen strings.Builder
	for {
		select {
		case chunk, ok := <-stream.Output():
			if !ok {
				t.Fatalf("stream closed while waiting for %q (saw %q)", marker, seen.String())
			}
			seen.Write(chunk)
			if strings.Contains(seen.String(), marker) {
				return seen.String()
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q (saw %q)", marker, seen.String())
		}
	}
}

func TestTerminalServiceRoundTrip(t *testing.T) {
	t.Setenv("MYAAW_TERMINAL", "1")
	t.Setenv("SHELL", "/bin/sh")
	setupConfigAgent(t, nil)

	terminals := NewTerminalService()
	if !terminals.Enabled() {
		t.Fatal("terminal should be enabled")
	}

	info, err := terminals.Open("myfriend", 100, 30)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if info.ID == "" || !strings.HasSuffix(info.Cwd, "/home") {
		t.Fatalf("info = %+v", info)
	}
	defer terminals.Close("myfriend", info.ID)

	stream, err := terminals.Attach("myfriend", info.ID)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer stream.Close()

	if err := terminals.Write("myfriend", info.ID, []byte("printf 'hello-terminal\\n'\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	waitForOutput(t, stream, "hello-terminal")

	if err := terminals.Resize("myfriend", info.ID, 120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	if err := terminals.Close("myfriend", info.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for terminals.Count() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if terminals.Count() != 0 {
		t.Fatalf("sessions = %d, want 0", terminals.Count())
	}

	// A closed session is gone for good.
	if err := terminals.Write("myfriend", info.ID, []byte("x")); !errors.Is(err, ErrTerminalNotFound) {
		t.Fatalf("write after close = %v", err)
	}
}

func TestTerminalServiceDisabled(t *testing.T) {
	t.Setenv("MYAAW_TERMINAL", "")
	setupConfigAgent(t, nil)

	terminals := NewTerminalService()
	if terminals.Enabled() {
		t.Fatal("terminal should be disabled by default")
	}
	if _, err := terminals.Open("myfriend", 80, 24); !errors.Is(err, ErrTerminalDisabled) {
		t.Fatalf("Open error = %v", err)
	}
	if _, err := terminals.Attach("myfriend", "nope"); !errors.Is(err, ErrTerminalDisabled) {
		t.Fatalf("Attach error = %v", err)
	}
}

func TestTerminalServiceSessionLimit(t *testing.T) {
	t.Setenv("MYAAW_TERMINAL", "1")
	t.Setenv("SHELL", "/bin/sh")
	setupConfigAgent(t, nil)

	terminals := NewTerminalService()
	for index := 0; index < maxSessionsPerAgent; index++ {
		info, err := terminals.Open("myfriend", 80, 24)
		if err != nil {
			t.Fatalf("Open %d: %v", index, err)
		}
		defer terminals.Close("myfriend", info.ID)
		// A session the dashboard is streaming is not evictable, which is what
		// makes the limit climb.
		stream, err := terminals.Attach("myfriend", info.ID)
		if err != nil {
			t.Fatalf("Attach %d: %v", index, err)
		}
		defer stream.Close()
	}

	if _, err := terminals.Open("myfriend", 80, 24); !errors.Is(err, ErrTerminalLimit) {
		t.Fatalf("Open beyond limit = %v", err)
	}
}

func TestTerminalServiceTouchKeepsSessionAlive(t *testing.T) {
	t.Setenv("MYAAW_TERMINAL", "1")
	t.Setenv("SHELL", "/bin/sh")
	setupConfigAgent(t, nil)

	terminals := NewTerminalService()
	info, err := terminals.Open("myfriend", 80, 24)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer terminals.Close("myfriend", info.ID)

	// Pretend the session sat silent past the grace window; a heartbeat from the
	// still-open tab has to save it from the sweep.
	terminals.mu.Lock()
	terminals.sessions[info.ID].lastUse = time.Now().Add(-3 * terminalGrace)
	terminals.mu.Unlock()

	if err := terminals.Touch("myfriend", info.ID); err != nil {
		t.Fatalf("Touch: %v", err)
	}

	second, err := terminals.Open("myfriend", 80, 24)
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	defer terminals.Close("myfriend", second.ID)

	if terminals.Count() != 2 {
		t.Fatalf("sessions = %d, want 2", terminals.Count())
	}
}

func TestTerminalServiceReapsAbandonedSession(t *testing.T) {
	t.Setenv("MYAAW_TERMINAL", "1")
	t.Setenv("SHELL", "/bin/sh")
	setupConfigAgent(t, nil)

	terminals := NewTerminalService()
	info, err := terminals.Open("myfriend", 80, 24)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	terminals.mu.Lock()
	terminals.sessions[info.ID].lastUse = time.Now().Add(-3 * terminalGrace)
	terminals.mu.Unlock()

	second, err := terminals.Open("myfriend", 80, 24)
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	defer terminals.Close("myfriend", second.ID)

	if terminals.Count() != 1 {
		t.Fatalf("sessions = %d, want 1", terminals.Count())
	}
	if err := terminals.Touch("myfriend", info.ID); !errors.Is(err, ErrTerminalNotFound) {
		t.Fatalf("touch of reaped session = %v", err)
	}
}

func TestTerminalServiceDetachAllowsReattach(t *testing.T) {
	t.Setenv("MYAAW_TERMINAL", "1")
	t.Setenv("SHELL", "/bin/sh")
	setupConfigAgent(t, nil)

	terminals := NewTerminalService()
	info, err := terminals.Open("myfriend", 80, 24)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer terminals.Close("myfriend", info.ID)

	first, err := terminals.Attach("myfriend", info.ID)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// Navigating away ends the stream but must not end the shell.
	first.Close()

	described, err := terminals.Info("myfriend", info.ID)
	if err != nil {
		t.Fatalf("Info after detach: %v", err)
	}
	if described.ID != info.ID || described.Shell != info.Shell {
		t.Fatalf("info = %+v, want %+v", described, info)
	}

	// And it can be streamed again.
	second, err := terminals.Attach("myfriend", info.ID)
	if err != nil {
		t.Fatalf("reattach: %v", err)
	}
	defer second.Close()
	if terminals.Count() != 1 {
		t.Fatalf("sessions = %d, want 1", terminals.Count())
	}
}

func TestTerminalServiceAttachSupersedesOldStream(t *testing.T) {
	t.Setenv("MYAAW_TERMINAL", "1")
	t.Setenv("SHELL", "/bin/sh")
	setupConfigAgent(t, nil)

	terminals := NewTerminalService()
	info, err := terminals.Open("myfriend", 80, 24)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer terminals.Close("myfriend", info.ID)

	stale, err := terminals.Attach("myfriend", info.ID)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// A reload attaches again, which has to cut the reader the old page left
	// blocked on Read. Leaving both alive is what makes output disappear.
	if _, err := terminals.Attach("myfriend", info.ID); err != nil {
		t.Fatalf("Attach second: %v", err)
	}

	select {
	case chunk, ok := <-stale.Output():
		if ok {
			t.Fatalf("stale stream still received %q", chunk)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stale stream was not closed by the newer attach")
	}
}

func TestTerminalServiceReplaysDetachedOutput(t *testing.T) {
	t.Setenv("MYAAW_TERMINAL", "1")
	t.Setenv("SHELL", "/bin/sh")
	setupConfigAgent(t, nil)

	terminals := NewTerminalService()
	info, err := terminals.Open("myfriend", 80, 24)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer terminals.Close("myfriend", info.ID)

	stream, err := terminals.Attach("myfriend", info.ID)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	stream.Close()

	// Output produced while nobody is watching is kept for the next attach, so a
	// reload does not lose what the shell printed in the meantime.
	if err := terminals.Write("myfriend", info.ID, []byte("printf 'while-away\\n'\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	again, err := terminals.Attach("myfriend", info.ID)
	if err != nil {
		t.Fatalf("reattach: %v", err)
	}
	defer again.Close()
	waitForOutput(t, again, "while-away")
}

func TestTerminalServiceEvictsDetachedSessionAtLimit(t *testing.T) {
	t.Setenv("MYAAW_TERMINAL", "1")
	t.Setenv("SHELL", "/bin/sh")
	setupConfigAgent(t, nil)

	terminals := NewTerminalService()
	first, err := terminals.Open("myfriend", 80, 24)
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	defer terminals.Close("myfriend", first.ID)
	second, err := terminals.Open("myfriend", 80, 24)
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	defer terminals.Close("myfriend", second.ID)

	// Both shells are being watched, so a third connection is refused.
	firstStream, err := terminals.Attach("myfriend", first.ID)
	if err != nil {
		t.Fatalf("Attach first: %v", err)
	}
	secondStream, err := terminals.Attach("myfriend", second.ID)
	if err != nil {
		t.Fatalf("Attach second: %v", err)
	}
	defer secondStream.Close()

	if _, err := terminals.Open("myfriend", 80, 24); !errors.Is(err, ErrTerminalLimit) {
		t.Fatalf("open while both are attached = %v", err)
	}

	// A detached shell yields its slot to a new connection.
	firstStream.Close()
	third, err := terminals.Open("myfriend", 80, 24)
	if err != nil {
		t.Fatalf("Open third: %v", err)
	}
	defer terminals.Close("myfriend", third.ID)

	if _, err := terminals.Info("myfriend", first.ID); !errors.Is(err, ErrTerminalNotFound) {
		t.Fatalf("evicted session lookup = %v", err)
	}
	if err := terminals.Touch("myfriend", second.ID); err != nil {
		t.Fatalf("attached session must survive eviction: %v", err)
	}
}

func TestTerminalServiceRejectsUnknownAgent(t *testing.T) {
	t.Setenv("MYAAW_TERMINAL", "1")
	setupConfigAgent(t, nil)

	terminals := NewTerminalService()
	if _, err := terminals.Open("../escape", 80, 24); err == nil {
		t.Fatal("expected an error for an invalid agent name")
	}
}
