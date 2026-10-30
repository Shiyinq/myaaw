package a2a

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"myaaw/internal/channel"

	"github.com/gofiber/fiber/v3"
)

func doREST(t *testing.T, app *fiber.App, method, path, token, body string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/a2a+json")
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
		// Allow empty bodies (e.g. DELETE responses).
		payload = map[string]any{}
	}
	return resp.StatusCode, payload
}

func TestRestSendAndGetTask(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "hello", ConversationID: "conv-rest"}})

	status, payload := doREST(t, app, "POST", "/message:send", "", `{"message":{"role":"ROLE_USER","parts":[{"text":"hi"}]}}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, payload = %+v", status, payload)
	}
	if payload["error"] != nil {
		t.Fatalf("payload = %+v", payload)
	}
	task, _ := payload["task"].(map[string]any)
	if task == nil {
		t.Fatalf("missing task: %+v", payload)
	}
	if task["contextId"] != "conv-rest" {
		t.Fatalf("contextId = %v", task["contextId"])
	}
	statusObj, _ := task["status"].(map[string]any)
	if statusObj["state"] != WireState(StateCompleted) {
		t.Fatalf("status = %+v", statusObj)
	}

	taskID, _ := task["id"].(string)
	status, payload = doREST(t, app, "GET", "/tasks/"+taskID, "", "")
	if status != fiber.StatusOK {
		t.Fatalf("get status = %d, payload = %+v", status, payload)
	}
	if payload["id"] != taskID {
		t.Fatalf("get payload = %+v", payload)
	}
}

func TestRestListTasksQueryParams(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	for range 2 {
		if status, payload := doREST(t, app, "POST", "/message:send", "", `{"message":{"role":"ROLE_USER","parts":[{"text":"hi"}]}}`); status != 200 {
			t.Fatalf("send status = %d, payload = %+v", status, payload)
		}
	}

	status, payload := doREST(t, app, "GET", "/tasks?historyLength=0&pageSize=1", "", "")
	if status != fiber.StatusOK {
		t.Fatalf("list status = %d, payload = %+v", status, payload)
	}
	tasks, _ := payload["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %+v", payload["tasks"])
	}
	if payload["pageSize"] != float64(1) || payload["totalSize"] != float64(2) {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestRestCancelTask(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	_, payload := doREST(t, app, "POST", "/message:send", "", `{"message":{"role":"ROLE_USER","parts":[{"text":"hi"}]}}`)
	task, _ := payload["task"].(map[string]any)
	taskID, _ := task["id"].(string)

	status, payload := doREST(t, app, "POST", "/tasks/"+taskID+":cancel", "", "{}")
	if status != fiber.StatusBadRequest {
		t.Fatalf("cancel terminal status = %d, payload = %+v", status, payload)
	}
	errBody, _ := payload["error"].(map[string]any)
	if errBody == nil || errBody["code"] != float64(400) {
		t.Fatalf("error = %+v", payload)
	}
}

func TestRestVersionRequired(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	req := httptest.NewRequest("POST", "/message:send", strings.NewReader(`{"message":{"role":"ROLE_USER","parts":[{"text":"hi"}]}}`))
	req.Header.Set("Content-Type", "application/a2a+json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	var payload map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, payload = %+v", resp.StatusCode, payload)
	}
	errBody, _ := payload["error"].(map[string]any)
	if errBody["code"] != float64(400) {
		t.Fatalf("error = %+v", payload)
	}
}

func TestRestErrorShape(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	status, payload := doREST(t, app, "GET", "/tasks/missing", "", "")
	if status != fiber.StatusNotFound {
		t.Fatalf("status = %d, payload = %+v", status, payload)
	}
	errBody, _ := payload["error"].(map[string]any)
	if errBody == nil || errBody["code"] != float64(404) || errBody["status"] != "NOT_FOUND" {
		t.Fatalf("error = %+v", payload)
	}
	details, _ := errBody["details"].([]any)
	if len(details) != 1 {
		t.Fatalf("details = %+v", errBody["details"])
	}
	detail, _ := details[0].(map[string]any)
	if detail["reason"] != "TASK_NOT_FOUND" || detail["domain"] != "a2a-protocol.org" {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestRestPushConfigCRUD(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	_, payload := doREST(t, app, "POST", "/message:send", "", `{"message":{"role":"ROLE_USER","parts":[{"text":"hi"}]}}`)
	task, _ := payload["task"].(map[string]any)
	taskID, _ := task["id"].(string)

	status, payload := doREST(t, app, "POST", "/tasks/"+taskID+"/pushNotificationConfigs", "", `{"url":"https://example.com/hook","token":"hook-token"}`)
	if status != 200 || payload["error"] != nil {
		t.Fatalf("create status = %d, payload = %+v", status, payload)
	}
	configID, _ := payload["id"].(string)
	if configID == "" || payload["url"] != "https://example.com/hook" {
		t.Fatalf("config = %+v", payload)
	}

	status, payload = doREST(t, app, "GET", "/tasks/"+taskID+"/pushNotificationConfigs", "", "")
	if status != 200 {
		t.Fatalf("list status = %d, payload = %+v", status, payload)
	}
	configs, _ := payload["configs"].([]any)
	if len(configs) != 1 {
		t.Fatalf("configs = %+v", configs)
	}

	status, payload = doREST(t, app, "GET", "/tasks/"+taskID+"/pushNotificationConfigs/"+configID, "", "")
	if status != 200 || payload["id"] != configID {
		t.Fatalf("get status = %d, payload = %+v", status, payload)
	}

	status, _ = doREST(t, app, "DELETE", "/tasks/"+taskID+"/pushNotificationConfigs/"+configID, "", "")
	if status != 200 {
		t.Fatalf("delete status = %d", status)
	}

	status, payload = doREST(t, app, "GET", "/tasks/"+taskID+"/pushNotificationConfigs", "", "")
	if status != 200 {
		t.Fatalf("list after delete status = %d", status)
	}
	configs, _ = payload["configs"].([]any)
	if len(configs) != 0 {
		t.Fatalf("configs after delete = %+v", configs)
	}
}

func TestRestStreaming(t *testing.T) {
	runner := &fakeRunner{
		out: &channel.OutgoingMessage{Text: "Hello world"},
		streamChunks: []channel.StreamChunk{
			{Kind: "text", Text: "Hello "},
			{Kind: "text", Text: "world"},
		},
	}
	app := newTestApp(t, runner)

	req := httptest.NewRequest("POST", "/message:stream", strings.NewReader(`{"message":{"role":"ROLE_USER","parts":[{"text":"hi"}]}}`))
	req.Header.Set("Content-Type", "application/a2a+json")
	req.Header.Set("A2A-Version", "1.0")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
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
	if strings.Contains(stream, `"jsonrpc"`) {
		t.Fatalf("REST stream must not wrap events in JSON-RPC envelopes:\n%s", stream)
	}
}

func TestRestSubscribeTerminalFails(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	_, payload := doREST(t, app, "POST", "/message:send", "", `{"message":{"role":"ROLE_USER","parts":[{"text":"hi"}]}}`)
	task, _ := payload["task"].(map[string]any)
	taskID, _ := task["id"].(string)

	status, payload := doREST(t, app, "POST", "/tasks/"+taskID+":subscribe", "", "")
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, payload = %+v", status, payload)
	}
	errBody, _ := payload["error"].(map[string]any)
	details, _ := errBody["details"].([]any)
	if len(details) != 1 {
		t.Fatalf("error = %+v", payload)
	}
	detail, _ := details[0].(map[string]any)
	if detail["reason"] != "UNSUPPORTED_OPERATION" {
		t.Fatalf("detail = %+v", detail)
	}
}
