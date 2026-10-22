package a2a

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myaaw/internal/channel"
	"myaaw/internal/config"

	"github.com/gofiber/fiber/v3"
	_ "github.com/mattn/go-sqlite3"
)

type fakeRunner struct {
	out          *channel.OutgoingMessage
	err          error
	input        *channel.IncomingMessage
	streamChunks []channel.StreamChunk
	onRun        func()
}

func (f *fakeRunner) BotContext(_ context.Context, msg *channel.IncomingMessage) (*channel.OutgoingMessage, error) {
	f.input = msg
	if f.onRun != nil {
		f.onRun()
	}
	return f.out, f.err
}

func (f *fakeRunner) BotStreamContext(_ context.Context, msg *channel.IncomingMessage, onChunk func(channel.StreamChunk)) (*channel.OutgoingMessage, error) {
	f.input = msg
	for _, chunk := range f.streamChunks {
		onChunk(chunk)
	}
	if f.onRun != nil {
		f.onRun()
	}
	return f.out, f.err
}

func newTestApp(t *testing.T, runner Runner) *fiber.App {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
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
	app.Get("/.well-known/agent.json", handler.Card)
	app.Post("/a2a", handler.RPC)
	return app
}

func doRPC(t *testing.T, app *fiber.App, token, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, payload
}

func TestRPCSendAndGet(t *testing.T) {
	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "remote answer", ConversationID: "conv-1"}}
	app := newTestApp(t, runner)

	status, payload := doRPC(t, app, "", `{
		"jsonrpc":"2.0","id":1,"method":"tasks/send",
		"params":{"message":{"role":"user","parts":[{"kind":"text","text":"hello"}]}}
	}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, payload = %+v", status, payload)
	}
	result, _ := payload["result"].(map[string]any)
	if result == nil {
		t.Fatalf("missing result: %+v", payload)
	}
	if result["contextId"] != "conv-1" {
		t.Fatalf("contextId = %v", result["contextId"])
	}
	statusObj, _ := result["status"].(map[string]any)
	if statusObj["state"] != StateCompleted {
		t.Fatalf("status = %+v", statusObj)
	}
	artifacts, _ := result["artifacts"].([]any)
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %+v", artifacts)
	}
	if runner.input == nil || !strings.HasPrefix(runner.input.Text, "hello") || runner.input.Channel != "a2a" || runner.input.ConversationID != "NEW" {
		t.Fatalf("runner input = %+v", runner.input)
	}

	taskID, _ := result["id"].(string)
	if taskID == "" {
		t.Fatal("missing task id")
	}
	status, payload = doRPC(t, app, "", `{"jsonrpc":"2.0","id":2,"method":"tasks/get","params":{"id":"`+taskID+`"}}`)
	if status != fiber.StatusOK {
		t.Fatalf("get status = %d, payload = %+v", status, payload)
	}
	if payload["error"] != nil {
		t.Fatalf("get error = %+v", payload["error"])
	}
}

func TestRPCFailedTaskAndMissingTask(t *testing.T) {
	runner := &fakeRunner{err: errors.New("boom")}
	app := newTestApp(t, runner)

	_, payload := doRPC(t, app, "", `{
		"jsonrpc":"2.0","id":1,"method":"tasks/send",
		"params":{"message":{"role":"user","parts":[{"kind":"text","text":"hello"}]}}
	}`)
	result, _ := payload["result"].(map[string]any)
	statusObj, _ := result["status"].(map[string]any)
	if statusObj["state"] != StateFailed {
		t.Fatalf("status = %+v", statusObj)
	}

	_, payload = doRPC(t, app, "", `{"jsonrpc":"2.0","id":2,"method":"tasks/get","params":{"id":"missing"}}`)
	errObj, _ := payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeTaskNotFound) {
		t.Fatalf("error = %+v", payload)
	}

	_, payload = doRPC(t, app, "", `{"jsonrpc":"2.0","id":3,"method":"unknown"}`)
	errObj, _ = payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeMethodNotFound) {
		t.Fatalf("error = %+v", payload)
	}
}

func TestRPCAuth(t *testing.T) {
	previous := config.A2AToken
	config.A2AToken = "secret"
	t.Cleanup(func() { config.A2AToken = previous })

	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}}
	app := newTestApp(t, runner)

	body := `{"jsonrpc":"2.0","id":1,"method":"tasks/send","params":{"message":{"role":"user","parts":[{"kind":"text","text":"hi"}]}}}`
	status, _ := doRPC(t, app, "", body)
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status without token = %d, want 401", status)
	}
	status, payload := doRPC(t, app, "secret", body)
	if status != fiber.StatusOK || payload["error"] != nil {
		t.Fatalf("status with token = %d, payload = %+v", status, payload)
	}
}

func TestRPCMultipleTokens(t *testing.T) {
	previousToken := config.A2AToken
	previousTokens := config.A2ATokens
	config.A2AToken = ""
	config.A2ATokens = []string{"caller-a", "caller-b"}
	t.Cleanup(func() {
		config.A2AToken = previousToken
		config.A2ATokens = previousTokens
	})

	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})
	body := `{"jsonrpc":"2.0","id":1,"method":"tasks/send","params":{"message":{"role":"user","parts":[{"kind":"text","text":"hi"}]}}}`
	if status, payload := doRPC(t, app, "caller-b", body); status != fiber.StatusOK || payload["error"] != nil {
		t.Fatalf("caller-b status = %d, payload = %+v", status, payload)
	}
	if status, _ := doRPC(t, app, "caller-c", body); status != fiber.StatusUnauthorized {
		t.Fatalf("caller-c status = %d, want 401", status)
	}
}

func TestRPCStreaming(t *testing.T) {
	runner := &fakeRunner{
		out: &channel.OutgoingMessage{Text: "Hello world", ConversationID: "conv-2"},
		streamChunks: []channel.StreamChunk{
			{Kind: "text", Text: "Hello "},
			{Kind: "text", Text: "world"},
		},
	}
	app := newTestApp(t, runner)

	req := httptest.NewRequest("POST", "/a2a", strings.NewReader(`{
		"jsonrpc":"2.0","id":1,"method":"tasks/sendSubscribe",
		"params":{"message":{"role":"user","parts":[{"kind":"text","text":"hi"}]}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	stream := string(body)
	for _, want := range []string{
		"event: status",
		`"state":"working"`,
		`"text":"Hello "`,
		`"text":"Hello world"`,
		`"lastChunk":true`,
		`"state":"completed"`,
		`"final":true`,
	} {
		if !strings.Contains(stream, want) {
			t.Errorf("stream missing %q:\n%s", want, stream)
		}
	}
	if runner.input == nil || runner.input.ConversationID != "NEW" {
		t.Fatalf("runner input = %+v", runner.input)
	}
}

func TestCard(t *testing.T) {
	app := newTestApp(t, &fakeRunner{})
	req := httptest.NewRequest("GET", "/.well-known/agent.json", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if card.Name == "" || !strings.HasSuffix(card.URL, "/a2a") {
		t.Fatalf("card = %+v", card)
	}
	if card.Capabilities.Streaming != true {
		t.Fatal("fase 2 should advertise streaming")
	}
	if card.Capabilities.PushNotifications != true {
		t.Fatal("fase 3 should advertise push notifications")
	}
}

func TestRPCResubscribeReplaysFinishedStream(t *testing.T) {
	runner := &fakeRunner{
		out: &channel.OutgoingMessage{Text: "Hello world", ConversationID: "conv-3"},
		streamChunks: []channel.StreamChunk{
			{Kind: "text", Text: "Hello "},
			{Kind: "text", Text: "world"},
		},
	}
	app := newTestApp(t, runner)

	req := httptest.NewRequest("POST", "/a2a", strings.NewReader(`{
		"jsonrpc":"2.0","id":1,"method":"tasks/sendSubscribe",
		"params":{"message":{"role":"user","parts":[{"kind":"text","text":"hi"}]}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	taskID := ""
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event StreamEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err == nil && event.TaskID != "" {
			taskID = event.TaskID
			break
		}
	}
	if taskID == "" {
		t.Fatalf("no task id in stream:\n%s", body)
	}

	req = httptest.NewRequest("POST", "/a2a", strings.NewReader(`{
		"jsonrpc":"2.0","id":2,"method":"tasks/resubscribe",
		"params":{"id":"`+taskID+`"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("resubscribe app.Test: %v", err)
	}
	defer resp.Body.Close()
	replayed, _ := io.ReadAll(resp.Body)

	for _, want := range []string{"event: status", `"lastChunk":true`, `"state":"completed"`, `"final":true`} {
		if !strings.Contains(string(replayed), want) {
			t.Errorf("resubscribe missing %q:\n%s", want, replayed)
		}
	}
}

func TestRPCResubscribeWithoutLiveStream(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	_, payload := doRPC(t, app, "", scopeTestSendBody)
	result, _ := payload["result"].(map[string]any)
	taskID, _ := result["id"].(string)
	if taskID == "" {
		t.Fatalf("missing task id: %+v", payload)
	}

	req := httptest.NewRequest("POST", "/a2a", strings.NewReader(`{
		"jsonrpc":"2.0","id":2,"method":"tasks/resubscribe",
		"params":{"id":"`+taskID+`"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if !strings.Contains(string(body), `"state":"completed"`) || !strings.Contains(string(body), `"final":true`) {
		t.Fatalf("body = %s", body)
	}
}

func TestRPCResponseShape(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	_, payload := doRPC(t, app, "", scopeTestSendBody)
	result, _ := payload["result"].(map[string]any)
	if result["kind"] != "task" {
		t.Fatalf("kind = %v", result["kind"])
	}
	history, _ := result["history"].([]any)
	if len(history) != 2 {
		t.Fatalf("history = %+v", history)
	}
	for _, item := range history {
		message, _ := item.(map[string]any)
		if message["kind"] != "message" || message["messageId"] == "" {
			t.Fatalf("message = %+v", message)
		}
	}
}

func TestRPCInboundMessageContext(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	body := `{"jsonrpc":"2.0","id":1,"method":"message/send","params":{"message":{"role":"user","contextId":"ctx-inline","parts":[{"kind":"text","text":"hi"}]}}}`
	_, payload := doRPC(t, app, "", body)
	result, _ := payload["result"].(map[string]any)
	if result["contextId"] != "ctx-inline" {
		t.Fatalf("contextId = %v", result["contextId"])
	}
}

func TestRPCInboundMessageTaskIDResume(t *testing.T) {
	store := newTestStore(t)
	service := NewService(store, &fakeRunner{out: &channel.OutgoingMessage{Text: "resumed"}})
	handler := NewHandler(service)
	app := fiber.New()
	app.Post("/a2a", handler.RPC)

	if err := store.Create(context.Background(), Task{
		ID:       "resume-inline",
		Caller:   "anonymous",
		Status:   TaskStatus{State: StateInputRequired},
		Messages: []Message{{Role: "user", Parts: []Part{{Kind: "text", Text: "first"}}}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	body := `{"jsonrpc":"2.0","id":1,"method":"message/send","params":{"message":{"role":"user","taskId":"resume-inline","parts":[{"kind":"text","text":"answer"}]}}}`
	_, payload := doRPC(t, app, "", body)
	if payload["error"] != nil {
		t.Fatalf("payload = %+v", payload)
	}
	result, _ := payload["result"].(map[string]any)
	if result["id"] != "resume-inline" {
		t.Fatalf("id = %v", result["id"])
	}
	statusObj, _ := result["status"].(map[string]any)
	if statusObj["state"] != StateCompleted {
		t.Fatalf("status = %+v", statusObj)
	}
}
