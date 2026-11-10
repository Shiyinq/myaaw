package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"myaaw/internal/agentdir"

	"github.com/spf13/cobra"
)

func targetKindsAndNames(targets []serviceTarget) [][2]string {
	out := make([][2]string, 0, len(targets))
	for _, target := range targets {
		out = append(out, [2]string{target.kind, target.name})
	}
	return out
}

func TestBuildServiceTargetsDefault(t *testing.T) {
	got := targetKindsAndNames(buildServiceTargets("", false, nil, "default"))
	want := [][2]string{
		{targetAgent, "default"},
		{targetAPI, ""},
		{targetWebUI, ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
}

func TestBuildServiceTargetsAgentFlag(t *testing.T) {
	got := targetKindsAndNames(buildServiceTargets("agent-b", false, []string{"default", "agent-b"}, "default"))
	want := [][2]string{
		{targetAgent, "agent-b"},
		{targetAPI, ""},
		{targetWebUI, ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
}

func TestBuildServiceTargetsAllAgents(t *testing.T) {
	got := targetKindsAndNames(buildServiceTargets("", true, []string{"default", "agent-b"}, "default"))
	want := [][2]string{
		{targetAgent, "default"},
		{targetAgent, "agent-b"},
		{targetAPI, ""},
		{targetWebUI, ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
}

func TestBuildServiceTargetsAllAgentsWinsOverAgentFlag(t *testing.T) {
	got := targetKindsAndNames(buildServiceTargets("other", true, []string{"default", "agent-b"}, "default"))
	want := [][2]string{
		{targetAgent, "default"},
		{targetAgent, "agent-b"},
		{targetAPI, ""},
		{targetWebUI, ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
}

func TestBuildServiceTargetsAllAgentsEmptyRegistry(t *testing.T) {
	got := targetKindsAndNames(buildServiceTargets("", true, nil, "default"))
	want := [][2]string{
		{targetAPI, ""},
		{targetWebUI, ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
}

func TestReverseTargets(t *testing.T) {
	targets := []serviceTarget{
		{kind: targetAgent, name: "default"},
		{kind: targetAPI},
		{kind: targetWebUI},
	}
	got := targetKindsAndNames(reverseTargets(targets))
	want := [][2]string{
		{targetWebUI, ""},
		{targetAPI, ""},
		{targetAgent, "default"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reversed = %v, want %v", got, want)
	}

	// The input must not be mutated.
	if targets[0].kind != targetAgent {
		t.Fatalf("reverseTargets mutated its input: %v", targets)
	}
}

func TestWaitDaemonStatusReturnsLivePID(t *testing.T) {
	calls := 0
	pid, ok := waitDaemonStatus(func() (int, bool, error) {
		calls++
		if calls < 3 {
			return 0, false, nil
		}
		return 4242, true, nil
	}, time.Second)
	if !ok || pid != 4242 {
		t.Fatalf("waitDaemonStatus = (%d, %v), want (4242, true)", pid, ok)
	}
}

func TestWaitDaemonStatusTimesOut(t *testing.T) {
	start := time.Now()
	if _, ok := waitDaemonStatus(func() (int, bool, error) { return 0, false, nil }, 50*time.Millisecond); ok {
		t.Fatal("a daemon that never starts must not be reported as running")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout not honoured: %v", elapsed)
	}
}

func TestDescribeTarget(t *testing.T) {
	if got := describeTarget(serviceTarget{kind: targetAgent, name: "agent-b"}); got != `agent "agent-b"` {
		t.Fatalf("agent label = %q", got)
	}
	if got := describeTarget(serviceTarget{kind: targetAPI}); got != "API" {
		t.Fatalf("api label = %q", got)
	}
	if got := describeTarget(serviceTarget{kind: targetWebUI}); got != "WebUI" {
		t.Fatalf("webui label = %q", got)
	}
}

func TestAllAgentsRequested(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("all-agent", false, "")
	if allAgentsRequested(cmd) {
		t.Fatal("the flag should default to false")
	}
	if err := cmd.Flags().Set("all-agent", "true"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if !allAgentsRequested(cmd) {
		t.Fatal("the flag was not detected")
	}

	if allAgentsRequested(&cobra.Command{Use: "other"}) {
		t.Fatal("commands without the flag must report false")
	}
}

// writeTestRegistry creates ~/.myaaw/config/agents.json and an initialized
// directory for every named agent in a temporary HOME.
func writeTestRegistry(t *testing.T, home string, body string, agents ...string) {
	t.Helper()

	dir := filepath.Join(home, ".myaaw", "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create registry dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agents.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	for _, name := range agents {
		agentConfig := filepath.Join(home, ".myaaw", "agents", name, "config")
		if err := os.MkdirAll(agentConfig, 0o755); err != nil {
			t.Fatalf("create agent %q dir: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(agentConfig, "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write agent %q config: %v", name, err)
		}
	}
}

func TestServerAgentTargetsHonoursAllAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYAAW_AGENT", "")
	writeTestRegistry(t, home, `{"default":"default","agents":{"default":{},"agent-b":{}}}`, "default", "agent-b")

	previous := serviceAllAgents
	t.Cleanup(func() { serviceAllAgents = previous })

	serviceAllAgents = true
	targets, err := serverAgentTargets()
	if err != nil {
		t.Fatalf("serverAgentTargets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("--all-agent returned %d targets, want 2: %v", len(targets), targets)
	}

	serviceAllAgents = false
	targets, err = serverAgentTargets()
	if err != nil {
		t.Fatalf("serverAgentTargets: %v", err)
	}
	if len(targets) != 1 || targets[0].name != agentdir.Active() {
		t.Fatalf("single-agent targets = %v", targets)
	}
}

func TestCurrentServiceTargetsAllAgentAddsAPIAndWebUI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYAAW_AGENT", "")
	writeTestRegistry(t, home, `{"default":"default","agents":{"default":{},"agent-b":{}}}`, "default", "agent-b")

	previous := serviceAllAgents
	t.Cleanup(func() { serviceAllAgents = previous })
	serviceAllAgents = true

	targets, err := currentServiceTargets()
	if err != nil {
		t.Fatalf("currentServiceTargets: %v", err)
	}
	got := targetKindsAndNames(targets)
	want := [][2]string{
		{targetAgent, "agent-b"},
		{targetAgent, "default"},
		{targetAPI, ""},
		{targetWebUI, ""},
	}
	if !reflect.DeepEqual(got, want) && !reflect.DeepEqual(got, [][2]string{
		{targetAgent, "default"},
		{targetAgent, "agent-b"},
		{targetAPI, ""},
		{targetWebUI, ""},
	}) {
		t.Fatalf("targets = %v", got)
	}
}
