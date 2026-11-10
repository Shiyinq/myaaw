package main

import (
	"reflect"
	"testing"
)

func TestRunningAgents(t *testing.T) {
	names := []string{"default", "agent-b", "agent-c"}
	running := map[string]bool{"agent-b": true}

	got := runningAgents(names, func(name string) bool { return running[name] })
	want := []string{"agent-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runningAgents = %v, want %v", got, want)
	}

	if got := runningAgents(names, func(string) bool { return false }); len(got) != 0 {
		t.Fatalf("runningAgents with nothing running = %v", got)
	}
}

func TestWebuiVersionLabel(t *testing.T) {
	if got := webuiVersionLabel("v2026.11.10.4"); got != "v2026.11.10.4" {
		t.Fatalf("label = %q", got)
	}
	if got := webuiVersionLabel("  "); got != "unknown" {
		t.Fatalf("empty label = %q, want unknown", got)
	}
}
