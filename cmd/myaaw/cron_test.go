package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	path := filepath.Join(home, ".myaaw", "agents", agentdir.DefaultAgentName, "cron", "jobs.json")
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

	path := filepath.Join(home, ".myaaw", "agents", agentdir.DefaultAgentName, "cron", "jobs.json")
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

	path := filepath.Join(home, ".myaaw", "agents", "agent-b", "cron", "jobs.json")
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

	defaultStore := filepath.Join(home, ".myaaw", "agents", agentdir.DefaultAgentName, "cron", "jobs.json")
	if _, err := os.Stat(defaultStore); !os.IsNotExist(err) {
		t.Errorf("default agent store was touched: %v", err)
	}
}

// `cron list <id>` answers "what is this job?" without opening the store or
// guessing at another agent's config.
func TestCronListShowsOneJob(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive(agentdir.DefaultAgentName); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	agent := agentdir.DefaultAgentName
	store := getStore(agent)
	water := cron.Job{
		ID:      "33333333-4444-4555-8666-777777777777",
		Name:    "Water reminder",
		AgentID: agent,
		Schedule: cron.Schedule{
			Kind: "at",
			Expr: "2026-11-05T15:13:10+07:00",
		},
		Payload: cron.Payload{Kind: "agentTurn", Content: "Time to drink water"},
		Delivery: cron.Delivery{
			Channel:  "telegram",
			To:       "-1001234567890",
			ThreadID: "77",
		},
		Enabled: true,
	}
	report := cron.Job{
		ID:      "33333334-4444-4555-8666-777777777777",
		Name:    "Daily Report",
		AgentID: agent,
		Schedule: cron.Schedule{
			Kind: "cron",
			Expr: "0 7 * * *",
		},
		Payload:  cron.Payload{Kind: "agentTurn", Content: "Ringkas traffic"},
		Delivery: cron.Delivery{Channel: "api", To: "123456789", SessionID: "0f4e8b3a-1111-4222-8333-944455556666"},
		Enabled:  true,
	}
	for _, job := range []cron.Job{water, report} {
		if err := store.Add(job); err != nil {
			t.Fatalf("add %s: %v", job.Name, err)
		}
	}

	// Without an id the table lists both jobs.
	table := captureStdout(t, func() { cronListCmd.Run(cronListCmd, nil) })
	for _, want := range []string{"Water reminder", "Daily Report"} {
		if !strings.Contains(table, want) {
			t.Errorf("table is missing %q:\n%s", want, table)
		}
	}

	// A prefix shows that one job in full, every stored field on its own line.
	detail := captureStdout(t, func() { cronListCmd.Run(cronListCmd, []string{"33333333"}) })
	for _, want := range [][2]string{
		{"ID", water.ID},
		{"Name", "Water reminder"},
		{"Schedule", "at · 2026-11-05T15:13:10+07:00"},
		{"Prompt", "Time to drink water"},
		{"Channel", "telegram"},
		{"To", "-1001234567890"},
		{"Topic", "77"},
	} {
		if !hasDetailLine(detail, want[0], want[1]) {
			t.Errorf("detail is missing %s %q:\n%s", want[0], want[1], detail)
		}
	}
	if strings.Contains(detail, "Daily Report") {
		t.Errorf("detail leaked another job:\n%s", detail)
	}

	// A prefix that matches both is rejected instead of guessing.
	ambiguous := captureStdout(t, func() { cronListCmd.Run(cronListCmd, []string{"3333333"}) })
	if !strings.Contains(ambiguous, "matches 2 jobs") {
		t.Errorf("ambiguous prefix was not reported:\n%s", ambiguous)
	}

	// An unknown id says so.
	missing := captureStdout(t, func() { cronListCmd.Run(cronListCmd, []string{"deadbeef"}) })
	if !strings.Contains(missing, "Job not found: deadbeef") {
		t.Errorf("missing job was not reported:\n%s", missing)
	}
}

// A job id means "show this job", so the whole log is returned; --limit only
// shapes the list across jobs.
func TestCronHistoryDetailShowsEveryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive(agentdir.DefaultAgentName); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	flags := cronHistoryCmd.Flags()
	if err := flags.Set("limit", "3"); err != nil {
		t.Fatalf("set --limit: %v", err)
	}
	t.Cleanup(func() { _ = flags.Set("limit", "10") })

	agent := agentdir.DefaultAgentName
	job := cron.Job{
		ID:      "11111111-2222-4333-8444-555555555555",
		Name:    "Water reminder",
		AgentID: agent,
		Payload: cron.Payload{Kind: "agentTurn", Content: "Minum air"},
		Delivery: cron.Delivery{
			Channel: "api",
			To:      "123456789",
		},
		Enabled: true,
	}
	logger := getHistoryLogger(agent)
	for i := 1; i <= 12; i++ {
		if err := logger.Log(job, "success", fmt.Sprintf("tick %d", i)); err != nil {
			t.Fatalf("log run %d: %v", i, err)
		}
	}

	// The detail view is not paged: all 12 runs come back even though --limit is 3.
	detail := captureStdout(t, func() { cronHistoryCmd.Run(cronHistoryCmd, []string{"11111111"}) })
	if got := strings.Count(detail, "tick "); got != 12 {
		t.Fatalf("detail showed %d runs, want 12:\n%s", got, detail)
	}

	// The list still honours --limit.
	list := captureStdout(t, func() { cronHistoryCmd.Run(cronHistoryCmd, nil) })
	if got := strings.Count(list, "tick "); got != 3 {
		t.Fatalf("list showed %d runs, want 3:\n%s", got, list)
	}
}

// A one-time job is removed after it runs, so its id lives only in the run log.
// Reading its detail must not depend on jobs.json still knowing the job.
func TestCronHistoryFindsRemovedJob(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive(agentdir.DefaultAgentName); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	agent := agentdir.DefaultAgentName
	removed := cron.Job{
		ID:      "11111111-2222-4333-8444-555555555555",
		Name:    "Water reminder",
		AgentID: agent,
		Payload: cron.Payload{Kind: "agentTurn", Content: "Time to drink water"},
		Delivery: cron.Delivery{
			Channel:   "api",
			To:        "123456789",
			SessionID: "0f4e8b3a-1111-4222-8333-944455556666",
		},
		Enabled: true,
	}
	// Only the run is recorded; the job itself was already removed by the
	// scheduler, exactly like a finished one-time job.
	if err := getHistoryLogger(agent).Log(removed, "success", "Job executed successfully"); err != nil {
		t.Fatalf("log run: %v", err)
	}

	out := captureStdout(t, func() { cronHistoryCmd.Run(cronHistoryCmd, []string{"11111111"}) })
	if strings.Contains(out, "Job not found") {
		t.Fatalf("a removed job was reported missing:\n%s", out)
	}
	for _, want := range [][2]string{
		{"Job ID", removed.ID},
		{"Job", "Water reminder"},
		{"Prompt", "Time to drink water"},
		{"Channel", "api"},
		{"To", "123456789"},
		{"Session", "0f4e8b3a-1111-4222-8333-944455556666"},
	} {
		if !hasDetailLine(out, want[0], want[1]) {
			t.Errorf("detail is missing %s %q:\n%s", want[0], want[1], out)
		}
	}
}

// The JSON flag is gone: the list is a table, and a job id is what reveals the
// detail of each run. The agent's cron tool relies on exactly that split.
func TestCronHistoryListAndDetail(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive(agentdir.DefaultAgentName); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	agent := agentdir.DefaultAgentName
	job := cron.Job{
		ID:      "11111111-2222-4333-8444-555555555555",
		Name:    "Water reminder",
		AgentID: agent,
		Payload: cron.Payload{Kind: "agentTurn", Content: "Time to drink water"},
		Delivery: cron.Delivery{
			Channel:   "telegram",
			To:        "-1001234567890",
			ThreadID:  "77",
			SessionID: "0f4e8b3a-1111-4222-8333-944455556666",
		},
		Enabled: true,
	}
	if err := getStore(agent).Add(job); err != nil {
		t.Fatalf("add job: %v", err)
	}
	if err := getHistoryLogger(agent).Log(job, "success", "Job executed successfully"); err != nil {
		t.Fatalf("log run: %v", err)
	}

	// No id: the list, which names the job next to its id and points at the
	// command that shows more.
	list := captureStdout(t, func() { cronHistoryCmd.Run(cronHistoryCmd, nil) })
	for _, want := range []string{"Water reminder", "11111111", "myaaw cron history <job-id>"} {
		if !strings.Contains(list, want) {
			t.Errorf("list is missing %q:\n%s", want, list)
		}
	}
	if strings.Contains(list, "Time to drink water") {
		t.Errorf("list leaked run detail:\n%s", list)
	}

	// A job id, even a prefix: every run of that job in full.
	detail := captureStdout(t, func() { cronHistoryCmd.Run(cronHistoryCmd, []string{"11111111"}) })
	for _, want := range [][2]string{
		{"Job", "Water reminder"},
		{"Prompt", "Time to drink water"},
		{"Channel", "telegram"},
		{"To", "-1001234567890"},
		{"Topic", "77"},
		{"Session", "0f4e8b3a-1111-4222-8333-944455556666"},
		{"Result", "Job executed successfully"},
	} {
		if !hasDetailLine(detail, want[0], want[1]) {
			t.Errorf("detail is missing %s %q:\n%s", want[0], want[1], detail)
		}
	}
}

// hasDetailLine reports whether a detail block carries "label value" on one line.
func hasDetailLine(detail, label, value string) bool {
	for _, line := range strings.Split(detail, "\n") {
		if strings.HasPrefix(line, label) && strings.HasSuffix(line, value) {
			return true
		}
	}
	return false
}
