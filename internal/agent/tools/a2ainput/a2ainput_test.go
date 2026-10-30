package a2ainput

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolWritesInputRequiredMarker(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envTaskDir, dir)

	out := New().CallTool(`{"question":"Which city?"}`, nil)
	if !strings.Contains(out, "recorded") {
		t.Fatalf("output = %q", out)
	}
	data, err := os.ReadFile(filepath.Join(dir, inputRequiredFile))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	var payload struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal marker: %v", err)
	}
	if payload.Question != "Which city?" {
		t.Fatalf("question = %q", payload.Question)
	}
}

func TestToolRequiresTaskDir(t *testing.T) {
	t.Setenv(envTaskDir, "")
	out := New().CallTool(`{"question":"Which city?"}`, nil)
	if !strings.Contains(out, "only available") {
		t.Fatalf("output = %q", out)
	}
}

func TestToolRequiresQuestion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envTaskDir, dir)
	out := New().CallTool(`{}`, nil)
	if !strings.Contains(out, "question is required") {
		t.Fatalf("output = %q", out)
	}
}
