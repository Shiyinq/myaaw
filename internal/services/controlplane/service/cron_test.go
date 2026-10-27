package service

import (
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/heartbeat" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		heartbeatCalls++
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
		To:           "123",
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
}
