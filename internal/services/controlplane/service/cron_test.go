package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCronServiceAddListRunRemove(t *testing.T) {
	agentRoot := setupConfigAgent(t, nil)

	heartbeatCalls := 0
	var heartbeatBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/heartbeat" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		heartbeatCalls++
		if err := json.NewDecoder(r.Body).Decode(&heartbeatBody); err != nil {
			t.Errorf("decode heartbeat body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	env := "MYAAW_BASE_URL=" + server.URL + "\n"
	if err := os.WriteFile(filepath.Join(agentRoot, ".env"), []byte(env), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cronSvc := NewCronService(NewAgentService())

	job, err := cronSvc.Add("myfriend", CronJobInput{
		Name:         "Reminder",
		ScheduleKind: "every",
		Expr:         "1h",
		Message:      "hello",
		Channel:      "telegram",
		To:           "-1001234567890",
		ThreadID:     "77",
		SessionID:    "0f4e8b3a-1111-4222-8333-944455556666",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if job.ID == "" || job.AgentID != "myfriend" || !job.Enabled {
		t.Fatalf("job = %+v", job)
	}

	jobs, err := cronSvc.List("myfriend")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != job.ID {
		t.Fatalf("jobs = %+v", jobs)
	}

	if err := cronSvc.Run("myfriend", job.ID[:8]); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if heartbeatCalls != 1 {
		t.Fatalf("heartbeat calls = %d", heartbeatCalls)
	}
	// "Run now" must forward the same target the job stores, or the reply lands
	// in General (Telegram) or in the wrong session.
	for key, want := range map[string]string{
		"channel":    "telegram",
		"to":         "-1001234567890",
		"thread_id":  "77",
		"session_id": "0f4e8b3a-1111-4222-8333-944455556666",
		"trigger":    "cron",
	} {
		if got := heartbeatBody[key]; got != want {
			t.Errorf("heartbeat %s = %q, want %q", key, got, want)
		}
	}

	history, err := cronSvc.History("myfriend", job.ID[:8], 10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 1 || history[0].Status != "success" {
		t.Fatalf("history = %+v", history)
	}

	if err := cronSvc.Remove("myfriend", job.ID[:8]); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	jobs, _ = cronSvc.List("myfriend")
	if len(jobs) != 0 {
		t.Fatalf("jobs after remove = %+v", jobs)
	}

	if err := cronSvc.Remove("myfriend", "missing"); !errors.Is(err, ErrCronJobNotFound) {
		t.Fatalf("err = %v, want ErrCronJobNotFound", err)
	}
}

func TestCronServiceValidatesInput(t *testing.T) {
	setupConfigAgent(t, nil)
	cronSvc := NewCronService(NewAgentService())

	_, err := cronSvc.Add("myfriend", CronJobInput{ScheduleKind: "bad", Expr: "x", Message: "m"})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
	_, err = cronSvc.Add("myfriend", CronJobInput{ScheduleKind: "every", Expr: "", Message: "m"})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
	_, err = cronSvc.Add("myfriend", CronJobInput{ScheduleKind: "every", Expr: "1h", Message: ""})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
	_, err = cronSvc.Add("myfriend", CronJobInput{
		ScheduleKind: "every", Expr: "1h", Message: "m", Channel: "telegram", To: "1", ThreadID: "topic",
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("non-numeric thread_id: err = %v, want ErrInvalidConfig", err)
	}
	_, err = cronSvc.Add("myfriend", CronJobInput{
		ScheduleKind: "every", Expr: "1h", Message: "m", Channel: "discord", To: "1", ChatType: "channel",
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("unknown chat_type: err = %v, want ErrInvalidConfig", err)
	}
	_, err = cronSvc.Add("myfriend", CronJobInput{ScheduleKind: "every", Expr: "1h", Message: "m"})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("missing channel: err = %v, want ErrInvalidConfig", err)
	}
	_, err = cronSvc.Add("myfriend", CronJobInput{
		ScheduleKind: "every", Expr: "1h", Message: "m", Channel: "slack", To: "1",
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("unknown channel: err = %v, want ErrInvalidConfig", err)
	}
	_, err = cronSvc.Add("myfriend", CronJobInput{
		ScheduleKind: "every", Expr: "1h", Message: "m", Channel: "telegram",
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("missing to: err = %v, want ErrInvalidConfig", err)
	}
	_, err = cronSvc.Add("myfriend", CronJobInput{
		ScheduleKind: "every", Expr: "1h", Message: "m", Channel: "api", To: "111222333444555666",
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("api without a session: err = %v, want ErrInvalidConfig", err)
	}
}

func TestCronServiceNormalizesDeliveryForChannel(t *testing.T) {
	setupConfigAgent(t, nil)
	cronSvc := NewCronService(NewAgentService())

	// A Telegram target cannot carry a Discord chat type, and the reverse.
	telegram, err := cronSvc.Add("myfriend", CronJobInput{
		Name:         "Telegram topic",
		ScheduleKind: "every",
		Expr:         "1h",
		Message:      "hello",
		Channel:      "telegram",
		To:           "-1001234567890",
		ThreadID:     "77",
		ChatType:     "dm",
	})
	if err != nil {
		t.Fatalf("Add telegram: %v", err)
	}
	if telegram.Delivery.ChatType != "" || telegram.Delivery.ThreadID != "77" {
		t.Fatalf("telegram delivery = %+v", telegram.Delivery)
	}

	discord, err := cronSvc.Add("myfriend", CronJobInput{
		Name:         "Discord DM",
		ScheduleKind: "every",
		Expr:         "1h",
		Message:      "hello",
		Channel:      "discord",
		To:           "111222333444555666",
		ThreadID:     "77",
		ChatType:     "dm",
	})
	if err != nil {
		t.Fatalf("Add discord: %v", err)
	}
	if discord.Delivery.ThreadID != "" || discord.Delivery.ChatType != "dm" {
		t.Fatalf("discord delivery = %+v", discord.Delivery)
	}

	apiJob, err := cronSvc.Add("myfriend", CronJobInput{
		Name:         "Web reminder",
		ScheduleKind: "every",
		Expr:         "1h",
		Message:      "hello",
		Channel:      "api",
		To:           "111222333444555666",
		SessionID:    "0f4e8b3a-1111-4222-8333-944455556666",
	})
	if err != nil {
		t.Fatalf("Add api: %v", err)
	}
	if apiJob.Delivery.SessionID != "0f4e8b3a-1111-4222-8333-944455556666" {
		t.Fatalf("api delivery = %+v", apiJob.Delivery)
	}
}

// The dashboard must be able to express the same delivery targets as the CLI and
// the cron tool: a forum topic, an exact session, and a Discord DM.
func TestCronServiceKeepsDeliveryTarget(t *testing.T) {
	setupConfigAgent(t, nil)
	cronSvc := NewCronService(NewAgentService())

	job, err := cronSvc.Add("myfriend", CronJobInput{
		Name:         "Topic reminder",
		ScheduleKind: "every",
		Expr:         "1h",
		Message:      "hello",
		Channel:      "telegram",
		To:           "-1001234567890",
		ThreadID:     "77",
		SessionID:    "0f4e8b3a-1111-4222-8333-944455556666",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if job.AgentID != "myfriend" {
		t.Fatalf("AgentID = %q", job.AgentID)
	}
	if job.Delivery.ThreadID != "77" || job.Delivery.SessionID != "0f4e8b3a-1111-4222-8333-944455556666" {
		t.Fatalf("delivery = %+v", job.Delivery)
	}

	stored, err := cronSvc.List("myfriend")
	if err != nil || len(stored) != 1 {
		t.Fatalf("list = %+v, err = %v", stored, err)
	}
	if stored[0].Delivery.ThreadID != "77" {
		t.Fatalf("stored delivery = %+v", stored[0].Delivery)
	}
}
