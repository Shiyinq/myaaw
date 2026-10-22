package a2a

import (
	"testing"

	"myaaw/internal/channel"
	"myaaw/internal/config"

	"github.com/gofiber/fiber/v3"
)

const scopeTestSendBody = `{"jsonrpc":"2.0","id":1,"method":"tasks/send","params":{"message":{"role":"user","parts":[{"kind":"text","text":"hi"}]}}}`

func setTestCallers(t *testing.T, entries []string) {
	t.Helper()
	previousToken := config.A2AToken
	previousTokens := config.A2ATokens
	previousCallers := config.A2ACallers
	config.A2AToken = ""
	config.A2ATokens = nil
	config.A2ACallers = entries
	t.Cleanup(func() {
		config.A2AToken = previousToken
		config.A2ATokens = previousTokens
		config.A2ACallers = previousCallers
	})
}

func TestRPCReadOnlyScope(t *testing.T) {
	setTestCallers(t, []string{"monitor:tok-read:read"})
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	status, payload := doRPC(t, app, "tok-read", scopeTestSendBody)
	if status != fiber.StatusForbidden {
		t.Fatalf("send status = %d, payload = %+v", status, payload)
	}
	errObj, _ := payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeForbidden) {
		t.Fatalf("send error = %+v", payload)
	}

	status, payload = doRPC(t, app, "tok-read", `{"jsonrpc":"2.0","id":2,"method":"tasks/get","params":{"id":"missing"}}`)
	if status != fiber.StatusOK {
		t.Fatalf("get status = %d, payload = %+v", status, payload)
	}
	errObj, _ = payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeTaskNotFound) {
		t.Fatalf("get error = %+v", payload)
	}
}

func TestRPCMethodAliases(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "aliased"}})

	status, payload := doRPC(t, app, "", `{"jsonrpc":"2.0","id":1,"method":"message/send","params":{"message":{"role":"user","parts":[{"kind":"text","text":"hi"}]}}}`)
	if status != fiber.StatusOK || payload["error"] != nil {
		t.Fatalf("status = %d, payload = %+v", status, payload)
	}
	result, _ := payload["result"].(map[string]any)
	if result == nil {
		t.Fatalf("missing result: %+v", payload)
	}
	statusObj, _ := result["status"].(map[string]any)
	if statusObj["state"] != StateCompleted {
		t.Fatalf("status = %+v", statusObj)
	}
}

func TestRPCTaskOwnership(t *testing.T) {
	setTestCallers(t, []string{"alice:tok-alice:delegate", "bob:tok-bob:delegate"})
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	status, payload := doRPC(t, app, "tok-alice", scopeTestSendBody)
	if status != fiber.StatusOK || payload["error"] != nil {
		t.Fatalf("send status = %d, payload = %+v", status, payload)
	}
	result, _ := payload["result"].(map[string]any)
	taskID, _ := result["id"].(string)
	if taskID == "" {
		t.Fatalf("missing task id: %+v", payload)
	}

	body := `{"jsonrpc":"2.0","id":2,"method":"tasks/get","params":{"id":"` + taskID + `"}}`
	status, payload = doRPC(t, app, "tok-alice", body)
	if status != fiber.StatusOK || payload["error"] != nil {
		t.Fatalf("owner get status = %d, payload = %+v", status, payload)
	}

	status, payload = doRPC(t, app, "tok-bob", body)
	if status != fiber.StatusOK {
		t.Fatalf("other get status = %d", status)
	}
	errObj, _ := payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeTaskNotFound) {
		t.Fatalf("other get payload = %+v", payload)
	}
}
