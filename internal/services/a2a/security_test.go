package a2a

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/channel"
	"myaaw/internal/config"

	"github.com/gofiber/fiber/v3"
)

func writeExtendedCard(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "extended.json")
	content := `{"description":"Extended description","skills":[{"id":"secret","name":"Secret Skill","description":"Only for authenticated callers","tags":["secret"]}]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write extended card: %v", err)
	}
	return path
}

func TestRPCExtendedCard(t *testing.T) {
	path := writeExtendedCard(t)
	previous := config.A2AExtendedCardPath
	config.A2AExtendedCardPath = path
	t.Cleanup(func() { config.A2AExtendedCardPath = previous })

	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	// Public card advertises the capability.
	req := httptest.NewRequest("GET", "/.well-known/agent-card.json", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("card: %v", err)
	}
	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	resp.Body.Close()
	if !card.Capabilities.ExtendedAgentCard {
		t.Fatalf("card should advertise extendedAgentCard: %+v", card.Capabilities)
	}

	// JSON-RPC returns the merged card.
	_, payload := doRPC(t, app, "", `{"jsonrpc":"2.0","id":1,"method":"GetExtendedAgentCard"}`)
	if payload["error"] != nil {
		t.Fatalf("payload = %+v", payload)
	}
	result, _ := payload["result"].(map[string]any)
	if result["description"] != "Extended description" {
		t.Fatalf("description = %v", result["description"])
	}
	skills, _ := result["skills"].([]any)
	if len(skills) != 1 {
		t.Fatalf("skills should contain the extended skill: %+v", skills)
	}
	var hasSecret bool
	for _, skill := range skills {
		object, _ := skill.(map[string]any)
		if object["id"] == "secret" {
			hasSecret = true
		}
	}
	if !hasSecret {
		t.Fatalf("secret skill missing from %+v", skills)
	}
}

func TestRPCExtendedCardNotSupported(t *testing.T) {
	previous := config.A2AExtendedCardPath
	config.A2AExtendedCardPath = ""
	t.Cleanup(func() { config.A2AExtendedCardPath = previous })

	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})
	_, payload := doRPC(t, app, "", `{"jsonrpc":"2.0","id":1,"method":"GetExtendedAgentCard"}`)
	errObj, _ := payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeUnsupportedOperation) {
		t.Fatalf("error = %+v", payload)
	}
}

func TestRPCExtendedCardNotConfigured(t *testing.T) {
	previous := config.A2AExtendedCardPath
	config.A2AExtendedCardPath = filepath.Join(t.TempDir(), "missing.json")
	t.Cleanup(func() { config.A2AExtendedCardPath = previous })

	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})
	_, payload := doRPC(t, app, "", `{"jsonrpc":"2.0","id":1,"method":"GetExtendedAgentCard"}`)
	errObj, _ := payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(CodeExtendedAgentCardNotConfigured) {
		t.Fatalf("error = %+v", payload)
	}
}

func TestWebhookURLSSRFGuard(t *testing.T) {
	previous := config.A2AAllowPrivateWebhooks
	config.A2AAllowPrivateWebhooks = false
	t.Cleanup(func() { config.A2AAllowPrivateWebhooks = previous })

	for _, blocked := range []string{
		"http://127.0.0.1:8080/hook",
		"http://localhost/hook",
		"http://10.0.0.5/hook",
		"http://169.254.169.254/latest",
		"http://[::1]/hook",
		"http://100.64.1.1/hook",
	} {
		if _, err := validateWebhookURL(blocked); err == nil {
			t.Errorf("validateWebhookURL(%q) should be rejected", blocked)
		}
	}

	for _, allowed := range []string{
		"https://example.com/hook",
		"https://93.184.216.34/hook",
	} {
		if _, err := validateWebhookURL(allowed); err != nil {
			t.Errorf("validateWebhookURL(%q) = %v", allowed, err)
		}
	}

	config.A2AAllowPrivateWebhooks = true
	if _, err := validateWebhookURL("http://127.0.0.1:8080/hook"); err != nil {
		t.Errorf("private webhooks should be allowed when opted in: %v", err)
	}
}

func TestRPCRateLimit(t *testing.T) {
	previousRate := config.A2ARateLimit
	previousBurst := config.A2ARateBurst
	config.A2ARateLimit = 0.001
	config.A2ARateBurst = 1
	t.Cleanup(func() {
		config.A2ARateLimit = previousRate
		config.A2ARateBurst = previousBurst
	})

	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	if status, _ := doRPC(t, app, "", v1SendBody); status != fiber.StatusOK {
		t.Fatalf("first request status = %d, want 200", status)
	}
	status, _ := doRPC(t, app, "", v1SendBody)
	if status != fiber.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", status)
	}
}

func TestMergeSkillsAppendsAndDeduplicates(t *testing.T) {
	base := []any{
		map[string]any{"id": "a", "name": "A"},
		map[string]any{"id": "b", "name": "B"},
	}
	extra := []any{
		map[string]any{"id": "b", "name": "B (duplicate)"},
		map[string]any{"id": "c", "name": "C"},
	}
	merged := mergeSkills(base, extra)
	items, _ := merged.([]any)
	if len(items) != 3 {
		t.Fatalf("merged = %+v", merged)
	}
	ids := map[string]bool{}
	for _, item := range items {
		object := item.(map[string]any)
		ids[object["id"].(string)] = true
	}
	if !ids["a"] || !ids["b"] || !ids["c"] {
		t.Fatalf("ids = %+v", ids)
	}
}

func TestWebhookURLRejectsNonHTTP(t *testing.T) {
	if _, err := validateWebhookURL("ftp://example.com/hook"); err == nil {
		t.Fatal("ftp URL should be rejected")
	}
	if _, err := validateWebhookURL("not-a-url"); err == nil {
		t.Fatal("non-URL should be rejected")
	}
}
