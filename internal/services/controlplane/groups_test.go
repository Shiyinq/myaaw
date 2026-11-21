package controlplane

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

func setupGroupTestAgents(t *testing.T, agents map[string]string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	reg := &agentdir.Registry{
		Agents: make(map[string]agentdir.AgentInfo, len(agents)),
	}
	for name, baseURL := range agents {
		if reg.Default == "" {
			reg.Default = name
		}
		agentRoot := filepath.Join(home, ".myaaw", "agents", name)
		for _, dir := range []string{
			filepath.Join(agentRoot, "home"),
			filepath.Join(agentRoot, "config"),
		} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", dir, err)
			}
		}
		if err := os.WriteFile(filepath.Join(agentRoot, "home", "AGENTS.md"), []byte("test"), 0o644); err != nil {
			t.Fatalf("write AGENTS.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(agentRoot, "config", "config.json"), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write config.json: %v", err)
		}
		env := fmt.Sprintf("MYAAW_BASE_URL=%s\n", baseURL)
		if err := os.WriteFile(filepath.Join(agentRoot, ".env"), []byte(env), 0o644); err != nil {
			t.Fatalf("write .env: %v", err)
		}
		reg.Agents[name] = agentdir.AgentInfo{AgentColor: "bleu"}
	}
	if err := agentdir.SaveRegistry(reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}
}

func TestGroupRoutesRequireAuth(t *testing.T) {
	setupGroupTestAgents(t, map[string]string{"alice": "http://127.0.0.1:1"})
	app := mustNewApp(t)

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/groups", ""},
		{"POST", "/groups", `{"name":"Team","members":["alice"]}`},
		{"GET", "/groups/abc", ""},
		{"PATCH", "/groups/abc", `{"name":"Team 2"}`},
		{"DELETE", "/groups/abc", ""},
		{"DELETE", "/groups/abc/messages", ""},
		{"POST", "/groups/abc/messages", `{"text":"hi"}`},
		{"POST", "/groups/abc/stop", ""},
		{"POST", "/groups/abc/continue", `{}`},
		{"GET", "/groups/abc/stream", ""},
	}
	for _, tc := range cases {
		doJSON(t, app, tc.method, tc.path, "", tc.body, fiber.StatusUnauthorized, nil)
	}
}

func TestGroupEndpointsLifecycle(t *testing.T) {
	setupGroupTestAgents(t, map[string]string{
		"alice": "http://127.0.0.1:1",
		"bob":   "http://127.0.0.1:1",
	})
	app := mustNewApp(t)
	token := login(t, app)

	// Empty list initially.
	var listResp struct {
		Groups []service.GroupSummary `json:"groups"`
	}
	doJSON(t, app, "GET", "/groups", token, "", fiber.StatusOK, &listResp)
	if len(listResp.Groups) != 0 {
		t.Fatalf("initial groups = %+v, want empty", listResp.Groups)
	}

	// Validation errors.
	doJSON(t, app, "POST", "/groups", token, `{"name":"","members":["alice"]}`, fiber.StatusBadRequest, nil)
	doJSON(t, app, "POST", "/groups", token, `{"name":"Bad","members":["ghost"]}`, fiber.StatusBadRequest, nil)
	doJSON(t, app, "POST", "/groups", token, `{"name":"Bad","members":["alice"],"discussion_mode":"invalid"}`, fiber.StatusBadRequest, nil)
	doJSON(t, app, "POST", "/groups", token, `{"name":"Bad","members":["alice"],"max_turns":99}`, fiber.StatusBadRequest, nil)

	// Create group.
	var created service.Group
	doJSON(t, app, "POST", "/groups", token, `{
		"name": "Architecture Council",
		"description": "Design reviews",
		"members": ["alice"],
		"discussion_mode": "mention",
		"max_turns": 6
	}`, fiber.StatusCreated, &created)
	if created.ID == "" || created.Name != "Architecture Council" || created.DiscussionMode != "mention" || created.MaxTurns != 6 {
		t.Fatalf("created = %+v", created)
	}
	if len(created.Members) != 1 || created.Members[0].Name != "alice" || created.Members[0].AgentColor != "bleu" {
		t.Fatalf("created members = %+v", created.Members)
	}

	// List groups includes created group.
	doJSON(t, app, "GET", "/groups", token, "", fiber.StatusOK, &listResp)
	if len(listResp.Groups) != 1 || listResp.Groups[0].ID != created.ID || listResp.Groups[0].MemberCount != 1 {
		t.Fatalf("list = %+v", listResp.Groups)
	}

	// PATCH group updates fields and adds a member.
	var patched service.Group
	doJSON(t, app, "PATCH", "/groups/"+created.ID, token, `{
		"name": "Council v2",
		"members": ["alice", "bob"],
		"max_turns": 10
	}`, fiber.StatusOK, &patched)
	if patched.Name != "Council v2" || len(patched.Members) != 2 || patched.MaxTurns != 10 {
		t.Fatalf("patched = %+v", patched)
	}

	// POST /groups/:id/messages validation.
	doJSON(t, app, "POST", "/groups/"+created.ID+"/messages", token, `{"text":"   "}`, fiber.StatusBadRequest, nil)
	doJSON(t, app, "POST", "/groups/"+created.ID+"/messages", token, `{"text":"hi","reply_to_id":"missing"}`, fiber.StatusBadRequest, nil)

	// POST /groups/:id/messages in mention mode without mention returns 202 and persists message.
	var sendResp struct {
		Message  service.GroupMessage  `json:"message"`
		RunState service.GroupRunState `json:"run_state"`
	}
	doJSON(t, app, "POST", "/groups/"+created.ID+"/messages", token, `{"text":"First note"}`, fiber.StatusAccepted, &sendResp)
	if sendResp.Message.ID == "" || sendResp.Message.SenderType != "user" || sendResp.RunState.Running {
		t.Fatalf("sendResp = %+v", sendResp)
	}

	// Send second message replying to the first with a selection quote.
	var replyResp struct {
		Message  service.GroupMessage  `json:"message"`
		RunState service.GroupRunState `json:"run_state"`
	}
	replyPayload := fmt.Sprintf(`{"text":"Follow up","reply_to_id":%q,"quotes":[{"id":"q1","text":"First","source":"answer"}]}`, sendResp.Message.ID)
	doJSON(t, app, "POST", "/groups/"+created.ID+"/messages", token, replyPayload, fiber.StatusAccepted, &replyResp)
	if replyResp.Message.ReplyPreview == nil || replyResp.Message.ReplyPreview.ID != sendResp.Message.ID || len(replyResp.Message.Quotes) != 1 {
		t.Fatalf("replyResp = %+v", replyResp)
	}

	// GET /groups/:id returns detail with 2 messages.
	var detail service.GroupDetail
	doJSON(t, app, "GET", "/groups/"+created.ID, token, "", fiber.StatusOK, &detail)
	if len(detail.Messages) != 2 || detail.Group.Name != "Council v2" {
		t.Fatalf("detail = %+v", detail)
	}

	// POST /groups/:id/stop when idle returns 200 {stopped: false}.
	var stopResp struct {
		Stopped  bool                  `json:"stopped"`
		RunState service.GroupRunState `json:"run_state"`
	}
	doJSON(t, app, "POST", "/groups/"+created.ID+"/stop", token, "", fiber.StatusOK, &stopResp)
	if stopResp.Stopped {
		t.Fatalf("expected stopped=false when idle")
	}

	// POST /groups/:id/continue when not paused returns 409.
	doJSON(t, app, "POST", "/groups/"+created.ID+"/continue", token, `{}`, fiber.StatusConflict, nil)

	// DELETE /groups/:id/messages clears history.
	var clearResp struct {
		Cleared bool `json:"cleared"`
	}
	doJSON(t, app, "DELETE", "/groups/"+created.ID+"/messages", token, "", fiber.StatusOK, &clearResp)
	if !clearResp.Cleared {
		t.Fatalf("expected cleared=true")
	}
	doJSON(t, app, "GET", "/groups/"+created.ID, token, "", fiber.StatusOK, &detail)
	if len(detail.Messages) != 0 {
		t.Fatalf("messages after clear = %d, want 0", len(detail.Messages))
	}

	// SSE stream returns 404 for unknown group.
	doJSON(t, app, "GET", "/groups/does-not-exist/stream", token, "", fiber.StatusNotFound, nil)

	// DELETE /groups/:id removes group.
	var delResp struct {
		Deleted bool `json:"deleted"`
	}
	doJSON(t, app, "DELETE", "/groups/"+created.ID, token, "", fiber.StatusOK, &delResp)
	if !delResp.Deleted {
		t.Fatalf("expected deleted=true")
	}
	doJSON(t, app, "GET", "/groups/"+created.ID, token, "", fiber.StatusNotFound, nil)
}

func TestGroupStreamEmitsInitialStatus(t *testing.T) {
	setupGroupTestAgents(t, map[string]string{"alice": "http://127.0.0.1:1"})
	app := mustNewApp(t)
	token := login(t, app)

	var created service.Group
	doJSON(t, app, "POST", "/groups", token, `{"name":"SSE Room","members":["alice"]}`, fiber.StatusCreated, &created)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Cancel right after deleting the group so the stream closes cleanly under app.Test.
	go func() {
		time.Sleep(150 * time.Millisecond)
		doJSON(t, app, "DELETE", "/groups/"+created.ID, token, "", fiber.StatusOK, nil)
	}()

	req := httptest.NewRequest(http.MethodGet, "/groups/"+created.ID+"/stream", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "event: status") || !strings.Contains(joined, created.ID) {
		t.Fatalf("stream output missing initial status event:\n%s", joined)
	}
}
