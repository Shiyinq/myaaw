package a2a

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"myaaw/internal/channel"
)

func TestPushConfigStore(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.SetPushConfig(ctx, "task-1", PushNotificationConfig{}); err == nil {
		t.Fatal("empty config should fail")
	}

	saved, err := store.SetPushConfig(ctx, "task-1", PushNotificationConfig{
		ID:    "config-1",
		URL:   "https://example.com/hook",
		Token: "hook-token",
	})
	if err != nil {
		t.Fatalf("SetPushConfig: %v", err)
	}
	if saved.ID != "config-1" {
		t.Fatalf("saved = %+v", saved)
	}

	if _, err := store.SetPushConfig(ctx, "task-1", PushNotificationConfig{
		ID:  "config-1",
		URL: "https://example.com/hook-2",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	configs, err := store.ListPushConfigs(ctx, "task-1")
	if err != nil || len(configs) != 1 {
		t.Fatalf("configs = %+v, err = %v", configs, err)
	}
	if configs[0].URL != "https://example.com/hook-2" || configs[0].Token != "" {
		t.Fatalf("config = %+v", configs[0])
	}

	if err := store.DeletePushConfig(ctx, "task-1", "config-1"); err != nil {
		t.Fatalf("DeletePushConfig: %v", err)
	}
	if _, err := store.GetPushConfig(ctx, "task-1", "config-1"); !errors.Is(err, ErrPushConfigNotFound) {
		t.Fatalf("err = %v, want ErrPushConfigNotFound", err)
	}
}

func TestRPCPushNotificationConfig(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	_, payload := doRPC(t, app, "", scopeTestSendBody)
	result, _ := payload["result"].(map[string]any)
	taskID, _ := result["id"].(string)
	if taskID == "" {
		t.Fatalf("missing task id: %+v", payload)
	}

	status, payload := doRPC(t, app, "", `{"jsonrpc":"2.0","id":2,"method":"tasks/pushNotificationConfig/set","params":{"id":"`+taskID+`","pushNotificationConfig":{"url":"https://example.com/hook","token":"hook-token"}}}`)
	if status != 200 || payload["error"] != nil {
		t.Fatalf("set status = %d, payload = %+v", status, payload)
	}
	config, _ := payload["result"].(map[string]any)
	configID, _ := config["id"].(string)
	if configID == "" || config["url"] != "https://example.com/hook" {
		t.Fatalf("config = %+v", config)
	}

	status, payload = doRPC(t, app, "", `{"jsonrpc":"2.0","id":3,"method":"tasks/pushNotificationConfig/list","params":{"id":"`+taskID+`"}}`)
	if status != 200 || payload["error"] != nil {
		t.Fatalf("list status = %d, payload = %+v", status, payload)
	}
	configs, _ := payload["result"].([]any)
	if len(configs) != 1 {
		t.Fatalf("configs = %+v", configs)
	}

	status, payload = doRPC(t, app, "", `{"jsonrpc":"2.0","id":4,"method":"tasks/pushNotificationConfig/get","params":{"id":"`+taskID+`","pushNotificationConfigId":"`+configID+`"}}`)
	if status != 200 || payload["error"] != nil {
		t.Fatalf("get status = %d, payload = %+v", status, payload)
	}

	status, payload = doRPC(t, app, "", `{"jsonrpc":"2.0","id":5,"method":"tasks/pushNotificationConfig/delete","params":{"id":"`+taskID+`","pushNotificationConfigId":"`+configID+`"}}`)
	if status != 200 || payload["error"] != nil {
		t.Fatalf("delete status = %d, payload = %+v", status, payload)
	}

	status, payload = doRPC(t, app, "", `{"jsonrpc":"2.0","id":6,"method":"tasks/pushNotificationConfig/list","params":{"id":"`+taskID+`"}}`)
	if status != 200 {
		t.Fatalf("list after delete status = %d", status)
	}
	configs, _ = payload["result"].([]any)
	if len(configs) != 0 {
		t.Fatalf("configs after delete = %+v", configs)
	}
}

func TestRPCPushConfigOwnership(t *testing.T) {
	setTestCallers(t, []string{"alice:tok-alice:delegate", "bob:tok-bob:delegate"})
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	_, payload := doRPC(t, app, "tok-alice", scopeTestSendBody)
	result, _ := payload["result"].(map[string]any)
	taskID, _ := result["id"].(string)
	if taskID == "" {
		t.Fatalf("missing task id: %+v", payload)
	}

	status, payload := doRPC(t, app, "tok-bob", `{"jsonrpc":"2.0","id":2,"method":"tasks/pushNotificationConfig/set","params":{"id":"`+taskID+`","pushNotificationConfig":{"url":"https://example.com/hook"}}}`)
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	errObj, _ := payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeTaskNotFound) {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestNotifyTaskSendsConfiguredWebhooks(t *testing.T) {
	type received struct {
		auth string
		task Task
	}
	notifications := make(chan received, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var task Task
		_ = json.NewDecoder(r.Body).Decode(&task)
		notifications <- received{auth: r.Header.Get("Authorization"), task: task}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newTestStore(t)
	service := NewService(store, &fakeRunner{})
	ctx := context.Background()

	if err := store.Create(ctx, Task{ID: "notify-1", Status: TaskStatus{State: StateCompleted}}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := store.SetPushConfig(ctx, "notify-1", PushNotificationConfig{
		ID:    "config-1",
		URL:   server.URL,
		Token: "hook-token",
	}); err != nil {
		t.Fatalf("SetPushConfig: %v", err)
	}

	service.notifyTask("notify-1")

	select {
	case got := <-notifications:
		if got.auth != "Bearer hook-token" {
			t.Fatalf("authorization = %q", got.auth)
		}
		if got.task.ID != "notify-1" || got.task.Status.State != StateCompleted {
			t.Fatalf("task = %+v", got.task)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook")
	}
}
