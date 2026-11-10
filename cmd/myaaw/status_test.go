package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeAgentFile creates a file under a temporary ~/.myaaw/agents/<agent>.
func writeAgentFile(t *testing.T, home, agent, name, body string) string {
	t.Helper()

	path := filepath.Join(home, ".myaaw", "agents", agent, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestAgentPortReadsEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeAgentFile(t, home, "agent-b", ".env", "PORT=9191\n")

	if got := agentPort("agent-b"); got != "9191" {
		t.Fatalf("agentPort = %q, want 9191", got)
	}
}

func TestAgentPortFallsBackToAssignedPort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeAgentFile(t, home, "agent-c", ".env", "")

	if got := agentPort("agent-c"); got != "8080" {
		t.Fatalf("agentPort = %q, want the first free port (8080)", got)
	}
}

func TestCollectAgentStatusReadsAgentFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeAgentFile(t, home, "agent-b", ".env", "PORT=9191\nSTREAM_RESPONSE=true\n")
	writeAgentFile(t, home, "agent-b", "config/config.json", `{
		"default_provider": "personal",
		"providers": {"personal": {"type": "openai"}},
		"heartbeat": {"active": true, "every": "20m", "to": "123", "channel": "telegram"},
		"channels": {
			"telegram": {"active": true, "token": "t", "mode": "polling"},
			"discord": {"active": false, "token": "d"}
		},
		"bot": {"stream": false}
	}`)
	if err := os.MkdirAll(filepath.Join(home, ".myaaw", "agents", "agent-b", "database"), 0o755); err != nil {
		t.Fatalf("create database dir: %v", err)
	}

	status := collectAgentStatus("agent-b")

	if status.Port != "9191" {
		t.Fatalf("port = %q", status.Port)
	}
	if !status.Telegram || status.TelegramMode != "polling" {
		t.Fatalf("telegram = %+v", status)
	}
	if status.Discord {
		t.Fatal("an inactive Discord channel was reported as configured")
	}
	if !status.HeartbeatActive || status.HeartbeatEvery != "20m" || status.HeartbeatTo != "123" || status.HeartbeatChannel != "telegram" {
		t.Fatalf("heartbeat = %+v", status)
	}
	if status.Provider != "openai" {
		t.Fatalf("provider = %q", status.Provider)
	}
	if status.Streaming {
		t.Fatal("bot.stream=false was ignored")
	}
	wantDB := filepath.Join(home, ".myaaw", "agents", "agent-b", "database", "myaaw.db")
	if status.DBPath != wantDB {
		t.Fatalf("database path = %q, want %q", status.DBPath, wantDB)
	}
	if status.DBErr != nil {
		t.Fatalf("database ping: %v", status.DBErr)
	}
}

func TestIsRegistryDefaultAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	registryDir := filepath.Join(home, ".myaaw", "config")
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatalf("create registry dir: %v", err)
	}
	body := `{"default":"agent-b","agents":{"agent-b":{},"agent-c":{}}}`
	if err := os.WriteFile(filepath.Join(registryDir, "agents.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	if !isRegistryDefaultAgent("agent-b") {
		t.Fatal("agent-b should be the registry default")
	}
	if isRegistryDefaultAgent("agent-c") {
		t.Fatal("agent-c is not the registry default")
	}
}
