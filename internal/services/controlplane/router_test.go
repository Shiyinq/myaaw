package controlplane

import (
	"database/sql"
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

func TestRouterReadOnlyManagement(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	agentRoot := filepath.Join(home, ".myaaw", "myfriend")

	agentConfig := `{
		"default_provider":"openai",
		"providers":{"openai":{"type":"openai","base_url":"https://api.openai.com","api_key":"sk-secret","default_model":"gpt-4o"}},
		"channels":{"telegram":{"active":true,"token":"telegram-secret","mode":"polling"}}
	}`
	if err := os.WriteFile(filepath.Join(agentRoot, "config", "config.json"), []byte(agentConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	logsDir := filepath.Join(agentRoot, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logsDir, "app.log"), []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	dbDir := filepath.Join(agentRoot, "database")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("mkdir database: %v", err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(dbDir, "myaaw.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE conversations (
		id TEXT PRIMARY KEY, user_id INTEGER, title TEXT, messages TEXT, active INTEGER, created_at DATETIME, updated_at DATETIME
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		"conv-1", 7, "First", `[{"role":"user","content":"hello"}]`, 1, time.Now().UTC(), time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	db.Close()

	app := mustNewApp(t)
	token := login(t, app)

	var providers struct {
		Default   string `json:"default_provider"`
		Providers []struct {
			Name   string `json:"name"`
			APIKey struct {
				Configured bool   `json:"configured"`
				Masked     string `json:"masked"`
			} `json:"api_key"`
		} `json:"providers"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/providers", token, "", fiber.StatusOK, &providers)
	if providers.Default != "openai" || len(providers.Providers) != 1 {
		t.Fatalf("providers = %+v", providers)
	}
	if !providers.Providers[0].APIKey.Configured || providers.Providers[0].APIKey.Masked != "********" {
		t.Fatalf("api key not masked: %+v", providers.Providers[0])
	}

	var channels struct {
		Channels []struct {
			Name  string `json:"name"`
			Token struct {
				Configured bool `json:"configured"`
			} `json:"token"`
		} `json:"channels"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/channels", token, "", fiber.StatusOK, &channels)
	if len(channels.Channels) != 1 || channels.Channels[0].Name != "telegram" || !channels.Channels[0].Token.Configured {
		t.Fatalf("channels = %+v", channels)
	}

	var logs struct {
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/logs", token, "", fiber.StatusOK, &logs)
	if len(logs.Files) != 1 || logs.Files[0].Name != "app.log" {
		t.Fatalf("logs = %+v", logs)
	}
	doJSON(t, app, "GET", "/agents/myfriend/logs/stream?file=../app.log", token, "", fiber.StatusBadRequest, nil)

	var conversations struct {
		Conversations []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"conversations"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/conversations", token, "", fiber.StatusOK, &conversations)
	if len(conversations.Conversations) != 1 || conversations.Conversations[0].ID != "conv-1" {
		t.Fatalf("conversations = %+v", conversations)
	}

	var conversation struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/conversations/conv-1", token, "", fiber.StatusOK, &conversation)
	if conversation.Title != "First" {
		t.Fatalf("conversation = %+v", conversation)
	}
}

func TestRouterWriteManagement(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)
	token := login(t, app)

	var provider struct {
		Name         string `json:"name"`
		DefaultModel string `json:"default_model"`
		APIKey       struct {
			Configured bool `json:"configured"`
		} `json:"api_key"`
	}
	doJSON(t, app, "POST", "/agents/myfriend/providers", token,
		`{"name":"openai","type":"openai","api_key":"sk-secret","default_model":"gpt-4o"}`,
		fiber.StatusCreated, &provider)
	if provider.Name != "openai" || !provider.APIKey.Configured {
		t.Fatalf("provider = %+v", provider)
	}

	doJSON(t, app, "PUT", "/agents/myfriend/providers/openai", token,
		`{"default_model":"gpt-4o-mini"}`, fiber.StatusOK, &provider)
	if provider.DefaultModel != "gpt-4o-mini" {
		t.Fatalf("provider = %+v", provider)
	}
	doJSON(t, app, "POST", "/agents/myfriend/providers/default", token,
		`{"name":"openai"}`, fiber.StatusOK, nil)
	doJSON(t, app, "DELETE", "/agents/myfriend/providers/openai", token, "", fiber.StatusOK, nil)
	doJSON(t, app, "DELETE", "/agents/myfriend/providers/missing", token, "", fiber.StatusNotFound, nil)

	var channel struct {
		Name  string `json:"name"`
		Token struct {
			Configured bool `json:"configured"`
		} `json:"token"`
	}
	doJSON(t, app, "PUT", "/agents/myfriend/channels/telegram", token,
		`{"active":true,"token":"tok","mode":"polling"}`, fiber.StatusOK, &channel)
	if channel.Name != "telegram" || !channel.Token.Configured {
		t.Fatalf("channel = %+v", channel)
	}
	doJSON(t, app, "DELETE", "/agents/myfriend/channels/telegram", token, "", fiber.StatusOK, nil)

	var created struct {
		Name string `json:"name"`
	}
	doJSON(t, app, "POST", "/agents", token, `{"name":"second"}`, fiber.StatusCreated, &created)
	if created.Name != "second" {
		t.Fatalf("created = %+v", created)
	}
	doJSON(t, app, "DELETE", "/agents/second", token, "", fiber.StatusOK, nil)
	doJSON(t, app, "DELETE", "/agents/myfriend", token, "", fiber.StatusBadRequest, nil)

	doJSON(t, app, "POST", "/agents/myfriend/default", token, "", fiber.StatusOK, nil)

	var status struct {
		Running bool `json:"running"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/server/status", token, "", fiber.StatusOK, &status)
	if status.Running {
		t.Fatalf("server should not be running: %+v", status)
	}

	var job struct {
		ID string `json:"id"`
	}
	doJSON(t, app, "POST", "/agents/myfriend/cron", token,
		`{"name":"Reminder","schedule_kind":"every","expr":"1h","message":"hello","channel":"telegram","to":"123"}`,
		fiber.StatusCreated, &job)
	if job.ID == "" {
		t.Fatal("cron job has no id")
	}

	var jobs struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/cron", token, "", fiber.StatusOK, &jobs)
	if len(jobs.Jobs) != 1 || jobs.Jobs[0].ID != job.ID {
		t.Fatalf("jobs = %+v", jobs)
	}
	doJSON(t, app, "DELETE", "/agents/myfriend/cron/"+job.ID, token, "", fiber.StatusOK, nil)
	doJSON(t, app, "DELETE", "/agents/myfriend/cron/missing", token, "", fiber.StatusNotFound, nil)
}

func TestRouterDocsRedirectsToIndex(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)

	req := httptest.NewRequest("GET", "/docs/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("status = %d, want 303", resp.StatusCode)
	}
	if location := resp.Header.Get("Location"); location != "/docs/index.html" {
		t.Fatalf("location = %q", location)
	}

	req = httptest.NewRequest("GET", "/docs/index.html", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("app.Test index: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "swagger-ui") {
		t.Fatal("index page does not look like Swagger UI")
	}
}

func TestAllowedOrigins(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "")
	if got := allowedOrigins(); len(got) != 1 || got[0] != "*" {
		t.Fatalf("origins = %v, want *", got)
	}

	t.Setenv("ALLOWED_ORIGINS", "https://a.example, https://b.example")
	got := allowedOrigins()
	if len(got) != 2 || got[0] != "https://a.example" || got[1] != "https://b.example" {
		t.Fatalf("origins = %v", got)
	}
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
