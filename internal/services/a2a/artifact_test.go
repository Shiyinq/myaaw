package a2a

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/agentdir"
	"myaaw/internal/channel"

	"github.com/gofiber/fiber/v3"
	_ "github.com/mattn/go-sqlite3"
)

func newArtifactApp(t *testing.T, runner Runner) (*Service, *fiber.App) {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "a2a.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	service := NewService(NewTaskStore(db), runner)
	if err := service.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	handler := NewHandler(service)
	app := fiber.New()
	app.Get("/.well-known/agent-card.json", handler.Card)
	app.Post("/a2a", handler.RPC)
	app.Get("/tasks/:id/artifacts", handler.Artifact)
	return service, app
}

func TestLargeFileArtifactUsesURLAndDownloads(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive("myfriend"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	payload := bytes.Repeat([]byte("0123456789"), inlineFileBytes/10+2048)
	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "ok", ConversationID: "conv-big"}}
	runner.onRun = func() {
		dir := os.Getenv("MYAAW_A2A_TASK_DIR")
		_ = os.MkdirAll(filepath.Join(dir, "output"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, "output", "video.mp4"), payload, 0o644)
	}

	service, app := newArtifactApp(t, runner)
	task, err := service.SendMessage(context.Background(), SendMessageRequest{
		Message: Message{Role: "user", Parts: []Part{{Text: "go"}}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	var found *Artifact
	for i := range task.Artifacts {
		if task.Artifacts[i].Name == "video.mp4" {
			found = &task.Artifacts[i]
		}
	}
	if found == nil {
		t.Fatalf("artifacts = %+v", task.Artifacts)
	}
	if len(found.Parts) != 1 || found.Parts[0].URL == "" || len(found.Parts[0].Raw) != 0 {
		t.Fatalf("expected a url part, got %+v", found.Parts)
	}
	if found.Parts[0].MediaType != "video/mp4" {
		t.Fatalf("mediaType = %q", found.Parts[0].MediaType)
	}
	if found.Parts[0].Filename != "video.mp4" {
		t.Fatalf("filename = %q", found.Parts[0].Filename)
	}

	parsed, err := url.Parse(found.Parts[0].URL)
	if err != nil {
		t.Fatalf("parse artifact url: %v", err)
	}
	req := httptest.NewRequest("GET", parsed.RequestURI(), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("download mismatch: got %d bytes, want %d", len(body), len(payload))
	}
}

func TestArtifactPathTraversalRejected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "ok", ConversationID: "conv"}}
	service, app := newArtifactApp(t, runner)
	task, err := service.SendMessage(context.Background(), SendMessageRequest{
		Message: Message{Role: "user", Parts: []Part{{Text: "go"}}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	req := httptest.NewRequest("GET", "/tasks/"+task.ID+"/artifacts?path=../../etc/passwd", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestArtifactOwnership(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	setTestCallers(t, []string{"alice:tok-alice:delegate", "bob:tok-bob:delegate"})
	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "ok", ConversationID: "conv"}}
	service, app := newArtifactApp(t, runner)

	task, err := service.SendMessage(context.Background(), SendMessageRequest{
		Caller:  "alice",
		Message: Message{Role: "user", Parts: []Part{{Text: "go"}}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	req := httptest.NewRequest("GET", "/tasks/"+task.ID+"/artifacts?path=response", nil)
	req.Header.Set("Authorization", "Bearer tok-bob")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("bob status = %d, want 404", resp.StatusCode)
	}
}
