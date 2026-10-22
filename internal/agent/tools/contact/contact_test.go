package contact

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	var gotAuth, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotMethod = req.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"t1","status":{"state":"completed"},"artifacts":[{"name":"response","parts":[{"kind":"text","text":"remote answer"}]}]}}`))
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
	if gotMethod != "message/send" {
		t.Fatalf("method = %q", gotMethod)
	}
}

func TestContactToolFallsBackToTasksSend(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			Params struct {
				Message struct {
					Kind      string `json:"kind"`
					MessageID string `json:"messageId"`
					TaskID    string `json:"taskId"`
				} `json:"message"`
				ID string `json:"id"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		methods = append(methods, req.Method)
		if req.Method == "message/send" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`))
			return
		}
		if req.Params.Message.Kind != "message" || req.Params.Message.MessageID == "" {
			t.Errorf("message = %+v", req.Params.Message)
		}
		if req.Params.Message.TaskID != "task-1" || req.Params.ID != "task-1" {
			t.Errorf("task ids = %q / %q", req.Params.Message.TaskID, req.Params.ID)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"task-1","status":{"state":"completed"},"artifacts":[{"name":"response","parts":[{"kind":"text","text":"legacy answer"}]}]}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"action":"send","contact":"remote","taskId":"task-1","message":"hi"}`, nil)
	if out != "legacy answer" {
		t.Fatalf("output = %q", out)
	}
	if len(methods) != 2 || methods[0] != "message/send" || methods[1] != "tasks/send" {
		t.Fatalf("methods = %v", methods)
	}
}

func TestContactToolReadsHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"t1","status":{"state":"completed"},"history":[{"role":"agent","parts":[{"kind":"text","text":"answer from history"}]}]}}`))
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
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"t1","status":{"state":"failed","message":{"role":"agent","parts":[{"kind":"text","text":"boom"}]}}}}`))
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
		if r.URL.Path != "/.well-known/agent.json" {
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

func TestContactToolSignsRequests(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPath := filepath.Join(t.TempDir(), "a2a_key.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	previous := config.A2ASigningKeyPath
	config.A2ASigningKeyPath = keyPath
	t.Cleanup(func() { config.A2ASigningKeyPath = previous })

	verified := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, err := a2a.VerifyBody(r.Header.Get(a2a.SignatureHeader), body, map[string]ed25519.PublicKey{a2a.KeyID(pub): pub})
		verified = err == nil
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"t1","status":{"state":"completed"},"artifacts":[{"name":"response","parts":[{"kind":"text","text":"ok"}]}]}}`))
	}))
	defer server.Close()

	setupContactAgent(t, `[{"name":"remote","url":"`+server.URL+`"}]`)

	out := NewContactTool().CallTool(`{"action":"send","contact":"remote","message":"hi"}`, nil)
	if out != "ok" {
		t.Fatalf("output = %q", out)
	}
	if !verified {
		t.Fatal("request signature was not valid")
	}
}

func TestContactToolInputRequiredResume(t *testing.T) {
	var gotTaskID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				ID string `json:"id"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotTaskID = req.Params.ID

		w.Header().Set("Content-Type", "application/json")
		if req.Params.ID == "" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"task-9","status":{"state":"input-required","message":{"role":"agent","parts":[{"kind":"text","text":"Tanggal berapa?"}]}},"messages":[]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"id":"task-9","status":{"state":"completed"},"artifacts":[{"name":"response","parts":[{"kind":"text","text":"laporan selesai"}]}]}}`))
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
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32002,"message":"task already exists"}}`))
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
	card.PublicKey = a2a.EncodePublicKey(pub)
	card.KeyID = a2a.KeyID(pub)
	signed, err := a2a.SignCard(card, priv)
	if err != nil {
		t.Fatalf("SignCard: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent.json" {
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
