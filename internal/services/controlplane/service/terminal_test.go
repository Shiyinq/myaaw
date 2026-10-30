package service

import (
	"errors"
	"strings"
	"testing"
	"time"
)

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

	reader, err := terminals.Reader("myfriend", info.ID)
	if err != nil {
		t.Fatalf("Reader: %v", err)
	}

	output := make(chan string, 1)
	go func() {
		buffer := make([]byte, 4096)
		var seen strings.Builder
		for {
			read, readErr := reader.Read(buffer)
			if read > 0 {
				seen.Write(buffer[:read])
				if strings.Contains(seen.String(), "hello-terminal") {
					output <- seen.String()
					return
				}
			}
			if readErr != nil {
				output <- seen.String()
				return
			}
		}
	}()

	if err := terminals.Write("myfriend", info.ID, []byte("printf 'hello-terminal\\n'\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case got := <-output:
		if !strings.Contains(got, "hello-terminal") {
			t.Fatalf("output = %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for shell output")
	}

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
	if _, err := terminals.Reader("myfriend", "nope"); !errors.Is(err, ErrTerminalDisabled) {
		t.Fatalf("Reader error = %v", err)
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

	// Pretend the session sat silent past the idle timeout; a heartbeat from the
	// still-open tab has to save it from the sweep.
	terminals.mu.Lock()
	terminals.sessions[info.ID].lastUse = time.Now().Add(-3 * terminalIdleTimeout)
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
	terminals.sessions[info.ID].lastUse = time.Now().Add(-3 * terminalIdleTimeout)
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

func TestTerminalServiceRejectsUnknownAgent(t *testing.T) {
	t.Setenv("MYAAW_TERMINAL", "1")
	setupConfigAgent(t, nil)

	terminals := NewTerminalService()
	if _, err := terminals.Open("../escape", 80, 24); err == nil {
		t.Fatal("expected an error for an invalid agent name")
	}
}
