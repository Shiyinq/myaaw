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
	app.Get("/.well-known/agent-card.json", handler.Card)
	app.Post("/a2a", handler.RPC)
	mountRest(app, handler)
	return app
}

func doRPC(t *testing.T, app *fiber.App, token, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("A2A-Version", "1.0")
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

const v1SendBody = `{
	"jsonrpc":"2.0","id":1,"method":"SendMessage",
	"params":{"message":{"role":"user","parts":[{"text":"hello"}]}}
}`

func resultTask(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	result, _ := payload["result"].(map[string]any)
	if result == nil {
		t.Fatalf("missing result: %+v", payload)
	}
	task, _ := result["task"].(map[string]any)
	if task == nil {
		t.Fatalf("missing result.task: %+v", result)
	}
	return task
}

func TestRPCSendAndGet(t *testing.T) {
	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "remote answer", ConversationID: "conv-1"}}
	app := newTestApp(t, runner)

	status, payload := doRPC(t, app, "", v1SendBody)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, payload = %+v", status, payload)
	}
	task := resultTask(t, payload)
	if task["contextId"] != "conv-1" {
		t.Fatalf("contextId = %v", task["contextId"])
	}
	statusObj, _ := task["status"].(map[string]any)
	if statusObj["state"] != WireState(StateCompleted) {
		t.Fatalf("status = %+v", statusObj)
	}
	artifacts, _ := task["artifacts"].([]any)
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %+v", artifacts)
	}
	if runner.input == nil || !strings.HasPrefix(runner.input.Text, "hello") || runner.input.Channel != "a2a" || runner.input.ConversationID == "" {
		t.Fatalf("runner input = %+v", runner.input)
	}

	taskID, _ := task["id"].(string)
	if taskID == "" {
		t.Fatal("missing task id")
	}
	status, payload = doRPC(t, app, "", `{"jsonrpc":"2.0","id":2,"method":"GetTask","params":{"id":"`+taskID+`"}}`)
	if status != fiber.StatusOK {
		t.Fatalf("get status = %d, payload = %+v", status, payload)
	}
	if payload["error"] != nil {
		t.Fatalf("get error = %+v", payload["error"])
	}
	got, _ := payload["result"].(map[string]any)
	if got["id"] != taskID {
		t.Fatalf("get result = %+v", got)
	}
}

func TestRPCFailedTaskAndMissingTask(t *testing.T) {
	runner := &fakeRunner{err: errors.New("boom")}
	app := newTestApp(t, runner)

	_, payload := doRPC(t, app, "", v1SendBody)
	task := resultTask(t, payload)
	statusObj, _ := task["status"].(map[string]any)
	if statusObj["state"] != WireState(StateFailed) {
		t.Fatalf("status = %+v", statusObj)
	}

	_, payload = doRPC(t, app, "", `{"jsonrpc":"2.0","id":2,"method":"GetTask","params":{"id":"missing"}}`)
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

func TestRPCVersionGate(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	req := httptest.NewRequest("POST", "/a2a", strings.NewReader(v1SendBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, _ := payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeVersionNotSupported) {
		t.Fatalf("missing version header error = %+v", payload)
	}

	status, payload := doRPC(t, app, "", v1SendBody)
	if status != fiber.StatusOK || payload["error"] != nil {
		t.Fatalf("v1.0 status = %d, payload = %+v", status, payload)
	}

	req = httptest.NewRequest("POST", "/a2a", strings.NewReader(v1SendBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("A2A-Version", "0.3")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	payload = nil
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, _ = payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeVersionNotSupported) {
		t.Fatalf("wrong version error = %+v", payload)
	}
}

func TestRPCAuth(t *testing.T) {
	previous := config.A2AToken
	config.A2AToken = "secret"
	t.Cleanup(func() { config.A2AToken = previous })

	runner := &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}}
	app := newTestApp(t, runner)

	status, _ := doRPC(t, app, "", v1SendBody)
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status without token = %d, want 401", status)
	}
	status, payload := doRPC(t, app, "secret", v1SendBody)
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
	if status, payload := doRPC(t, app, "caller-b", v1SendBody); status != fiber.StatusOK || payload["error"] != nil {
		t.Fatalf("caller-b status = %d, payload = %+v", status, payload)
	}
	if status, _ := doRPC(t, app, "caller-c", v1SendBody); status != fiber.StatusUnauthorized {
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
		"jsonrpc":"2.0","id":1,"method":"SendStreamingMessage",
		"params":{"message":{"role":"user","parts":[{"text":"hi"}]}}
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("A2A-Version", "1.0")
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
		`"task":`,
		WireState(StateWorking),
		`"text":"Hello "`,
		`"text":"Hello world"`,
		`"lastChunk":true`,
		WireState(StateCompleted),
		`"final":true`,
	} {
		if !strings.Contains(stream, want) {
			t.Errorf("stream missing %q:\n%s", want, stream)
		}
	}
	if runner.input == nil || runner.input.ConversationID == "" {
		t.Fatalf("runner input = %+v", runner.input)
	}
}

func TestCard(t *testing.T) {
	app := newTestApp(t, &fakeRunner{})
	req := httptest.NewRequest("GET", "/.well-known/agent-card.json", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if card.Name == "" {
		t.Fatalf("card = %+v", card)
	}
	if len(card.SupportedInterfaces) != 2 {
		t.Fatalf("supportedInterfaces = %+v", card.SupportedInterfaces)
	}
	iface := card.SupportedInterfaces[0]
	if !strings.HasSuffix(iface.URL, "/a2a") || iface.ProtocolBinding != "JSONRPC" || iface.ProtocolVersion != SupportedProtocolVersion {
		t.Fatalf("interface = %+v", iface)
	}
	restIface := card.SupportedInterfaces[1]
	if restIface.ProtocolBinding != "HTTP+JSON" || restIface.ProtocolVersion != SupportedProtocolVersion {
		t.Fatalf("rest interface = %+v", restIface)
	}
	if !card.Capabilities.Streaming {
		t.Fatal("card should advertise streaming")
	}
	if !card.Capabilities.PushNotifications {
		t.Fatal("card should advertise push notifications")
	}
	if card.Version != SupportedProtocolVersion {
		t.Fatalf("card version = %q, want %q", card.Version, SupportedProtocolVersion)
	}
	if len(card.DefaultInputModes) == 0 || len(card.DefaultOutputModes) == 0 || card.Skills == nil {
		t.Fatalf("required card fields missing: %+v", card)
	}
}

func TestCardCaching(t *testing.T) {
	app := newTestApp(t, &fakeRunner{})

	req := httptest.NewRequest("GET", "/.well-known/agent-card.json", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	etag := resp.Header.Get("ETag")
	resp.Body.Close()
	if etag == "" {
		t.Fatal("card response is missing an ETag")
	}
	if cacheControl := resp.Header.Get("Cache-Control"); !strings.Contains(cacheControl, "no-cache") {
		t.Fatalf("Cache-Control = %q", cacheControl)
	}

	req = httptest.NewRequest("GET", "/.well-known/agent-card.json", nil)
	req.Header.Set("If-None-Match", etag)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotModified {
		t.Fatalf("If-None-Match status = %d, want 304", resp.StatusCode)
	}

	req = httptest.NewRequest("GET", "/.well-known/agent-card.json", nil)
	req.Header.Set("If-None-Match", `"stale-etag"`)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("stale etag status = %d, want 200", resp.StatusCode)
	}
}

func TestRPCLegacyCardPathGone(t *testing.T) {
	app := newTestApp(t, &fakeRunner{})
	req := httptest.NewRequest("GET", "/.well-known/agent.json", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("legacy card path status = %d, want 404", resp.StatusCode)
	}
}

func TestRPCSubscribeTerminalTaskFails(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	_, payload := doRPC(t, app, "", v1SendBody)
	task := resultTask(t, payload)
	taskID, _ := task["id"].(string)
	if taskID == "" {
		t.Fatalf("missing task id: %+v", payload)
	}

	_, payload = doRPC(t, app, "", `{"jsonrpc":"2.0","id":2,"method":"SubscribeToTask","params":{"id":"`+taskID+`"}}`)
	errObj, _ := payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeUnsupportedOperation) {
		t.Fatalf("error = %+v", payload)
	}
}

func TestStreamBufferingReplaysAndForwards(t *testing.T) {
	service := NewService(newTestStore(t), &fakeRunner{})

	service.startStream("t1")
	service.publishResponse("t1", StreamResponse{Task: &Task{ID: "t1", Status: TaskStatus{State: StateWorking}}})

	snapshot, events, active, exists := service.subscribeStream("t1")
	if !active || !exists {
		t.Fatalf("active = %v, exists = %v", active, exists)
	}
	if len(snapshot) != 1 || snapshot[0].Task == nil {
		t.Fatalf("snapshot = %+v", snapshot)
	}

	service.publishResponse("t1", StreamResponse{StatusUpdate: &TaskStatusUpdateEvent{TaskID: "t1"}})
	select {
	case response := <-events:
		if response.StatusUpdate == nil {
			t.Fatalf("forwarded response = %+v", response)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for forwarded event")
	}

	service.finishStream("t1")
	select {
	case _, open := <-events:
		if open {
			t.Fatal("subscriber channel should be closed after finish")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for stream close")
	}

	snapshot, _, active, exists = service.subscribeStream("t1")
	if active || !exists {
		t.Fatalf("finished stream active = %v, exists = %v", active, exists)
	}
	if len(snapshot) != 2 {
		t.Fatalf("finished snapshot = %+v", snapshot)
	}
}

func TestRPCResponseShape(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	_, payload := doRPC(t, app, "", v1SendBody)
	task := resultTask(t, payload)
	history, _ := task["history"].([]any)
	if len(history) != 2 {
		t.Fatalf("history = %+v", task["history"])
	}
	for _, item := range history {
		message, _ := item.(map[string]any)
		if message["messageId"] == "" {
			t.Fatalf("message = %+v", message)
		}
		role, _ := message["role"].(string)
		if role != WireRole("user") && role != WireRole("agent") {
			t.Fatalf("role = %q", role)
		}
	}
	if _, ok := task["messages"]; ok {
		t.Fatal("task must not expose the legacy messages field")
	}
}

func TestRPCInboundMessageContext(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	body := `{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"role":"user","contextId":"ctx-inline","parts":[{"text":"hi"}]}}}`
	_, payload := doRPC(t, app, "", body)
	task := resultTask(t, payload)
	if task["contextId"] != "ctx-inline" {
		t.Fatalf("contextId = %v", task["contextId"])
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
		Messages: []Message{{Role: "user", Parts: []Part{{Text: "first"}}}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	body := `{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"role":"user","taskId":"resume-inline","parts":[{"text":"answer"}]}}}`
	_, payload := doRPC(t, app, "", body)
	if payload["error"] != nil {
		t.Fatalf("payload = %+v", payload)
	}
	task := resultTask(t, payload)
	if task["id"] != "resume-inline" {
		t.Fatalf("id = %v", task["id"])
	}
	statusObj, _ := task["status"].(map[string]any)
	if statusObj["state"] != WireState(StateCompleted) {
		t.Fatalf("status = %+v", statusObj)
	}
}

func TestRPCListTasks(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	for range 2 {
		if status, payload := doRPC(t, app, "", v1SendBody); status != fiber.StatusOK || payload["error"] != nil {
			t.Fatalf("send status = %d, payload = %+v", status, payload)
		}
	}

	status, payload := doRPC(t, app, "", `{"jsonrpc":"2.0","id":3,"method":"ListTasks","params":{"historyLength":0}}`)
	if status != fiber.StatusOK || payload["error"] != nil {
		t.Fatalf("list status = %d, payload = %+v", status, payload)
	}
	result, _ := payload["result"].(map[string]any)
	tasks, _ := result["tasks"].([]any)
	if len(tasks) != 2 {
		t.Fatalf("tasks = %+v", result["tasks"])
	}
	if result["totalSize"] != float64(2) {
		t.Fatalf("totalSize = %v", result["totalSize"])
	}
	for _, item := range tasks {
		task, _ := item.(map[string]any)
		if _, ok := task["history"]; ok {
			t.Fatalf("historyLength 0 should omit history: %+v", task)
		}
	}
}

func TestRPCCancelTask(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	_, payload := doRPC(t, app, "", v1SendBody)
	task := resultTask(t, payload)
	taskID, _ := task["id"].(string)
	if taskID == "" {
		t.Fatalf("missing task id: %+v", payload)
	}

	_, payload = doRPC(t, app, "", `{"jsonrpc":"2.0","id":2,"method":"CancelTask","params":{"id":"`+taskID+`"}}`)
	errObj, _ := payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeTaskNotCancelable) {
		t.Fatalf("error = %+v", payload)
	}
}
