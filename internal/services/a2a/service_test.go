package a2a

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
)

func TestSendTaskAsyncPushNotification(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "async done", ConversationID: "conv-9"}}
	service := NewService(newTestStore(t), runner)

	notifications := make(chan Task, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var task Task
		_ = json.NewDecoder(r.Body).Decode(&task)
		notifications <- task
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	task, err := service.SendTask(context.Background(), SendTaskParams{
		Async:               true,
		PushNotificationURL: server.URL,
		Message:             Message{Role: "user", Parts: []Part{{Kind: "text", Text: "hello"}}},
	})
	if err != nil {
		t.Fatalf("SendTask async: %v", err)
	}
	if task.Status.State != StateSubmitted {
		t.Fatalf("initial state = %q, want submitted", task.Status.State)
	}

	select {
	case got := <-notifications:
		if got.Status.State != StateCompleted {
			t.Fatalf("notification state = %q", got.Status.State)
		}
		if len(got.Artifacts) != 1 || got.Artifacts[0].Parts[0].Text != "async done" {
			t.Fatalf("notification = %+v", got)
		}
		if got.ContextID != "conv-9" {
			t.Fatalf("contextId = %q", got.ContextID)
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
}

func TestSendTaskAcceptsFileParts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive("myfriend"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}}
	service := NewService(newTestStore(t), runner)

	imageBytes := base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0x00})
	fileBytes := base64.StdEncoding.EncodeToString([]byte("hello file"))
	task, err := service.SendTask(context.Background(), SendTaskParams{
		Message: Message{Role: "user", Parts: []Part{
			{Kind: "text", Text: "analyze these"},
			{Kind: "file", File: &FilePart{Name: "photo.jpg", MimeType: "image/jpeg", Bytes: imageBytes}},
			{Kind: "file", File: &FilePart{Name: "notes.txt", MimeType: "text/plain", Bytes: fileBytes}},
		}},
	})
	if err != nil {
		t.Fatalf("SendTask: %v", err)
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

func TestSendTaskInputRequiredAndResume(t *testing.T) {
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
	task, err := service.SendTask(context.Background(), SendTaskParams{
		Message: Message{Role: "user", Parts: []Part{{Kind: "text", Text: "weather?"}}},
	})
	if err != nil {
		t.Fatalf("SendTask: %v", err)
	}
	if task.Status.State != StateInputRequired {
		t.Fatalf("state = %q, want input-required", task.Status.State)
	}
	if task.Status.Message == nil || !strings.Contains(messageText(*task.Status.Message), "Which city?") {
		t.Fatalf("status message = %+v", task.Status.Message)
	}

	resumed, err := service.SendTask(context.Background(), SendTaskParams{
		ID:      task.ID,
		Message: Message{Role: "user", Parts: []Part{{Kind: "text", Text: "Jakarta"}}},
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

func TestSendTaskCollectsOutputFiles(t *testing.T) {
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
	task, err := service.SendTask(context.Background(), SendTaskParams{
		Message: Message{Role: "user", Parts: []Part{{Kind: "text", Text: "make a report"}}},
	})
	if err != nil {
		t.Fatalf("SendTask: %v", err)
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
	if len(found.Parts) != 1 || found.Parts[0].File == nil {
		t.Fatalf("artifact = %+v", found)
	}
	if found.Parts[0].File.MimeType != "text/plain; charset=utf-8" {
		t.Fatalf("mime = %q", found.Parts[0].File.MimeType)
	}
	decoded, err := base64.StdEncoding.DecodeString(found.Parts[0].File.Bytes)
	if err != nil || string(decoded) != "hello report" {
		t.Fatalf("bytes = %q, err = %v", decoded, err)
	}
}

func TestSendTaskRejectsInvalidPushURL(t *testing.T) {
	service := NewService(newTestStore(t), &fakeRunner{})
	_, err := service.SendTask(context.Background(), SendTaskParams{
		Async:               true,
		PushNotificationURL: "not-a-url",
		Message:             Message{Role: "user", Parts: []Part{{Kind: "text", Text: "hi"}}},
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
		task, _ := service.SendTask(context.Background(), SendTaskParams{
			Message: Message{Role: "user", Parts: []Part{{Kind: "text", Text: "long task"}}},
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

	notifications := make(chan Task, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var task Task
		_ = json.NewDecoder(r.Body).Decode(&task)
		notifications <- task
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	service := NewService(store, &fakeRunner{out: &channel.OutgoingMessage{Text: "resumed", ConversationID: "conv-r"}})

	task := Task{
		ID:        "resume-1",
		ContextID: "ctx-1",
		Status:    TaskStatus{State: StateWorking},
		Messages:  []Message{{Role: "user", Parts: []Part{{Kind: "text", Text: "hello"}}}},
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
		if got.Status.State != StateCompleted || got.ContextID != "conv-r" {
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
		Messages: []Message{{Role: "user", Parts: []Part{{Kind: "text", Text: "hello"}}}},
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

func TestSendTaskUsesPerCallerUserIDs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}}
	service := NewService(newTestStore(t), runner)

	send := func(caller string) *channel.IncomingMessage {
		t.Helper()
		if _, err := service.SendTask(context.Background(), SendTaskParams{
			Caller:  caller,
			Message: Message{Role: "user", Parts: []Part{{Kind: "text", Text: "hi"}}},
		}); err != nil {
			t.Fatalf("SendTask(%s): %v", caller, err)
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
