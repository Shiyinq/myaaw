package daemon

import (
	"path/filepath"
	"testing"
)

func TestNewAgentManagerUsesThatAgentsLogs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	manager, err := NewAgentManager("agent-b")
	if err != nil {
		t.Fatalf("NewAgentManager: %v", err)
	}
	if manager.Name != "myaaw-server-agent-b" {
		t.Fatalf("name = %q, want myaaw-server-agent-b", manager.Name)
	}

	wantLog := filepath.Join(home, ".myaaw", "agents", "agent-b", "logs", "myaaw-server-agent-b.log")
	if manager.LogFile != wantLog {
		t.Fatalf("log = %q, want %q", manager.LogFile, wantLog)
	}

	wantPID := filepath.Join(home, ".myaaw", "pids", "myaaw-server-agent-b.pid")
	if manager.PIDFile != wantPID {
		t.Fatalf("pid file = %q, want %q", manager.PIDFile, wantPID)
	}
}

func TestNewManagerUsesActiveAgentLogs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYAAW_AGENT", "")

	manager, err := NewManager("myaaw-server-default")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	wantLog := filepath.Join(home, ".myaaw", "agents", "default", "logs", "myaaw-server-default.log")
	if manager.LogFile != wantLog {
		t.Fatalf("log = %q, want %q", manager.LogFile, wantLog)
	}
}

func TestNewGlobalManagerUsesGlobalLogs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	manager, err := NewGlobalManager("myaaw-api")
	if err != nil {
		t.Fatalf("NewGlobalManager: %v", err)
	}

	wantLog := filepath.Join(home, ".myaaw", "logs", "myaaw-api.log")
	if manager.LogFile != wantLog {
		t.Fatalf("log = %q, want %q", manager.LogFile, wantLog)
	}
	if wantPID := filepath.Join(home, ".myaaw", "pids", "myaaw-api.pid"); manager.PIDFile != wantPID {
		t.Fatalf("pid file = %q, want %q", manager.PIDFile, wantPID)
	}
}
