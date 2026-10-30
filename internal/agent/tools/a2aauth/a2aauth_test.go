package a2aauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolWritesAuthRequiredMarker(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envTaskDir, dir)

	out := New().CallTool(`{"message":"Please provide a token with the delegate scope"}`, nil)
	if !strings.Contains(out, "recorded") {
		t.Fatalf("output = %q", out)
	}
	data, err := os.ReadFile(filepath.Join(dir, authRequiredFile))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal marker: %v", err)
	}
	if payload.Message != "Please provide a token with the delegate scope" {
		t.Fatalf("message = %q", payload.Message)
	}
}

func TestToolRequiresTaskDir(t *testing.T) {
	t.Setenv(envTaskDir, "")
	out := New().CallTool(`{"message":"Authenticate"}`, nil)
	if !strings.Contains(out, "only available") {
		t.Fatalf("output = %q", out)
	}
}

func TestToolRequiresMessage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envTaskDir, dir)
	out := New().CallTool(`{}`, nil)
	if !strings.Contains(out, "message is required") {
		t.Fatalf("output = %q", out)
	}
}
