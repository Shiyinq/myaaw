package contact

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/config"
	"myaaw/internal/services/a2a"
)

func setupContactAgent(t *testing.T, entries string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive("myfriend"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	dir := filepath.Join(home, ".myaaw", "myfriend")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir agent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "contacts.json"), []byte(entries), 0o600); err != nil {
		t.Fatalf("write contacts: %v", err)
	}
}

func TestContactToolDelegatesTask(t *testing.T) {
	var gotAuth, gotMethod, gotVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("A2A-Version")
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotMethod = req.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"task":{"id":"t1","status":{"state":"TASK_STATE_COMPLETED"},"artifacts":[{"name":"response","parts":[{"text":"remote answer"}]}]}}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`","token":"tok"}]`)

	out := NewContactTool().CallTool(`{"contact":"remote","message":"hi"}`, nil)
	if out != "remote answer" {
		t.Fatalf("output = %q", out)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotMethod != "SendMessage" {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotVersion != "1.0" {
		t.Fatalf("A2A-Version = %q", gotVersion)
	}
}

func TestContactToolSendsTaskIDOnResume(t *testing.T) {
	var gotMethod, gotTaskID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			Params struct {
				Message struct {
					TaskID string `json:"taskId"`
				} `json:"message"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotMethod = req.Method
		gotTaskID = req.Params.Message.TaskID
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"task":{"id":"task-1","status":{"state":"TASK_STATE_COMPLETED"},"artifacts":[{"name":"response","parts":[{"text":"v1 answer"}]}]}}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"action":"send","contact":"remote","taskId":"task-1","message":"hi"}`, nil)
	if out != "v1 answer" {
		t.Fatalf("output = %q", out)
	}
	if gotMethod != "SendMessage" {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotTaskID != "task-1" {
		t.Fatalf("taskId = %q", gotTaskID)
	}
}

func TestContactToolReadsHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"task":{"id":"t1","status":{"state":"TASK_STATE_COMPLETED"},"history":[{"role":"ROLE_AGENT","parts":[{"text":"answer from history"}]}]}}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"action":"send","contact":"remote","message":"hi"}`, nil)
	if out != "answer from history" {
		t.Fatalf("output = %q", out)
	}
}

func TestContactToolErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"task":{"id":"t1","status":{"state":"TASK_STATE_FAILED","message":{"role":"agent","parts":[{"text":"boom"}]}}}}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)
	tool := NewContactTool()

	out := tool.CallTool(`{"contact":"remote","message":"hi"}`, nil)
	if !strings.Contains(out, "failed") || !strings.Contains(out, "boom") {
		t.Fatalf("output = %q", out)
	}

	out = tool.CallTool(`{"contact":"missing","message":"hi"}`, nil)
	if !strings.Contains(out, "not found") {
		t.Fatalf("output = %q", out)
	}
}

func TestContactToolListAndGet(t *testing.T) {
	card := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent-card.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"remote-agent","description":"Remote helper","skills":[{"id":"weather","name":"Weather"}]}`))
	}))
	defer card.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+card.URL+`/a2a","token":"supersecretvalue","description":"Remote agent"}]`)
	tool := NewContactTool()

	out := tool.CallTool(`{"action":"list"}`, nil)
	for _, want := range []string{"Contacts (1)", "- remote: Remote agent", card.URL + "/a2a", "[token configured]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "supersecretvalue") {
		t.Fatalf("list output leaked the token: %q", out)
	}

	out = tool.CallTool(`{"action":"get_contact","contact":"remote"}`, nil)
	for _, want := range []string{"Contact: remote", "Token: configured", "Agent card: remote-agent", "weather (Weather)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("get output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "supersecretvalue") {
		t.Fatalf("get output leaked the token: %q", out)
	}

	out = tool.CallTool(`{"action":"get","contact":"missing"}`, nil)
	if !strings.Contains(out, "not found") || !strings.Contains(out, "remote") {
		t.Fatalf("output = %q", out)
	}
}

func TestContactToolAdd(t *testing.T) {
	setupContactAgent(t, `[]`)
	tool := NewContactTool()

	out := tool.CallTool(`{"action":"add_contact","contact":"newagent","url":"http://localhost:9099/a2a","token":"supersecretvalue","description":"New agent"}`, nil)
	if !strings.Contains(out, `Added contact "newagent"`) || !strings.Contains(out, "Token: configured") {
		t.Fatalf("output = %q", out)
	}
	if strings.Contains(out, "supersecretvalue") {
		t.Fatalf("add output leaked the token: %q", out)
	}

	path, err := agentdir.ActiveContactsPath()
	if err != nil {
		t.Fatalf("ActiveContactsPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contacts: %v", err)
	}
	var entries []contactEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("parse contacts: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "newagent" || entries[0].Token != "supersecretvalue" {
		t.Fatalf("entries = %+v", entries)
	}

	out = tool.CallTool(`{"action":"add","contact":"NewAgent","url":"http://localhost:9098/a2a"}`, nil)
	if !strings.Contains(out, "already exists") {
		t.Fatalf("duplicate output = %q", out)
	}

	out = tool.CallTool(`{"action":"add","contact":"bad","url":"ftp://example.com/a2a"}`, nil)
	if !strings.Contains(out, "http(s)") {
		t.Fatalf("invalid url output = %q", out)
	}

	out = tool.CallTool(`{"action":"add","contact":"../etc","url":"http://localhost:9097/a2a"}`, nil)
	if !strings.Contains(out, "invalid contact name") {
		t.Fatalf("invalid name output = %q", out)
	}

	out = tool.CallTool(`{"action":"list"}`, nil)
	if !strings.Contains(out, "newagent") {
		t.Fatalf("list output = %q", out)
	}
}

func TestContactToolDownloadsURLAritfact(t *testing.T) {
	content := []byte("video-bytes-here")
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(content)
	}))
	defer fileServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"task":{"id":"t1","status":{"state":"TASK_STATE_COMPLETED"},"artifacts":[{"name":"video.mp4","parts":[{"url":"` + fileServer.URL + `/video.mp4","filename":"video.mp4","mediaType":"video/mp4"}]}]}}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"action":"send","contact":"remote","message":"hi"}`, nil)
	if !strings.Contains(out, "Files:") || !strings.Contains(out, "video.mp4") || !strings.Contains(out, "saved at") {
		t.Fatalf("output = %q", out)
	}

	home := os.Getenv("HOME")
	saved := filepath.Join(home, ".myaaw", "myfriend", "home", "workspace", "downloads", "t1", "video.mp4")
	data, err := os.ReadFile(saved)
	if err != nil {
		t.Fatalf("read saved artifact: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("saved content = %q", data)
	}
}

func TestContactToolSavesRawArtifact(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("hello file"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"task":{"id":"t2","status":{"state":"TASK_STATE_COMPLETED"},"artifacts":[{"name":"notes.txt","parts":[{"raw":"` + encoded + `","filename":"notes.txt","mediaType":"text/plain"}]}]}}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"action":"send","contact":"remote","message":"hi"}`, nil)
	if !strings.Contains(out, "Files:") || !strings.Contains(out, "notes.txt") {
		t.Fatalf("output = %q", out)
	}

	home := os.Getenv("HOME")
	saved := filepath.Join(home, ".myaaw", "myfriend", "home", "workspace", "downloads", "t2", "notes.txt")
	data, err := os.ReadFile(saved)
	if err != nil {
		t.Fatalf("read saved artifact: %v", err)
	}
	if string(data) != "hello file" {
		t.Fatalf("saved content = %q", data)
	}
}

func TestContactToolAsyncSend(t *testing.T) {
	var returnImmediately bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			Params struct {
				Configuration struct {
					ReturnImmediately bool `json:"returnImmediately"`
				} `json:"configuration"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		returnImmediately = req.Params.Configuration.ReturnImmediately
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"task":{"id":"async-1","status":{"state":"TASK_STATE_SUBMITTED"}}}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"action":"send","contact":"remote","message":"long job","async":true}`, nil)
	if !strings.Contains(out, "[async taskId=async-1]") {
		t.Fatalf("output = %q", out)
	}
	if !returnImmediately {
		t.Fatal("SendMessage must set configuration.returnImmediately=true")
	}
}

func TestContactToolStatusCompleted(t *testing.T) {
	var method, taskID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			Params struct {
				ID string `json:"id"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		method = req.Method
		taskID = req.Params.ID
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"async-1","status":{"state":"TASK_STATE_COMPLETED"},"artifacts":[{"name":"response","parts":[{"text":"done"}]}]}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"action":"status","contact":"remote","taskId":"async-1"}`, nil)
	if out != "done" {
		t.Fatalf("output = %q", out)
	}
	if method != "GetTask" || taskID != "async-1" {
		t.Fatalf("method = %q, taskID = %q", method, taskID)
	}
}

func TestContactToolStatusRunning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"async-1","status":{"state":"TASK_STATE_WORKING"}}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"action":"status","contact":"remote","taskId":"async-1"}`, nil)
	if !strings.Contains(out, "[async taskId=async-1]") || !strings.Contains(out, "still running") {
		t.Fatalf("output = %q", out)
	}
}

func TestContactToolEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"action":"send","contact":"remote","message":"hi"}`, nil)
	if !strings.Contains(out, "empty response") {
		t.Fatalf("output = %q", out)
	}
}

func TestContactToolUnknownAction(t *testing.T) {
	setupContactAgent(t, `[]`)
	tool := NewContactTool()

	out := tool.CallTool(`{"action":"remove","contact":"x"}`, nil)
	if !strings.Contains(out, "unknown contact action") {
		t.Fatalf("output = %q", out)
	}

	out = tool.CallTool(`{"contact":"x"}`, nil)
	if !strings.Contains(out, "action is required") {
		t.Fatalf("output = %q", out)
	}
}

func TestContactToolInputRequiredResume(t *testing.T) {
	var gotTaskID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				Message struct {
					TaskID string `json:"taskId"`
				} `json:"message"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotTaskID = req.Params.Message.TaskID

		w.Header().Set("Content-Type", "application/json")
		if gotTaskID == "" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"task":{"id":"task-9","status":{"state":"TASK_STATE_INPUT_REQUIRED","message":{"role":"agent","parts":[{"text":"Tanggal berapa?"}]}},"history":[]}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"task":{"id":"task-9","status":{"state":"TASK_STATE_COMPLETED"},"artifacts":[{"name":"response","parts":[{"text":"laporan selesai"}]}]}}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)
	tool := NewContactTool()

	out := tool.CallTool(`{"action":"send","contact":"remote","message":"buat laporan"}`, nil)
	if gotTaskID != "" {
		t.Fatalf("first send taskId = %q, want empty", gotTaskID)
	}
	if !strings.Contains(out, "[input-required taskId=task-9]") || !strings.Contains(out, "Tanggal berapa?") {
		t.Fatalf("output = %q", out)
	}

	out = tool.CallTool(`{"action":"send","contact":"remote","taskId":"task-9","message":"1 Oktober"}`, nil)
	if gotTaskID != "task-9" {
		t.Fatalf("resume taskId = %q, want task-9", gotTaskID)
	}
	if out != "laporan selesai" {
		t.Fatalf("output = %q", out)
	}
}

func TestContactToolRejectsStaleTaskID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32004,"message":"task is not resumable"}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"action":"send","contact":"remote","taskId":"task-1","message":"hi"}`, nil)
	if !strings.Contains(out, "not waiting for input") {
		t.Fatalf("output = %q", out)
	}
}

func TestContactToolReportsCardSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	card := a2a.BuildCard("remote", "http://localhost:9999", nil, false)
	signed, err := a2a.SignCard(card, priv)
	if err != nil {
		t.Fatalf("SignCard: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent-card.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(signed)
	}))
	defer server.Close()

	previous := config.A2ATrustedKeys
	config.A2ATrustedKeys = "remote:" + a2a.EncodePublicKey(pub)
	t.Cleanup(func() { config.A2ATrustedKeys = previous })

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`/a2a"}]`)

	out := NewContactTool().CallTool(`{"action":"get","contact":"remote"}`, nil)
	if !strings.Contains(out, "Card signature: verified") {
		t.Fatalf("output = %q", out)
	}
}

func TestContactToolRetriesWithSameMessageID(t *testing.T) {
	var mu sync.Mutex
	var messageIDs []string
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				Message struct {
					MessageID string `json:"messageId"`
				} `json:"message"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		calls++
		call := calls
		messageIDs = append(messageIDs, req.Params.Message.MessageID)
		mu.Unlock()
		if call == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("temporary failure"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"task":{"id":"t1","status":{"state":"TASK_STATE_COMPLETED"},"artifacts":[{"name":"response","parts":[{"text":"second try"}]}]}}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"contact":"remote","message":"hi"}`, nil)
	if out != "second try" {
		t.Fatalf("output = %q", out)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(messageIDs) != 2 {
		t.Fatalf("requests = %d, want 2", len(messageIDs))
	}
	if messageIDs[0] == "" || messageIDs[0] != messageIDs[1] {
		t.Fatalf("messageIds = %q, want the same non-empty id", messageIDs)
	}
}

func TestContactToolRecoversTaskAfterTimeout(t *testing.T) {
	previousTimeout := config.A2AContactTimeoutSeconds
	config.A2AContactTimeoutSeconds = 1
	t.Cleanup(func() { config.A2AContactTimeoutSeconds = previousTimeout })

	var mu sync.Mutex
	var messageIDs []string
	var immediate []bool
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				Message struct {
					MessageID string `json:"messageId"`
				} `json:"message"`
				Configuration struct {
					ReturnImmediately bool `json:"returnImmediately"`
				} `json:"configuration"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		calls++
		call := calls
		messageIDs = append(messageIDs, req.Params.Message.MessageID)
		immediate = append(immediate, req.Params.Configuration.ReturnImmediately)
		mu.Unlock()
		if call == 1 {
			// Outlive the caller's 1s budget so the client times out.
			time.Sleep(1500 * time.Millisecond)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"task":{"id":"t-run","status":{"state":"TASK_STATE_WORKING"}}}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"contact":"remote","message":"hi"}`, nil)
	if !strings.Contains(out, "[async taskId=t-run]") {
		t.Fatalf("output = %q, want async task recovery", out)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(messageIDs) != 2 {
		t.Fatalf("requests = %d, want 2", len(messageIDs))
	}
	if messageIDs[0] == "" || messageIDs[0] != messageIDs[1] {
		t.Fatalf("messageIds = %q, want the same non-empty id", messageIDs)
	}
	if immediate[0] || !immediate[1] {
		t.Fatalf("returnImmediately = %v, want [false true]", immediate)
	}
}
