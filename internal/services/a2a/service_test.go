package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	"myaaw/internal/config"
)

func TestSendMessageAsyncPushNotification(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	previous := config.A2AAllowPrivateWebhooks
	config.A2AAllowPrivateWebhooks = true
	t.Cleanup(func() { config.A2AAllowPrivateWebhooks = previous })
	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "async done", ConversationID: "conv-9"}}
	service := NewService(newTestStore(t), runner)

	notifications := make(chan StreamResponse, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response StreamResponse
		_ = json.NewDecoder(r.Body).Decode(&response)
		notifications <- response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	task, err := service.SendMessage(context.Background(), SendMessageRequest{
		Configuration: &SendMessageConfiguration{
			ReturnImmediately: true,
			TaskPushNotificationConfig: &PushNotificationConfig{
				URL: server.URL,
			},
		},
		Message: Message{Role: "user", Parts: []Part{{Text: "hello"}}},
	})
	if err != nil {
		t.Fatalf("SendMessage async: %v", err)
	}
	if task.Status.State != StateSubmitted {
		t.Fatalf("initial state = %q, want submitted", task.Status.State)
	}

	select {
	case got := <-notifications:
		if got.StatusUpdate == nil || got.StatusUpdate.Status.State != StateCompleted {
			t.Fatalf("notification = %+v", got)
		}
		if got.StatusUpdate.TaskID != task.ID {
			t.Fatalf("notification task id = %q, want %q", got.StatusUpdate.TaskID, task.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for push notification")
	}

	stored, err := service.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Status.State != StateCompleted {
		t.Fatalf("stored state = %q", stored.Status.State)
	}
	if stored.ContextID != "conv-9" {
		t.Fatalf("contextId = %q", stored.ContextID)
	}
}

func TestSendMessageAcceptsFileParts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive("myfriend"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}}
	service := NewService(newTestStore(t), runner)

	task, err := service.SendMessage(context.Background(), SendMessageRequest{
		Message: Message{Role: "user", Parts: []Part{
			{Text: "analyze these"},
			{Raw: []byte{0xff, 0xd8, 0xff, 0x00}, Filename: "photo.jpg", MediaType: "image/jpeg"},
			{Raw: []byte("hello file"), Filename: "notes.txt", MediaType: "text/plain"},
		}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if runner.input == nil {
		t.Fatal("runner was not called")
	}
	if len(runner.input.Images) != 1 {
		t.Fatalf("images = %+v", runner.input.Images)
	}
	for _, want := range []string{"analyze these", "Attached image: photo.jpg", "Attached file: notes.txt"} {
		if !strings.Contains(runner.input.Text, want) {
			t.Fatalf("text missing %q: %s", want, runner.input.Text)
		}
	}

	jobsDir, err := agentdir.ActiveJobsDir()
	if err != nil {
		t.Fatalf("ActiveJobsDir: %v", err)
	}
	attachment := filepath.Join(jobsDir, "a2a", task.ID, "notes.txt")
	data, err := os.ReadFile(attachment)
	if err != nil {
		t.Fatalf("read attachment: %v", err)
	}
	if string(data) != "hello file" {
		t.Fatalf("attachment = %q", data)
	}
}

func TestSendMessageInputRequiredAndResume(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive("myfriend"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	calls := 0
	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "final answer", ConversationID: "conv-1"}}
	runner.onRun = func() {
		calls++
		if calls != 1 {
			return
		}
		dir := os.Getenv("MYAAW_A2A_TASK_DIR")
		if dir == "" {
			t.Error("MYAAW_A2A_TASK_DIR is empty")
			return
		}
		_ = os.WriteFile(filepath.Join(dir, "input-required.json"), []byte(`{"question":"Which city?"}`), 0o644)
	}

	service := NewService(newTestStore(t), runner)
	task, err := service.SendMessage(context.Background(), SendMessageRequest{
		Message: Message{Role: "user", Parts: []Part{{Text: "weather?"}}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if task.Status.State != StateInputRequired {
		t.Fatalf("state = %q, want input-required", task.Status.State)
	}
	if task.Status.Message == nil || !strings.Contains(encodeMessageText(task.Status.Message), "Which city?") {
		t.Fatalf("status message = %+v", task.Status.Message)
	}

	resumed, err := service.SendMessage(context.Background(), SendMessageRequest{
		Message: Message{Role: "user", TaskID: task.ID, Parts: []Part{{Text: "Jakarta"}}},
	})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.Status.State != StateCompleted {
		t.Fatalf("resumed state = %q, want completed", resumed.Status.State)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestSendMessageCollectsOutputFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive("myfriend"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "report ready", ConversationID: "conv-3"}}
	runner.onRun = func() {
		dir := os.Getenv("MYAAW_A2A_TASK_DIR")
		_ = os.MkdirAll(filepath.Join(dir, "output"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, "output", "report.txt"), []byte("hello report"), 0o644)
	}

	service := NewService(newTestStore(t), runner)
	task, err := service.SendMessage(context.Background(), SendMessageRequest{
		Message: Message{Role: "user", Parts: []Part{{Text: "make a report"}}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	var found *Artifact
	for i := range task.Artifacts {
		if task.Artifacts[i].Name == "report.txt" {
			found = &task.Artifacts[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("artifacts = %+v", task.Artifacts)
	}
	if len(found.Parts) != 1 || len(found.Parts[0].Raw) == 0 {
		t.Fatalf("artifact = %+v", found)
	}
	if found.Parts[0].MediaType != "text/plain; charset=utf-8" {
		t.Fatalf("mediaType = %q", found.Parts[0].MediaType)
	}
	if string(found.Parts[0].Raw) != "hello report" {
		t.Fatalf("raw = %q", found.Parts[0].Raw)
	}
}

func TestSendMessageRejectsInvalidPushURL(t *testing.T) {
	service := NewService(newTestStore(t), &fakeRunner{})
	_, err := service.SendMessage(context.Background(), SendMessageRequest{
		Configuration: &SendMessageConfiguration{
			ReturnImmediately:          true,
			TaskPushNotificationConfig: &PushNotificationConfig{URL: "not-a-url"},
		},
		Message: Message{Role: "user", Parts: []Part{{Text: "hi"}}},
	})
	if err == nil {
		t.Fatal("expected invalid push URL error")
	}
}

type blockingRunner struct {
	started chan struct{}
}

func (b *blockingRunner) BotContext(ctx context.Context, _ *channel.IncomingMessage) (*channel.OutgoingMessage, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingRunner) BotStreamContext(ctx context.Context, _ *channel.IncomingMessage, _ func(channel.StreamChunk)) (*channel.OutgoingMessage, error) {
	return b.BotContext(ctx, nil)
}

func TestCancelTaskStopsRunningRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	runner := &blockingRunner{started: make(chan struct{}, 1)}
	store := newTestStore(t)
	service := NewService(store, runner)

	done := make(chan Task, 1)
	go func() {
		task, _ := service.SendMessage(context.Background(), SendMessageRequest{
			Message: Message{Role: "user", Parts: []Part{{Text: "long task"}}},
		})
		done <- task
	}()

	select {
	case <-runner.started:
	case <-time.After(3 * time.Second):
		t.Fatal("runner did not start")
	}

	pending, err := store.ListPending(context.Background())
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %+v, err = %v", pending, err)
	}

	canceled, err := service.CancelTask(context.Background(), pending[0].ID)
	if err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if canceled.Status.State != StateCanceled {
		t.Fatalf("canceled state = %q", canceled.Status.State)
	}

	select {
	case result := <-done:
		if result.Status.State != StateCanceled {
			t.Fatalf("final state = %q, want canceled", result.Status.State)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not stop after cancel")
	}

	stored, err := store.Get(context.Background(), pending[0].ID)
	if err != nil || stored.Status.State != StateCanceled {
		t.Fatalf("stored = %+v, err = %v", stored, err)
	}
}

func TestResumePendingTask(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	notifications := make(chan StreamResponse, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response StreamResponse
		_ = json.NewDecoder(r.Body).Decode(&response)
		notifications <- response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	service := NewService(store, &fakeRunner{out: &channel.OutgoingMessage{Text: "resumed", ConversationID: "conv-r"}})

	task := Task{
		ID:        "resume-1",
		ContextID: "ctx-1",
		Status:    TaskStatus{State: StateWorking},
		Messages:  []Message{{Role: "user", Parts: []Part{{Text: "hello"}}}},
		Attempts:  1,
		PushURL:   server.URL,
	}
	if err := store.Create(context.Background(), task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	resumed, err := service.ResumePending(context.Background())
	if err != nil || resumed != 1 {
		t.Fatalf("resumed = %d, err = %v", resumed, err)
	}

	select {
	case got := <-notifications:
		if got.StatusUpdate == nil || got.StatusUpdate.Status.State != StateCompleted || got.StatusUpdate.ContextID != "conv-r" {
			t.Fatalf("notification = %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for resumed task")
	}

	stored, err := store.Get(context.Background(), "resume-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", stored.Attempts)
	}
}

func TestResumePendingFailsExhaustedTask(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store := newTestStore(t)
	service := NewService(store, &fakeRunner{out: &channel.OutgoingMessage{Text: "never"}})

	task := Task{
		ID:       "resume-2",
		Status:   TaskStatus{State: StateWorking},
		Messages: []Message{{Role: "user", Parts: []Part{{Text: "hello"}}}},
		Attempts: maxTaskAttempts,
	}
	if err := store.Create(context.Background(), task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	resumed, err := service.ResumePending(context.Background())
	if err != nil || resumed != 0 {
		t.Fatalf("resumed = %d, err = %v", resumed, err)
	}

	stored, err := store.Get(context.Background(), "resume-2")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Status.State != StateFailed {
		t.Fatalf("state = %q, want failed", stored.Status.State)
	}
	if stored.Status.Message == nil || !strings.Contains(stored.Status.Message.Parts[0].Text, "attempts") {
		t.Fatalf("message = %+v", stored.Status.Message)
	}
}

func TestSendMessageUsesPerCallerUserIDs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}}
	service := NewService(newTestStore(t), runner)

	send := func(caller string) *channel.IncomingMessage {
		t.Helper()
		if _, err := service.SendMessage(context.Background(), SendMessageRequest{
			Caller:  caller,
			Message: Message{Role: "user", Parts: []Part{{Text: "hi"}}},
		}); err != nil {
			t.Fatalf("SendMessage(%s): %v", caller, err)
		}
		return runner.input
	}

	alice := send("alice")
	if alice == nil {
		t.Fatal("runner did not receive a message")
	}
	if alice.UserID >= -1000 {
		t.Fatalf("alice user id = %d, want a synthetic id below -1000", alice.UserID)
	}
	if alice.SenderName != "a2a:alice" {
		t.Fatalf("sender name = %q", alice.SenderName)
	}
	aliceID := alice.UserID

	if again := send("alice"); again.UserID != aliceID {
		t.Fatalf("alice id changed: %d -> %d", aliceID, again.UserID)
	}
	bob := send("bob")
	if bob.UserID == aliceID {
		t.Fatalf("bob and alice share user id %d", aliceID)
	}
	if bob.UserID == 0 {
		t.Fatal("A2A must not share user 0 with terminal or control-plane chats")
	}
}

func TestSendMessageIsIdempotentByMessageID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	runs := 0
	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "done"}}
	runner.onRun = func() { runs++ }
	service := NewService(newTestStore(t), runner)

	req := SendMessageRequest{Message: Message{
		MessageID: "msg-1",
		Role:      "user",
		Parts:     []Part{{Text: "hello"}},
	}}
	first, err := service.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("first SendMessage: %v", err)
	}
	second, err := service.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("retry SendMessage: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("retry created a new task: %q -> %q", first.ID, second.ID)
	}
	if runs != 1 {
		t.Fatalf("agent ran %d times for one message, want 1", runs)
	}

	other := req
	other.Caller = "other"
	third, err := service.SendMessage(context.Background(), other)
	if err != nil {
		t.Fatalf("other caller SendMessage: %v", err)
	}
	if third.ID == first.ID {
		t.Fatalf("different callers shared task %q", third.ID)
	}
	if runs != 2 {
		t.Fatalf("agent ran %d times, want 2", runs)
	}
}

func TestSendMessageRejectsOversizedInlinePayload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}}
	service := NewService(newTestStore(t), runner)

	_, err := service.SendMessage(context.Background(), SendMessageRequest{
		Message: Message{Role: "user", Parts: []Part{
			{Raw: bytes.Repeat([]byte("a"), 5<<20), Filename: "a.bin"},
			{Raw: bytes.Repeat([]byte("b"), 4<<20), Filename: "b.bin"},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "inline file parts exceed") {
		t.Fatalf("err = %v, want inline payload error", err)
	}
}

func TestQueuedTaskCancelIsHonored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	var runs atomic.Int32
	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "should not run"}}
	runner.onRun = func() { runs.Add(1) }
	service := NewService(newTestStore(t), runner)

	// Hold the agent loop so the task stays queued.
	service.runMu.Lock()
	locked := true
	defer func() {
		if locked {
			service.runMu.Unlock()
		}
	}()

	task, err := service.SendMessageAsync(context.Background(), SendMessageRequest{
		Message: Message{MessageID: "cancel-queued", Role: "user", Parts: []Part{{Text: "hello"}}},
	})
	if err != nil {
		t.Fatalf("SendMessageAsync: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if _, err := service.CancelTask(context.Background(), task.ID); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}

	service.runMu.Unlock()
	locked = false

	deadline := time.Now().Add(2 * time.Second)
	for {
		stored, err := service.GetTask(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if stored.Status.State != StateCanceled {
			t.Fatalf("state = %q, want canceled", stored.Status.State)
		}
		if runs.Load() != 0 {
			t.Fatal("agent ran a task that was canceled while queued")
		}
		if stored.Attempts != 0 {
			t.Fatalf("attempts = %d, want 0 (never started)", stored.Attempts)
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunTimeoutStartsAfterQueue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	previous := config.AgentRunTimeoutSeconds
	config.AgentRunTimeoutSeconds = 1
	t.Cleanup(func() { config.AgentRunTimeoutSeconds = previous })

	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "done"}, runDelay: 300 * time.Millisecond}
	service := NewService(newTestStore(t), runner)

	service.runMu.Lock()
	task, err := service.SendMessageAsync(context.Background(), SendMessageRequest{
		Message: Message{Role: "user", Parts: []Part{{Text: "hello"}}},
	})
	if err != nil {
		t.Fatalf("SendMessageAsync: %v", err)
	}
	// Wait longer than the run timeout while the task is still queued.
	time.Sleep(1500 * time.Millisecond)
	service.runMu.Unlock()

	deadline := time.Now().Add(3 * time.Second)
	for {
		stored, err := service.GetTask(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if stored.Status.State == StateCompleted {
			return
		}
		if stored.Status.State == StateFailed {
			t.Fatalf("queued task failed: %v", stored.Status.Message)
		}
		if time.Now().After(deadline) {
			t.Fatalf("state = %q, want completed", stored.Status.State)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunTimeoutPersistsFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	previous := config.AgentRunTimeoutSeconds
	config.AgentRunTimeoutSeconds = 1
	t.Cleanup(func() { config.AgentRunTimeoutSeconds = previous })

	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "late"}, runDelay: 5 * time.Second}
	service := NewService(newTestStore(t), runner)

	task, err := service.SendMessageAsync(context.Background(), SendMessageRequest{
		Message: Message{Role: "user", Parts: []Part{{Text: "hello"}}},
	})
	if err != nil {
		t.Fatalf("SendMessageAsync: %v", err)
	}

	deadline := time.Now().Add(4 * time.Second)
	for {
		stored, err := service.GetTask(context.Background(), task.ID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if stored.Status.State == StateFailed {
			if stored.Attempts != 1 {
				t.Fatalf("attempts = %d, want 1", stored.Attempts)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("state = %q, want failed (timeout must be persisted)", stored.Status.State)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestResumePendingDoesNotCountRestarts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store := newTestStore(t)
	service := NewService(store, &fakeRunner{out: &channel.OutgoingMessage{Text: "resumed"}})

	if err := store.Create(context.Background(), Task{
		ID:        "resume-attempts",
		ContextID: "ctx-1",
		Status:    TaskStatus{State: StateWorking},
		Messages:  []Message{{Role: "user", Parts: []Part{{Text: "hello"}}}},
		Attempts:  1,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	service.runMu.Lock()
	resumed, err := service.ResumePending(context.Background())
	if err != nil || resumed != 1 {
		service.runMu.Unlock()
		t.Fatalf("resumed = %d, err = %v", resumed, err)
	}
	time.Sleep(100 * time.Millisecond)

	queued, err := store.Get(context.Background(), "resume-attempts")
	if err != nil {
		service.runMu.Unlock()
		t.Fatalf("Get: %v", err)
	}
	if queued.Attempts != 1 {
		service.runMu.Unlock()
		t.Fatalf("attempts while queued = %d, want 1 (restart must not count)", queued.Attempts)
	}

	service.runMu.Unlock()
	deadline := time.Now().Add(3 * time.Second)
	for {
		stored, err := store.Get(context.Background(), "resume-attempts")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if stored.Status.State == StateCompleted {
			if stored.Attempts != 2 {
				t.Fatalf("attempts after start = %d, want 2", stored.Attempts)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("state = %q, want completed", stored.Status.State)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
