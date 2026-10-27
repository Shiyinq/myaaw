package main

import (
	"testing"

	"myaaw/internal/agentdir"
)

func TestPrepareAgentEnvAssignsUniquePorts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	// First agent, whatever its name, gets 8080.
	if err := prepareAgentEnv("agent-a", 0); err != nil {
		t.Fatalf("prepareAgentEnv(agent-a): %v", err)
	}
	envA, err := agentdir.EnvPath("agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if port := readEnvValue(envA, "PORT"); port != "8080" {
		t.Fatalf("agent-a PORT = %q, want 8080", port)
	}
	if err := agentdir.RegisterAgent("agent-a"); err != nil {
		t.Fatal(err)
	}

	if err := prepareAgentEnv("agent-b", 0); err != nil {
		t.Fatalf("prepareAgentEnv(agent-b): %v", err)
	}
	envB, err := agentdir.EnvPath("agent-b")
	if err != nil {
		t.Fatal(err)
	}
	if port := readEnvValue(envB, "PORT"); port != "8081" {
		t.Fatalf("agent-b PORT = %q, want 8081", port)
	}
	if base := readEnvValue(envB, "MYAAW_BASE_URL"); base != "http://localhost:8081" {
		t.Fatalf("agent-b MYAAW_BASE_URL = %q", base)
	}
	if err := agentdir.RegisterAgent("agent-b"); err != nil {
		t.Fatal(err)
	}

	// An explicit port already assigned to another agent is rejected.
	if err := prepareAgentEnv("agent-c", 8080); err == nil {
		t.Fatal("expected a port conflict error")
	}
	// Re-applying the same agent's own port is allowed.
	if err := prepareAgentEnv("agent-b", 8081); err != nil {
		t.Fatalf("re-applying own port failed: %v", err)
	}
	// Out-of-range ports are rejected.
	if err := prepareAgentEnv("agent-c", 70000); err == nil {
		t.Fatal("expected a range error")
	}
}

func TestPrepareAgentEnvKeepsExistingPort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	if err := prepareAgentEnv("agent-a", 9100); err != nil {
		t.Fatal(err)
	}
	// A later onboarding run without --port must not move the agent.
	if err := prepareAgentEnv("agent-a", 0); err != nil {
		t.Fatal(err)
	}
	envPath, err := agentdir.EnvPath("agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if port := readEnvValue(envPath, "PORT"); port != "9100" {
		t.Fatalf("PORT = %q, want the existing 9100", port)
	}
}
