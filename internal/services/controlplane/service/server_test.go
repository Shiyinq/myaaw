package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServerServiceStatusAndIdempotentStop(t *testing.T) {
	agentRoot := setupConfigAgent(t, nil)

	env := "PORT=65530\nMYAAW_BASE_URL=http://127.0.0.1:65530\n"
	if err := os.WriteFile(filepath.Join(agentRoot, ".env"), []byte(env), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	serverSvc := NewServerService(NewAgentService())
	status, err := serverSvc.Status("myfriend")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Running {
		t.Fatalf("server should not be running: %+v", status)
	}
	if status.Port != 65530 || status.BaseURL != "http://127.0.0.1:65530" {
		t.Fatalf("status = %+v", status)
	}

	if err := serverSvc.Stop("myfriend"); err != nil {
		t.Fatalf("Stop should be idempotent: %v", err)
	}
}
