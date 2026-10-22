package a2a

import (
	"testing"

	"myaaw/internal/channel"
	"myaaw/internal/config"

	"github.com/gofiber/fiber/v3"
)

const scopeTestSendBody = `{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"role":"user","parts":[{"text":"hi"}]}}}`

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
	if payload["error"] != "insufficient scope" {
		t.Fatalf("send error = %+v", payload)
	}

	status, payload = doRPC(t, app, "tok-read", `{"jsonrpc":"2.0","id":2,"method":"GetTask","params":{"id":"missing"}}`)
	if status != fiber.StatusOK {
		t.Fatalf("get status = %d, payload = %+v", status, payload)
	}
	errObj, _ := payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeTaskNotFound) {
		t.Fatalf("get error = %+v", payload)
	}
}

func TestRPCLegacyMethodsRejected(t *testing.T) {
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "aliased"}})

	for _, method := range []string{"tasks/send", "message/send", "tasks/get", "tasks/sendSubscribe", "tasks/resubscribe", "tasks/pushNotificationConfig/set"} {
		status, payload := doRPC(t, app, "", `{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":{}}`)
		if status != fiber.StatusOK {
			t.Fatalf("%s status = %d, payload = %+v", method, status, payload)
		}
		errObj, _ := payload["error"].(map[string]any)
		if errObj == nil || errObj["code"] != float64(CodeMethodNotFound) {
			t.Fatalf("%s error = %+v", method, payload)
		}
	}
}

func TestRPCTaskOwnership(t *testing.T) {
	setTestCallers(t, []string{"alice:tok-alice:delegate", "bob:tok-bob:delegate"})
	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	status, payload := doRPC(t, app, "tok-alice", scopeTestSendBody)
	if status != fiber.StatusOK || payload["error"] != nil {
		t.Fatalf("send status = %d, payload = %+v", status, payload)
	}
	task := resultTask(t, payload)
	taskID, _ := task["id"].(string)
	if taskID == "" {
		t.Fatalf("missing task id: %+v", payload)
	}

	body := `{"jsonrpc":"2.0","id":2,"method":"GetTask","params":{"id":"` + taskID + `"}}`
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
