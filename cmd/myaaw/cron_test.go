package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/agentdir"
	"myaaw/internal/cron"
)

// A cron job must be written to the store of the agent named by --agent, even
// when the process itself is running as a different agent. Otherwise a chat on
// agent X could drop its job into the registry default's jobs.json.
func TestCronAddWritesToFlaggedAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive(agentdir.DefaultAgentName); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	flags := cronAddCmd.Flags()
	values := map[string]string{
		"name":       "Daily Report",
		"message":    "Summarize the day",
		"channel":    "telegram",
		"to":         "123456789",
		"at":         "10m",
		"thread-id":  "42",
		"session-id": "0f4e8b3a-1111-4222-8333-944455556666",
		"agent":      "agent-b",
	}
	for name, value := range values {
		if err := flags.Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	t.Cleanup(func() {
		for name := range values {
			_ = flags.Set(name, "")
		}
	})

	cronAddCmd.Run(cronAddCmd, nil)

	path := filepath.Join(home, ".myaaw", "agent-b", "cron", "jobs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var jobs []cron.Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		t.Fatalf("decode jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	job := jobs[0]
	if job.AgentID != "agent-b" {
		t.Errorf("AgentID = %q, want agent-b", job.AgentID)
	}
	if job.Delivery.ThreadID != "42" || job.Delivery.SessionID != "0f4e8b3a-1111-4222-8333-944455556666" {
		t.Errorf("delivery target lost: %+v", job.Delivery)
	}

	defaultStore := filepath.Join(home, ".myaaw", agentdir.DefaultAgentName, "cron", "jobs.json")
	if _, err := os.Stat(defaultStore); !os.IsNotExist(err) {
		t.Errorf("default agent store was touched: %v", err)
	}
}
