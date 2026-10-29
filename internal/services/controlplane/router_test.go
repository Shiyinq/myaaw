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
	"strconv"
	"strings"
	"testing"
	"time"

	"myaaw/internal/agent/approvals"
	"myaaw/internal/agentdir"
	"myaaw/internal/obs"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"
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
		RunID    string `json:"run_id"`
		Timeline []struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"timeline"`
	}
	doJSON(t, app, "POST", "/agents/myfriend/chat", token, `{"user_id":1,"text":"hi"}`, fiber.StatusOK, &chat)
	if chat.Text != "ok" || chat.RunID != "run_upstream" || len(chat.Timeline) != 1 || chat.Timeline[0].Kind != "text" {
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
	for _, want := range []string{"event: thought", "event: done", `"text":"ok"`, `"run_id":"run_upstream"`} {
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

func TestRouterContext(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	agentHome := filepath.Join(home, ".myaaw", "myfriend", "home")
	if err := os.MkdirAll(filepath.Join(agentHome, "memory", "daily"), 0o755); err != nil {
		t.Fatalf("mkdir memory: %v", err)
	}
	for path, content := range map[string]string{
		filepath.Join(agentHome, "AGENTS.md"):          "# Agents",
		filepath.Join(agentHome, "memory", "RULES.md"): "# Rules",
		filepath.Join(agentHome, "NOTES.md"):           "# Not allowed",
		filepath.Join(agentHome, "BOOTSTRAP.md"):       "# Not allowed either",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	app := mustNewApp(t)
	token := login(t, app)

	type contextList struct {
		Files []struct {
			Name   string `json:"name"`
			Path   string `json:"path"`
			Folder string `json:"folder"`
			Size   int64  `json:"size"`
		} `json:"files"`
	}
	var list contextList
	doJSON(t, app, "GET", "/agents/myfriend/context", token, "", fiber.StatusOK, &list)
	if len(list.Files) != 2 {
		t.Fatalf("files = %+v", list.Files)
	}
	if list.Files[0].Path != "AGENTS.md" || list.Files[0].Folder != "root" {
		t.Fatalf("first file = %+v", list.Files[0])
	}
	if list.Files[1].Path != "memory/RULES.md" || list.Files[1].Folder != "memory" {
		t.Fatalf("second file = %+v", list.Files[1])
	}

	var read struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		Folder  string `json:"folder"`
		Content string `json:"content"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/context/file?path=memory/RULES.md", token, "", fiber.StatusOK, &read)
	if read.Path != "memory/RULES.md" || read.Folder != "memory" || read.Content != "# Rules" {
		t.Fatalf("read = %+v", read)
	}

	var written struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	doJSON(t, app, "PUT", "/agents/myfriend/context/file", token,
		`{"path":"memory/daily/2026-10-20.md","content":"# Log"}`, fiber.StatusOK, &written)
	if written.Path != "memory/daily/2026-10-20.md" || written.Content != "# Log" {
		t.Fatalf("written = %+v", written)
	}

	doJSON(t, app, "GET", "/agents/myfriend/context", token, "", fiber.StatusOK, &list)
	if len(list.Files) != 3 {
		t.Fatalf("files after write = %+v", list.Files)
	}

	doJSON(t, app, "GET", "/agents/myfriend/context/file?path=notes/todo.md", token, "", fiber.StatusBadRequest, nil)
	doJSON(t, app, "GET", "/agents/myfriend/context/file?path=../secret.md", token, "", fiber.StatusBadRequest, nil)
	doJSON(t, app, "GET", "/agents/myfriend/context/file?path=NOTES.md", token, "", fiber.StatusBadRequest, nil)
	doJSON(t, app, "GET", "/agents/myfriend/context/file?path=BOOTSTRAP.md", token, "", fiber.StatusBadRequest, nil)
	doJSON(t, app, "GET", "/agents/myfriend/context/file?path=MISSING.md", token, "", fiber.StatusBadRequest, nil)
	doJSON(t, app, "GET", "/agents/myfriend/context/file?path=HEARTBEAT.md", token, "", fiber.StatusNotFound, nil)
	doJSON(t, app, "PUT", "/agents/myfriend/context/file", token,
		`{"path":"NOTES.md","content":"x"}`, fiber.StatusBadRequest, nil)
}

func TestRouterSkillsAndTools(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	agentRoot := filepath.Join(home, ".myaaw", "myfriend")
	skillsDir := filepath.Join(agentRoot, "skills")
	if err := os.MkdirAll(filepath.Join(skillsDir, "weather"), 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "weather", "SKILL.md"), []byte("---\nname: Weather\ndescription: Fetch weather\n---\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	app := mustNewApp(t)
	token := login(t, app)

	type skillItem struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Source  string `json:"source"`
	}
	var skills struct {
		Skills []skillItem `json:"skills"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/skills", token, "", fiber.StatusOK, &skills)
	if len(skills.Skills) != 1 || skills.Skills[0].Name != "weather" || !skills.Skills[0].Enabled || skills.Skills[0].Source != "default" {
		t.Fatalf("skills = %+v", skills)
	}

	doJSON(t, app, "PATCH", "/agents/myfriend/skills/weather", token, `{"enabled":false}`, fiber.StatusOK, nil)
	doJSON(t, app, "GET", "/agents/myfriend/skills", token, "", fiber.StatusOK, &skills)
	if skills.Skills[0].Enabled || skills.Skills[0].Source != "config" {
		t.Fatalf("skills after toggle = %+v", skills)
	}

	// Removing the config override falls back to the frontmatter/default.
	doJSON(t, app, "DELETE", "/agents/myfriend/skills/weather/config", token, "", fiber.StatusOK, nil)
	doJSON(t, app, "GET", "/agents/myfriend/skills", token, "", fiber.StatusOK, &skills)
	if !skills.Skills[0].Enabled || skills.Skills[0].Source != "default" {
		t.Fatalf("skills after config reset = %+v", skills)
	}
	doJSON(t, app, "DELETE", "/agents/myfriend/skills/weather/config", token, "", fiber.StatusOK, nil)

	var skillDetail struct {
		Name    string `json:"name"`
		Content string `json:"content"`
		Files   []struct {
			Path string `json:"path"`
			Size int64  `json:"size"`
		} `json:"files"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/skills/weather", token, "", fiber.StatusOK, &skillDetail)
	if skillDetail.Name != "weather" || len(skillDetail.Files) != 1 || skillDetail.Files[0].Path != "SKILL.md" {
		t.Fatalf("skill detail = %+v", skillDetail)
	}
	if !strings.Contains(skillDetail.Content, "Fetch weather") {
		t.Fatalf("skill content = %q", skillDetail.Content)
	}

	var skillFile struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Binary  bool   `json:"binary"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/skills/weather/file?path=SKILL.md", token, "", fiber.StatusOK, &skillFile)
	if skillFile.Binary || !strings.Contains(skillFile.Content, "Fetch weather") {
		t.Fatalf("skill file = %+v", skillFile)
	}
	doJSON(t, app, "GET", "/agents/myfriend/skills/weather/file?path=../secret", token, "", fiber.StatusBadRequest, nil)
	doJSON(t, app, "GET", "/agents/myfriend/skills/weather/file?path=missing.md", token, "", fiber.StatusNotFound, nil)
	doJSON(t, app, "PATCH", "/agents/myfriend/skills/missing", token, `{"enabled":true}`, fiber.StatusNotFound, nil)

	// Simulate a terminal deletion: the list must update without any cache.
	if err := os.RemoveAll(filepath.Join(skillsDir, "weather")); err != nil {
		t.Fatalf("remove skill dir: %v", err)
	}
	doJSON(t, app, "GET", "/agents/myfriend/skills", token, "", fiber.StatusOK, &skills)
	if len(skills.Skills) != 0 {
		t.Fatalf("skills after terminal delete = %+v", skills)
	}
	// The detail GET cleans the stale skills.json key and still returns 404.
	doJSON(t, app, "GET", "/agents/myfriend/skills/weather", token, "", fiber.StatusNotFound, nil)
	doJSON(t, app, "DELETE", "/agents/myfriend/skills/weather", token, "", fiber.StatusNotFound, nil)

	type toolsView struct {
		Builtin []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"builtin"`
		Custom []struct {
			Name    string          `json:"name"`
			Command string          `json:"command"`
			Enabled bool            `json:"enabled"`
			Schema  json.RawMessage `json:"schema,omitempty"`
		} `json:"custom"`
		MCPServers []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
			Env     struct {
				Configured bool   `json:"configured"`
				Masked     string `json:"masked"`
			} `json:"env"`
		} `json:"mcp_servers"`
	}
	var tools toolsView
	doJSON(t, app, "GET", "/agents/myfriend/tools", token, "", fiber.StatusOK, &tools)
	if len(tools.Builtin) != 9 {
		t.Fatalf("builtin = %+v", tools.Builtin)
	}

	doJSON(t, app, "POST", "/agents/myfriend/tools/custom", token,
		`{"name":"hello","command":"node","args":["x.js"],"schema":{"type":"function","function":{"name":"hello"}}}`, fiber.StatusOK, nil)
	doJSON(t, app, "POST", "/agents/myfriend/tools/custom", token,
		`{"name":"HELLO","command":"node"}`, fiber.StatusConflict, nil)
	doJSON(t, app, "POST", "/agents/myfriend/tools/custom", token,
		`{"name":"broken","command":"node","schema":{"type":"object"}}`, fiber.StatusBadRequest, nil)
	doJSON(t, app, "PATCH", "/agents/myfriend/tools/custom/hello", token,
		`{"command":"python3","enabled":false}`, fiber.StatusOK, nil)
	doJSON(t, app, "PATCH", "/agents/myfriend/tools/custom/hello", token, `{}`, fiber.StatusBadRequest, nil)
	doJSON(t, app, "POST", "/agents/myfriend/tools/mcp", token,
		`{"name":"playwright","command":"npx","env":{"TOKEN":"secret"}}`, fiber.StatusOK, nil)
	doJSON(t, app, "PATCH", "/agents/myfriend/tools/mcp/playwright", token, `{}`, fiber.StatusBadRequest, nil)

	doJSON(t, app, "GET", "/agents/myfriend/tools", token, "", fiber.StatusOK, &tools)
	if len(tools.Custom) != 1 || tools.Custom[0].Command != "python3" || tools.Custom[0].Enabled {
		t.Fatalf("custom = %+v", tools.Custom)
	}
	if len(tools.Custom[0].Schema) == 0 {
		t.Fatalf("schema should be returned when present: %+v", tools.Custom[0])
	}
	if len(tools.MCPServers) != 1 || !tools.MCPServers[0].Env.Configured || tools.MCPServers[0].Env.Masked != "********" {
		t.Fatalf("mcp = %+v", tools.MCPServers)
	}

	doJSON(t, app, "PATCH", "/agents/myfriend/tools/builtin/bash", token, `{"enabled":false}`, fiber.StatusOK, nil)
	doJSON(t, app, "PATCH", "/agents/myfriend/tools/builtin/nope", token, `{"enabled":false}`, fiber.StatusBadRequest, nil)
	doJSON(t, app, "GET", "/agents/myfriend/tools", token, "", fiber.StatusOK, &tools)
	for _, tool := range tools.Builtin {
		if tool.Name == "bash" && tool.Enabled {
			t.Fatalf("bash should be disabled: %+v", tools.Builtin)
		}
	}

	doJSON(t, app, "DELETE", "/agents/myfriend/tools/mcp/playwright", token, "", fiber.StatusOK, nil)
	doJSON(t, app, "DELETE", "/agents/myfriend/tools/mcp/playwright", token, "", fiber.StatusNotFound, nil)
	doJSON(t, app, "DELETE", "/agents/myfriend/tools/custom/hello", token, "", fiber.StatusOK, nil)

	// Simulate a terminal edit of tools.json: the next list reads it fresh.
	toolsDir := filepath.Join(agentRoot, "tools")
	if err := os.WriteFile(filepath.Join(toolsDir, "tools.json"),
		[]byte(`{"builtin_tools":{"bash":false},"custom_tools":[{"name":"terminal-tool","command":"echo"}],"mcp_servers":[]}`), 0o644); err != nil {
		t.Fatalf("rewrite tools.json: %v", err)
	}
	doJSON(t, app, "GET", "/agents/myfriend/tools", token, "", fiber.StatusOK, &tools)
	if len(tools.Custom) != 1 || tools.Custom[0].Name != "terminal-tool" {
		t.Fatalf("custom after terminal edit = %+v", tools.Custom)
	}
}

func TestRouterContacts(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)
	token := login(t, app)

	type contactsView struct {
		Contacts []struct {
			Name  string `json:"name"`
			URL   string `json:"url"`
			Token struct {
				Configured bool   `json:"configured"`
				Masked     string `json:"masked"`
			} `json:"token"`
		} `json:"contacts"`
	}
	var list contactsView
	doJSON(t, app, "GET", "/agents/myfriend/contacts", token, "", fiber.StatusOK, &list)
	if len(list.Contacts) != 0 {
		t.Fatalf("contacts = %+v", list.Contacts)
	}

	doJSON(t, app, "POST", "/agents/myfriend/contacts", token,
		`{"name":"remote","url":"https://example.com/a2a","token":"tok","description":"Remote agent"}`,
		fiber.StatusOK, nil)
	doJSON(t, app, "POST", "/agents/myfriend/contacts", token,
		`{"name":"REMOTE","url":"https://other.example/a2a"}`, fiber.StatusConflict, nil)
	doJSON(t, app, "POST", "/agents/myfriend/contacts", token,
		`{"name":"bad","url":"not-a-url"}`, fiber.StatusBadRequest, nil)

	doJSON(t, app, "GET", "/agents/myfriend/contacts", token, "", fiber.StatusOK, &list)
	if len(list.Contacts) != 1 || list.Contacts[0].Name != "remote" || list.Contacts[0].URL != "https://example.com/a2a" {
		t.Fatalf("contacts = %+v", list.Contacts)
	}
	if !list.Contacts[0].Token.Configured || list.Contacts[0].Token.Masked != "********" {
		t.Fatalf("token should be masked: %+v", list.Contacts[0].Token)
	}

	doJSON(t, app, "DELETE", "/agents/myfriend/contacts/remote", token, "", fiber.StatusOK, nil)
	doJSON(t, app, "DELETE", "/agents/myfriend/contacts/remote", token, "", fiber.StatusNotFound, nil)
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
		"conv-1", 0, "First", `[{"role":"user","content":"hello"}]`, 1, time.Now().UTC(), time.Now().UTC(),
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

func TestRouterConversationManagement(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	agentRoot := filepath.Join(home, ".myaaw", "myfriend")
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
	now := time.Now().UTC()
	for _, row := range []struct {
		id     string
		userID int
		title  string
	}{
		{"conv-mine", 0, "Mine report"},
		{"conv-other", 7, "Secret plan"},
	} {
		if _, err := db.Exec(
			`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
			row.id, row.userID, row.title, `[{"role":"user","content":"hello"}]`, 1, now, now,
		); err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}
	db.Close()

	app := mustNewApp(t)
	token := login(t, app)

	var me struct {
		Username string `json:"username"`
		UserID   int    `json:"user_id"`
	}
	doJSON(t, app, "GET", "/auth/me", token, "", fiber.StatusOK, &me)
	if me.Username != "myaaw" || me.UserID != 0 {
		t.Fatalf("me = %+v", me)
	}

	type conversationList struct {
		Conversations []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"conversations"`
		Total int `json:"total"`
	}

	// No user_id: defaults to the authenticated user (0).
	var mine conversationList
	doJSON(t, app, "GET", "/agents/myfriend/conversations", token, "", fiber.StatusOK, &mine)
	if mine.Total != 1 || len(mine.Conversations) != 1 || mine.Conversations[0].ID != "conv-mine" {
		t.Fatalf("mine = %+v", mine)
	}

	// Explicit user_id still works for administration.
	var other conversationList
	doJSON(t, app, "GET", "/agents/myfriend/conversations?user_id=7", token, "", fiber.StatusOK, &other)
	if other.Total != 1 || len(other.Conversations) != 1 || other.Conversations[0].ID != "conv-other" {
		t.Fatalf("other = %+v", other)
	}

	var search conversationList
	doJSON(t, app, "GET", "/agents/myfriend/conversations?search=report", token, "", fiber.StatusOK, &search)
	if search.Total != 1 || len(search.Conversations) != 1 || search.Conversations[0].ID != "conv-mine" {
		t.Fatalf("search = %+v", search)
	}

	var searchMiss conversationList
	doJSON(t, app, "GET", "/agents/myfriend/conversations?search=secret", token, "", fiber.StatusOK, &searchMiss)
	if searchMiss.Total != 0 || len(searchMiss.Conversations) != 0 {
		t.Fatalf("search miss = %+v", searchMiss)
	}

	var detail struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/conversations/conv-mine", token, "", fiber.StatusOK, &detail)
	if detail.Title != "Mine report" {
		t.Fatalf("detail = %+v", detail)
	}
	doJSON(t, app, "GET", "/agents/myfriend/conversations/conv-other", token, "", fiber.StatusForbidden, nil)

	var renamed struct {
		OK    bool   `json:"ok"`
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	doJSON(t, app, "PATCH", "/agents/myfriend/conversations/conv-mine", token,
		`{"title":"  Updated title  "}`, fiber.StatusOK, &renamed)
	if !renamed.OK || renamed.ID != "conv-mine" || renamed.Title != "Updated title" {
		t.Fatalf("renamed = %+v", renamed)
	}
	doJSON(t, app, "PATCH", "/agents/myfriend/conversations/conv-other", token,
		`{"title":"Stolen"}`, fiber.StatusForbidden, nil)
	doJSON(t, app, "PATCH", "/agents/myfriend/conversations/conv-mine", token,
		`{"title":"   "}`, fiber.StatusBadRequest, nil)
	doJSON(t, app, "PATCH", "/agents/myfriend/conversations/missing", token,
		`{"title":"Nope"}`, fiber.StatusNotFound, nil)

	doJSON(t, app, "GET", "/agents/myfriend/conversations/conv-mine", token, "", fiber.StatusOK, &detail)
	if detail.Title != "Updated title" {
		t.Fatalf("detail after rename = %+v", detail)
	}

	doJSON(t, app, "DELETE", "/agents/myfriend/conversations/conv-other", token, "", fiber.StatusForbidden, nil)
	doJSON(t, app, "DELETE", "/agents/myfriend/conversations/conv-mine", token, "", fiber.StatusOK, nil)
	doJSON(t, app, "GET", "/agents/myfriend/conversations/conv-mine", token, "", fiber.StatusNotFound, nil)
}

func TestRouterUsesConfiguredUserID(t *testing.T) {
	gotUser := make(chan int, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			UserID int `json:"user_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotUser <- payload.UserID
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"ok"}`))
	}))
	defer upstream.Close()

	setupTestAgent(t, upstream.URL)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("myaawadmin"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	credentialsPath := filepath.Join(home, ".myaaw", "controlplane.json")
	credentials := fmt.Sprintf(
		`{"username":"myaaw","password_hash":%q,"user_id":7,"created_at":"2026-01-01T00:00:00Z"}`,
		string(hash),
	)
	if err := os.WriteFile(credentialsPath, []byte(credentials), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	dbDir := filepath.Join(home, ".myaaw", "myfriend", "database")
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
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		"conv-config", 7, "Configured user", `[]`, 1, now, now,
	); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	db.Close()

	app := mustNewApp(t)
	token := login(t, app)

	var me struct {
		UserID int `json:"user_id"`
	}
	doJSON(t, app, "GET", "/auth/me", token, "", fiber.StatusOK, &me)
	if me.UserID != 7 {
		t.Fatalf("me.UserID = %d, want 7", me.UserID)
	}

	var list struct {
		Total         int `json:"total"`
		Conversations []struct {
			ID string `json:"id"`
		} `json:"conversations"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/conversations", token, "", fiber.StatusOK, &list)
	if list.Total != 1 || len(list.Conversations) != 1 || list.Conversations[0].ID != "conv-config" {
		t.Fatalf("conversations = %+v", list)
	}

	doJSON(t, app, "GET", "/agents/myfriend/conversations?user_id=0", token, "", fiber.StatusOK, &list)
	if list.Total != 0 || len(list.Conversations) != 0 {
		t.Fatalf("user 0 should not see user 7 conversations: %+v", list)
	}

	doJSON(t, app, "POST", "/agents/myfriend/chat", token, `{"user_id":123,"text":"hi"}`, fiber.StatusOK, nil)
	if userID := <-gotUser; userID != 7 {
		t.Fatalf("proxied user_id = %d, want 7", userID)
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
		Created bool `json:"created"`
		Config  struct {
			Bot struct {
				Type string `json:"type"`
			} `json:"bot"`
		} `json:"config"`
	}
	doJSON(t, app, "POST", "/agents/second/onboard", token, `{}`, fiber.StatusCreated, &created)
	if !created.Created {
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

func TestRouterOnboardConfigAndModels(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)
	token := login(t, app)

	type configView struct {
		DefaultProvider string `json:"default_provider"`
		Providers       []struct {
			Name         string `json:"name"`
			DefaultModel string `json:"default_model"`
		} `json:"providers"`
		Bot struct {
			Type          string   `json:"type"`
			OwnerIDs      []string `json:"owner_ids"`
			MaxIterations int      `json:"max_iterations"`
		} `json:"bot"`
		Heartbeat struct {
			Every string `json:"every"`
		} `json:"heartbeat"`
		Cron struct {
			Active bool `json:"active"`
		} `json:"cron"`
	}

	var onboard struct {
		Created bool       `json:"created"`
		Config  configView `json:"config"`
	}
	doJSON(t, app, "POST", "/agents/second/onboard", token, `{}`, fiber.StatusCreated, &onboard)
	if !onboard.Created {
		t.Fatalf("onboard = %+v", onboard)
	}
	if onboard.Config.Bot.MaxIterations == 0 || !onboard.Config.Cron.Active {
		t.Fatalf("onboard should apply the template defaults: %+v", onboard.Config)
	}

	// A provider section configures the agent without touching the world.
	doJSON(t, app, "POST", "/agents/second/onboard", token,
		`{"provider":{"type":"openai","name":"deepseek","base_url":"http://127.0.0.1:1","api_key":"sk-test","model":"deepseek-chat"}}`,
		fiber.StatusOK, &onboard)
	if onboard.Created {
		t.Fatal("second onboarding should reconfigure, not create")
	}
	if onboard.Config.DefaultProvider != "deepseek" || len(onboard.Config.Providers) != 1 {
		t.Fatalf("provider not applied: %+v", onboard.Config)
	}

	// Model discovery falls back to the default model when the provider is
	// unreachable, so the dashboard wizard can always continue.
	var models struct {
		Models []string `json:"models"`
		Source string   `json:"source"`
	}
	doJSON(t, app, "POST", "/providers/models", token,
		`{"type":"openai","base_url":"http://127.0.0.1:1","api_key":"sk-test"}`,
		fiber.StatusOK, &models)
	if models.Source != "default" || len(models.Models) == 0 {
		t.Fatalf("models = %+v", models)
	}
	doJSON(t, app, "GET", "/agents/second/providers/deepseek/models", token, "", fiber.StatusOK, &models)
	if models.Source != "default" || len(models.Models) == 0 {
		t.Fatalf("saved provider models = %+v", models)
	}
	doJSON(t, app, "GET", "/agents/second/providers/missing/models", token, "", fiber.StatusNotFound, nil)
	doJSON(t, app, "POST", "/providers/models", token, `{"type":"nope"}`, fiber.StatusBadRequest, nil)

	// PATCH only changes the fields it receives; no key is ever removed.
	doJSON(t, app, "PATCH", "/agents/second/config", token,
		`{"bot":{"max_iterations":120,"owner_ids":["7"]}}`, fiber.StatusOK, &onboard.Config)
	if onboard.Config.Bot.MaxIterations != 120 || len(onboard.Config.Bot.OwnerIDs) != 1 {
		t.Fatalf("patch not applied: %+v", onboard.Config.Bot)
	}
	if onboard.Config.Bot.Type != "private" || !onboard.Config.Cron.Active || onboard.Config.Heartbeat.Every == "" {
		t.Fatalf("patch removed untouched keys: %+v", onboard.Config)
	}

	// PATCH reset restores only the named bot fields.
	doJSON(t, app, "PATCH", "/agents/second/config/reset", token,
		`{"bot":["max_iterations"]}`, fiber.StatusOK, &onboard.Config)
	if onboard.Config.Bot.MaxIterations == 120 {
		t.Fatalf("max_iterations was not reset: %+v", onboard.Config.Bot)
	}
	if len(onboard.Config.Bot.OwnerIDs) != 1 {
		t.Fatalf("reset touched owner_ids: %+v", onboard.Config.Bot)
	}

	// A full reset restores the template, providers included.
	doJSON(t, app, "POST", "/agents/second/config/reset", token, "", fiber.StatusOK, &onboard.Config)
	if onboard.Config.DefaultProvider != "" || len(onboard.Config.Providers) != 0 {
		t.Fatalf("full reset kept providers: %+v", onboard.Config)
	}

	doJSON(t, app, "DELETE", "/agents/second", token, "", fiber.StatusOK, nil)
}

func TestRouterAcceptsRawAuthorizationToken(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)
	token := login(t, app)

	req := httptest.NewRequest("GET", "/agents", nil)
	req.Header.Set("Authorization", token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
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

func TestRouterObservabilityEndpoints(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)
	token := login(t, app)

	// Nothing recorded yet: an empty window, not an error.
	var summary struct {
		Runs    int     `json:"runs"`
		CostUSD float64 `json:"cost_usd"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/observability/summary?days=7", token, "", fiber.StatusOK, &summary)
	if summary.Runs != 0 {
		t.Fatalf("runs = %d, want 0", summary.Runs)
	}

	path, err := agentdir.ObservabilityPath("myfriend")
	if err != nil {
		t.Fatalf("observability path: %v", err)
	}
	store, err := obs.Open(path, obs.Options{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	startedAt := time.Now().Add(-time.Hour).UnixMilli()
	run := obs.Run{
		ID: "run_test", StartedAt: startedAt, EndedAt: startedAt + 1200,
		Channel: "telegram", UserID: "42", ConversationID: "conv-1",
		Provider: "deepseek", Model: "deepseek-flash", Agent: "myfriend",
		Status: obs.StatusCompleted, InputTokens: 1000, OutputTokens: 100,
		CacheHitTokens: 800, ModelCostUSD: 0.001, TotalMs: 1200, FirstTokenMs: 400,
	}
	if err := store.UpsertRun(run); err != nil {
		t.Fatalf("upsert run: %v", err)
	}
	spans := []obs.Span{
		{ID: "sp_turn", RunID: run.ID, ParentID: run.ID, Kind: obs.KindStep, Name: "Step 1", StepIndex: 1, StartedAt: startedAt, EndedAt: startedAt + 1100, DurationMs: 1100, Status: obs.StatusCompleted},
		{ID: "sp_llm", RunID: run.ID, ParentID: "sp_turn", Kind: obs.KindLLM, Name: "deepseek-flash", StepIndex: 1, RequestIndex: 1, StartedAt: startedAt, EndedAt: startedAt + 1000, DurationMs: 1000, FirstTokenMs: 400, GenerationMs: 600, InputTokens: 1000, OutputTokens: 100, CacheHitTokens: 800, CostUSD: 0.001, Status: obs.StatusCompleted},
	}
	if err := store.UpsertSpans(spans); err != nil {
		t.Fatalf("upsert spans: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	doJSON(t, app, "GET", "/agents/myfriend/observability/summary?days=7", token, "", fiber.StatusOK, &summary)
	if summary.Runs != 1 {
		t.Fatalf("runs = %d, want 1", summary.Runs)
	}
	if summary.CostUSD != 0.001 {
		t.Fatalf("cost = %v, want 0.001", summary.CostUSD)
	}

	var list struct {
		Runs  []obs.Run `json:"runs"`
		Total int       `json:"total"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/observability/runs?limit=10", token, "", fiber.StatusOK, &list)
	if list.Total != 1 || len(list.Runs) != 1 || list.Runs[0].ID != run.ID {
		t.Fatalf("list = %+v", list)
	}

	var detail struct {
		Run   obs.Run    `json:"run"`
		Spans []obs.Span `json:"spans"`
		Tree  *struct {
			ID       string `json:"id"`
			Children []struct {
				ID string `json:"id"`
			} `json:"children"`
		} `json:"tree"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/observability/runs/run_test", token, "", fiber.StatusOK, &detail)
	if detail.Run.ID != run.ID || len(detail.Spans) != 2 {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.Tree == nil || len(detail.Tree.Children) != 1 || detail.Tree.Children[0].ID != "sp_turn" {
		t.Fatalf("tree = %+v", detail.Tree)
	}

	doJSON(t, app, "GET", "/agents/myfriend/observability/runs/missing", token, "", fiber.StatusNotFound, nil)

	var deleted struct {
		Removed int64 `json:"removed"`
	}
	doJSON(t, app, "DELETE", fmt.Sprintf("/agents/myfriend/observability/runs?before=%d", time.Now().UnixMilli()), token, "", fiber.StatusOK, &deleted)
	if deleted.Removed != 1 {
		t.Fatalf("removed = %d, want 1", deleted.Removed)
	}
}

func TestRouterObservabilityRejectsInvalidParams(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)
	token := login(t, app)

	doJSON(t, app, "GET", "/agents/myfriend/observability/summary?days=zero", token, "", fiber.StatusBadRequest, nil)
	doJSON(t, app, "GET", "/agents/myfriend/observability/runs?limit=many", token, "", fiber.StatusBadRequest, nil)
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
		_, _ = w.Write([]byte(`{"text":"ok","run_id":"run_upstream","timeline":[{"kind":"text","text":"ok"}]}`))
	})
	mux.HandleFunc("/api/chat/stream", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = fmt.Fprint(w, "event: thought\ndata: {\"text\":\"hmm\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = fmt.Fprint(w, "event: done\ndata: {\"ok\":true,\"text\":\"ok\",\"run_id\":\"run_upstream\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestRouterObservabilitySessionDetail(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)
	token := login(t, app)

	path, err := agentdir.ObservabilityPath("myfriend")
	if err != nil {
		t.Fatalf("observability path: %v", err)
	}
	store, err := obs.Open(path, obs.Options{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	base := time.Now().Add(-time.Hour).UnixMilli()
	// Insert newest first: the endpoint must still answer oldest first.
	turns := []struct {
		id        string
		startedAt int64
		steps     int
	}{
		{"run_new", base + 1000, 3},
		{"run_old", base, 1},
	}
	for _, turn := range turns {
		run := obs.Run{
			ID: turn.id, StartedAt: turn.startedAt, ConversationID: "conv-session",
			Channel: "cli", Model: "deepseek-flash", Status: obs.StatusCompleted,
			TotalMs: 1200, InputTokens: 1000, OutputTokens: 100, ModelCostUSD: 0.001,
		}
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
		for step := 1; step <= turn.steps; step++ {
			if err := store.UpsertSpans([]obs.Span{{
				ID: fmt.Sprintf("%s_step_%d", turn.id, step), RunID: turn.id,
				Kind: obs.KindStep, StepIndex: step,
				StartedAt: turn.startedAt, Status: obs.StatusCompleted,
			}}); err != nil {
				t.Fatalf("upsert step: %v", err)
			}
		}
		// The trajectory UI needs the prompt and the tool payloads.
		if err := store.UpsertSpans([]obs.Span{
			{
				ID: turn.id + "_user", RunID: turn.id, Kind: obs.KindUser, Name: "user",
				StartedAt: turn.startedAt, Status: obs.StatusCompleted,
				InputJSON: `{"messages":[{"role":"user","content":"hi"}]}`,
			},
			{
				ID: turn.id + "_llm", RunID: turn.id, Kind: obs.KindLLM, Name: "deepseek-flash",
				StepIndex: 1, RequestIndex: 1, StartedAt: turn.startedAt, Status: obs.StatusCompleted,
				InputJSON:  `{"messages":[{"role":"user","content":"hi"}]}`,
				OutputJSON: `{"role":"assistant","content":"hello"}`,
			},
		}); err != nil {
			t.Fatalf("upsert content spans: %v", err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	var detail struct {
		ConversationID string  `json:"conversation_id"`
		Channel        string  `json:"channel"`
		Turns          int     `json:"turns"`
		StartedAt      int64   `json:"started_at"`
		LastActiveAt   int64   `json:"last_active_at"`
		CostUSD        float64 `json:"cost_usd"`
		Runs           []struct {
			ID    string     `json:"id"`
			Steps int        `json:"steps"`
			Spans []obs.Span `json:"spans"`
		} `json:"runs"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/observability/sessions/conv-session", token, "", fiber.StatusOK, &detail)

	if detail.Turns != 2 || len(detail.Runs) != 2 {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.Runs[0].ID != "run_old" || detail.Runs[1].ID != "run_new" {
		t.Fatalf("turns are not oldest first: %+v", detail.Runs)
	}
	if detail.Runs[0].Steps != 1 || detail.Runs[1].Steps != 3 {
		t.Fatalf("step counts = %+v", detail.Runs)
	}

	// Each turn ships its spans, including the captured content.
	if len(detail.Runs[0].Spans) == 0 {
		t.Fatal("session detail must include spans by default")
	}
	var sawLLM bool
	for _, span := range detail.Runs[0].Spans {
		if span.Kind == obs.KindLLM && span.InputJSON != "" && span.OutputJSON != "" {
			sawLLM = true
		}
	}
	if !sawLLM {
		t.Fatalf("llm span content is missing: %+v", detail.Runs[0].Spans)
	}

	var metadataOnly struct {
		Runs []struct {
			ID    string     `json:"id"`
			Spans []obs.Span `json:"spans"`
		} `json:"runs"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/observability/sessions/conv-session?spans=false", token, "", fiber.StatusOK, &metadataOnly)
	if len(metadataOnly.Runs) != 2 {
		t.Fatalf("metadata runs = %d", len(metadataOnly.Runs))
	}
	for _, run := range metadataOnly.Runs {
		if len(run.Spans) != 0 {
			t.Fatalf("spans=false returned %d spans for %s", len(run.Spans), run.ID)
		}
	}
	if detail.StartedAt != base || detail.LastActiveAt != base+1000 {
		t.Fatalf("window = %d..%d", detail.StartedAt, detail.LastActiveAt)
	}
	if detail.CostUSD != 0.002 {
		t.Fatalf("cost = %v", detail.CostUSD)
	}

	doJSON(t, app, "GET", "/agents/myfriend/observability/sessions/missing", token, "", fiber.StatusNotFound, nil)
}

func TestRouterApprovalsManagement(t *testing.T) {
	upstream := newUpstream(t)
	setupTestAgent(t, upstream.URL)
	app := mustNewApp(t)
	token := login(t, app)

	var patched struct {
		Approvals struct {
			Mode  string   `json:"mode"`
			Allow []string `json:"allow"`
		} `json:"approvals"`
	}
	doJSON(t, app, "PATCH", "/agents/myfriend/config", token,
		`{"approvals":{"mode":"manual","allow":["filesystem:read_file"]}}`,
		fiber.StatusOK, &patched)
	if patched.Approvals.Mode != "manual" || len(patched.Approvals.Allow) != 1 || patched.Approvals.Allow[0] != "filesystem:read_file" {
		t.Fatalf("patched approvals = %+v", patched.Approvals)
	}

	path, err := agentdir.ApprovalsPath("myfriend")
	if err != nil {
		t.Fatal(err)
	}
	store := approvals.NewStore(path)
	if err := store.GrantAlways("bash", approvals.Scope{Channel: "telegram", UserID: "111"}); err != nil {
		t.Fatal(err)
	}

	var view struct {
		Config struct {
			Mode string `json:"mode"`
		} `json:"config"`
		Grants []struct {
			Key    string `json:"key"`
			Always []struct {
				Channel string `json:"channel"`
				UserID  string `json:"user_id"`
			} `json:"always"`
		} `json:"grants"`
	}
	doJSON(t, app, "GET", "/agents/myfriend/approvals", token, "", fiber.StatusOK, &view)
	if view.Config.Mode != "manual" || len(view.Grants) != 1 || view.Grants[0].Key != "bash" || len(view.Grants[0].Always) != 1 || view.Grants[0].Always[0].Channel != "telegram" {
		t.Fatalf("approvals view = %+v", view)
	}

	doJSON(t, app, "DELETE", "/agents/myfriend/approvals/grants/bash", token, "", fiber.StatusOK, nil)
	if grants := approvals.NewStore(path).Snapshot(); len(grants) != 0 {
		t.Fatalf("grants after revoke = %v", grants)
	}
	doJSON(t, app, "DELETE", "/agents/myfriend/approvals/grants/bash", token, "", fiber.StatusNotFound, nil)
}

func TestRouterApprovalDecisionForwardsUserID(t *testing.T) {
	headerCh := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/approvals/abc", func(w http.ResponseWriter, r *http.Request) {
		headerCh <- r.Header.Get(approvals.UserIDHeader)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"decision":"allow_once"}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	setupTestAgent(t, server.URL)
	app := mustNewApp(t)
	token := login(t, app)

	var me struct {
		UserID int `json:"user_id"`
	}
	doJSON(t, app, "GET", "/auth/me", token, "", fiber.StatusOK, &me)
	doJSON(t, app, "POST", "/agents/myfriend/approvals/abc/decision", token, `{"decision":"allow_once"}`, fiber.StatusOK, nil)

	select {
	case got := <-headerCh:
		if want := strconv.Itoa(me.UserID); got != want {
			t.Fatalf("%s = %q; want %q", approvals.UserIDHeader, got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received the decision")
	}
}

func TestRouterRevokeApprovalGrantScope(t *testing.T) {
	upstream := newUpstream(t)
	setupTestAgent(t, upstream.URL)
	app := mustNewApp(t)
	token := login(t, app)

	path, err := agentdir.ApprovalsPath("myfriend")
	if err != nil {
		t.Fatal(err)
	}
	store := approvals.NewStore(path)
	if err := store.GrantAlways("bash", approvals.Scope{Channel: "telegram", UserID: "111"}); err != nil {
		t.Fatal(err)
	}
	if err := store.GrantAlways("bash", approvals.Scope{Channel: "discord", UserID: "222"}); err != nil {
		t.Fatal(err)
	}

	doJSON(t, app, "DELETE", "/agents/myfriend/approvals/grants/bash?channel=telegram&user=111", token, "", fiber.StatusOK, nil)
	grants := approvals.NewStore(path).Snapshot()
	if len(grants["bash"].Always) != 1 || grants["bash"].Always[0].Channel != "discord" {
		t.Fatalf("grants after scoped revoke = %+v", grants)
	}

	doJSON(t, app, "DELETE", "/agents/myfriend/approvals/grants/bash?channel=telegram&user=999", token, "", fiber.StatusNotFound, nil)
}
