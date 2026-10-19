package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupLogAgent(t *testing.T) string {
	t.Helper()

	agentRoot := setupConfigAgent(t, nil)
	logsDir := filepath.Join(agentRoot, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logsDir, "app.log"), []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	return agentRoot
}

func TestLogsServiceListAndResolve(t *testing.T) {
	setupLogAgent(t)
	logs := NewLogsService()

	files, err := logs.List("myfriend")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 || files[0].Name != "app.log" {
		t.Fatalf("files = %+v", files)
	}

	if _, err := logs.Resolve("myfriend", "app.log"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for _, name := range []string{"", "../app.log", "sub/app.log", `sub\app.log`} {
		if _, err := logs.Resolve("myfriend", name); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("Resolve(%q) err = %v, want ErrInvalidPath", name, err)
		}
	}
	if _, err := logs.Resolve("myfriend", "missing.log"); !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("err = %v, want ErrPathNotFound", err)
	}
}

func TestLogsServiceTail(t *testing.T) {
	agentRoot := setupLogAgent(t)
	logs := NewLogsService()

	logPath, err := logs.Resolve("myfriend", "app.log")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lines := make(chan string, 10)
	go func() {
		_ = logs.Tail(ctx, logPath, func(line string) error {
			lines <- line
			return nil
		})
	}()

	readLine := func() string {
		t.Helper()
		select {
		case line := <-lines:
			return line
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for log line")
			return ""
		}
	}

	if got := readLine(); got != "line1" {
		t.Fatalf("first line = %q", got)
	}
	if got := readLine(); got != "line2" {
		t.Fatalf("second line = %q", got)
	}

	file, err := os.OpenFile(filepath.Join(agentRoot, "logs", "app.log"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	if _, err := file.WriteString("line3\n"); err != nil {
		t.Fatalf("append log: %v", err)
	}
	file.Close()

	if got := readLine(); got != "line3" {
		t.Fatalf("third line = %q", got)
	}
}
