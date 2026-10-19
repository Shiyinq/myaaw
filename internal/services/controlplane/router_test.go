package controlplane

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myaaw/internal/agentdir"

	"github.com/gofiber/fiber/v3"
)

func TestRouterRequiresAuth(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)

	req := httptest.NewRequest("GET", "/agents", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestRouterRejectsWrongPassword(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)

	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"username":"myaaw","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestRouterListsAgentsAndProxiesChat(t *testing.T) {
	upstream := newUpstream(t)
	setupTestAgent(t, upstream.URL)
	app := mustNewApp(t)
	token := login(t, app)

	var list struct {
		Agents []struct {
			Name    string `json:"name"`
			Default bool   `json:"default"`
			Running bool   `json:"running"`
			BaseURL string `json:"base_url"`
		} `json:"agents"`
	}
	doJSON(t, app, "GET", "/agents", token, "", fiber.StatusOK, &list)
	if len(list.Agents) != 1 {
		t.Fatalf("agents = %+v", list.Agents)
	}
	agent := list.Agents[0]
	if agent.Name != "myfriend" || !agent.Default || !agent.Running {
		t.Fatalf("agent = %+v", agent)
	}
	if agent.BaseURL != upstream.URL {
		t.Fatalf("base_url = %q, want %q", agent.BaseURL, upstream.URL)
	}

	var chat struct {
		Text     string `json:"text"`
		Timeline []struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"timeline"`
	}
	doJSON(t, app, "POST", "/agents/myfriend/chat", token, `{"user_id":1,"text":"hi"}`, fiber.StatusOK, &chat)
	if chat.Text != "ok" || len(chat.Timeline) != 1 || chat.Timeline[0].Kind != "text" {
		t.Fatalf("chat response = %+v", chat)
	}
}

func TestRouterProxiesChatStream(t *testing.T) {
	upstream := newUpstream(t)
	setupTestAgent(t, upstream.URL)
	app := mustNewApp(t)
	token := login(t, app)

	req := httptest.NewRequest("POST", "/agents/myfriend/chat/stream", strings.NewReader(`{"user_id":1,"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	stream := string(body)
	for _, want := range []string{"event: thought", "event: done", `"text":"ok"`} {
		if !strings.Contains(stream, want) {
			t.Errorf("stream missing %q:\n%s", want, stream)
		}
	}
}

func TestRouterServesControlPlaneSwagger(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)

	req := httptest.NewRequest("GET", "/docs/doc.json", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	doc := string(body)
	for _, want := range []string{
		"Myaaw Control Plane API",
		"/agents",
		"/auth/login",
		"BearerAuth",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("swagger doc missing %q", want)
		}
	}
	if strings.Contains(doc, "/webhook/bot") {
		t.Error("control plane swagger should not include agent webhook routes")
	}
}

func TestRouterFilesystem(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)
	token := login(t, app)

	var roots struct {
		Path    string `json:"path"`
		Entries []struct {
			Name  string `json:"name"`
			Path  string `json:"path"`
			IsDir bool   `json:"is_dir"`
		} `json:"entries"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/fs", token, "", fiber.StatusOK, &roots)
	if len(roots.Entries) != 3 || roots.Entries[0].Path != "/home" {
		t.Fatalf("roots = %+v", roots)
	}

	doJSON(t, app, "POST", "/agents/myfriend/fs/write", token,
		`{"path":"/home/AGENTS.md","content":"# Updated"}`, fiber.StatusOK, nil)

	var read struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/fs/read?path=/home/AGENTS.md", token, "", fiber.StatusOK, &read)
	if read.Content != "# Updated" {
		t.Fatalf("content = %q", read.Content)
	}

	var list struct {
		Entries []struct {
			Name string `json:"name"`
		} `json:"entries"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/fs?path=/home", token, "", fiber.StatusOK, &list)
	if len(list.Entries) != 1 || list.Entries[0].Name != "AGENTS.md" {
		t.Fatalf("entries = %+v", list.Entries)
	}

	doJSON(t, app, "GET", "/agents/myfriend/fs/read?path=/etc/passwd", token, "", fiber.StatusBadRequest, nil)
	doJSON(t, app, "DELETE", "/agents/myfriend/fs?path=/home", token, "", fiber.StatusBadRequest, nil)
}

func TestRouterRejectsUnknownAgent(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)
	token := login(t, app)

	req := httptest.NewRequest("POST", "/agents/missing/chat", strings.NewReader(`{"user_id":1,"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func mustNewApp(t *testing.T) *fiber.App {
	t.Helper()
	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	return app
}

func login(t *testing.T, app *fiber.App) string {
	t.Helper()
	var out struct {
		Token string `json:"token"`
	}
	doJSON(t, app, "POST", "/auth/login", "", `{"username":"myaaw","password":"myaawadmin"}`, fiber.StatusOK, &out)
	if out.Token == "" {
		t.Fatal("login returned an empty token")
	}
	return out.Token
}

func doJSON(t *testing.T, app *fiber.App, method, path, token, body string, wantStatus int, out any) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, want %d, body = %s", method, path, resp.StatusCode, wantStatus, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("unmarshal %s: %v, body = %s", path, err, data)
		}
	}
}

func setupTestAgent(t *testing.T, baseURL string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	agentRoot := filepath.Join(home, ".myaaw", "myfriend")
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

	registry := &agentdir.Registry{
		Default: "myfriend",
		Agents:  map[string]agentdir.AgentInfo{"myfriend": {}},
	}
	if err := agentdir.SaveRegistry(registry); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}
}

func newUpstream(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"ok","timeline":[{"kind":"text","text":"ok"}]}`))
	})
	mux.HandleFunc("/api/chat/stream", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = fmt.Fprint(w, "event: thought\ndata: {\"text\":\"hmm\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = fmt.Fprint(w, "event: done\ndata: {\"ok\":true,\"text\":\"ok\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}
