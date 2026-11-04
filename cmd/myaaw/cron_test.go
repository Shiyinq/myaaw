package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/agentdir"
	"myaaw/internal/cron"
)

func TestCronAddRequiresSessionForAPITargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive(agentdir.DefaultAgentName); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	flags := cronAddCmd.Flags()
	values := map[string]string{
		"name":    "Web reminder",
		"message": "Time to drink water",
		"channel": "api",
		"to":      "123456789",
		"at":      "10m",
	}
	for name, value := range values {
		if err := flags.Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	t.Cleanup(func() {
		for _, name := range []string{"name", "message", "channel", "to", "at", "session-id"} {
			_ = flags.Set(name, "")
		}
	})

	cronAddCmd.Run(cronAddCmd, nil)

	path := filepath.Join(home, ".myaaw", agentdir.DefaultAgentName, "cron", "jobs.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("an api job without a session id was stored: %v", err)
	}

	// The same command succeeds once the session is pinned.
	if err := flags.Set("session-id", "0f4e8b3a-1111-4222-8333-944455556666"); err != nil {
		t.Fatalf("set --session-id: %v", err)
	}
	cronAddCmd.Run(cronAddCmd, nil)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("api job with a session id was not stored: %v", err)
	}
}

// chat_type only distinguishes a Discord DM from a channel. Telegram encodes the
// room kind in the chat id, so the CLI must not store a misleading value.
func TestCronAddDropsChatTypeForNonDiscord(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive(agentdir.DefaultAgentName); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	flags := cronAddCmd.Flags()
	values := map[string]string{
		"name":      "Sapa group",
		"message":   "Hai semuanya",
		"channel":   "telegram",
		"to":        "-1001234567890",
		"at":        "10m",
		"chat-type": "private",
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

	path := filepath.Join(home, ".myaaw", agentdir.DefaultAgentName, "cron", "jobs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var jobs []cron.Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		t.Fatalf("decode jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
	if jobs[0].Delivery.ChatType != "" {
		t.Fatalf("telegram chat_type = %q, want it dropped", jobs[0].Delivery.ChatType)
	}
	if jobs[0].Delivery.To != "-1001234567890" {
		t.Fatalf("target changed: %+v", jobs[0].Delivery)
	}
}

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
