package handler

import (
	"encoding/json"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"myaaw/internal/onboard"
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

func TestLocalChatGPTRequest(t *testing.T) {
	for _, tc := range []struct {
		peer, host, origin string
		want               bool
	}{
		{"127.0.0.1", "localhost:8080", "http://localhost:5173", true},
		{"::1", "[::1]:8080", "http://[::1]:5173", true},
		{"127.0.0.1", "127.0.0.1:8080", "", true},
		{"192.0.2.1", "localhost:8080", "http://localhost:5173", false},
		{"127.0.0.1", "remote.example", "http://localhost:5173", false},
		{"127.0.0.1", "localhost:8080", "https://remote.example", false},
		{"127.0.0.1", "localhost.evil.example", "http://localhost", false},
		{"127.0.0.1", "localhost:8080", "null", false},
		{"127.0.0.1", "localhost:8080", "file://localhost", false},
		{"127.0.0.1", "localhost:8080", "http://user@localhost", false},
		{"127.0.0.1", "localhost:8080", "http://localhost/path", false},
		{"127.0.0.1", "localhost:8080", "http://localhost?secret=1", false},
		{"", "localhost:8080", "http://localhost", false},
	} {
		if got := localChatGPTRequest(tc.peer, tc.host, tc.origin); got != tc.want {
			t.Errorf("local(%q, %q, %q) = %v", tc.peer, tc.host, tc.origin, got)
		}
	}
}

func TestChatGPTControllersLocalScopesAndNoStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, name := range []string{"local", "other"} {
		if _, err := onboard.Initialize(name, false); err != nil {
			t.Fatal(err)
		}
	}
	h := &Handler{config: service.NewConfigService()}
	app := fiber.New(fiber.Config{ProxyHeader: "X-Forwarded-For"})
	peer := "127.0.0.1"
	app.Use(func(c fiber.Ctx) error {
		c.RequestCtx().SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP(peer), Port: 1234})
		return c.Next()
	})
	app.Post("/agents/:name/providers/chatgpt/connect", h.ConnectChatGPT)
	app.Get("/agents/:name/providers/chatgpt/connect/:session", h.ChatGPTStatus)
	app.Delete("/agents/:name/providers/chatgpt/connect/:session", h.CancelChatGPT)
	request := func(method, path, origin string, extra ...map[string]string) (int, []byte) {
		t.Helper()
		req := httptest.NewRequest(method, "http://localhost:8080"+path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Forwarded-For", "127.0.0.1")
		req.Header.Set("X-Forwarded-Host", "localhost:8080")
		for _, headers := range extra {
			for name, value := range headers {
				req.Header.Set(name, value)
			}
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("login response must not be cached")
		}
		return resp.StatusCode, body
	}
	peer = "192.0.2.10"
	if code, body := request("POST", "/agents/local/providers/chatgpt/connect", "http://localhost:5173"); code != 403 || !strings.Contains(string(body), "same machine") {
		t.Fatalf("spoofed forwarding accepted: %d %s", code, body)
	}
	peer = "127.0.0.1"
	if code, _ := request("POST", "/agents/local/providers/chatgpt/connect", "https://evil.example"); code != 403 {
		t.Fatal("remote origin accepted", code)
	}
	code, body := request("POST", "/agents/local/providers/chatgpt/connect", "http://localhost:5173")
	if code != 200 {
		t.Fatalf("connect = %d %s", code, body)
	}
	var view service.ChatGPTConnectView
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = h.config.ChatGPTStatus("local", view.SessionID, true) })
	if view.SessionID == "" || view.AuthorizationURL == "" {
		t.Fatal("missing connect fields")
	}
	for _, method := range []string{"GET", "DELETE"} {
		if code, _ := request(method, "/agents/other/providers/chatgpt/connect/"+view.SessionID, ""); code != 404 {
			t.Fatal("cross-agent session visible", code)
		}
	}
	// hooks.server.ts forwards browser headers while rewriting Host to the
	// local control plane. GET fetches generally have Referer but no Origin.
	statusPath := "/agents/local/providers/chatgpt/connect/" + view.SessionID
	for _, headers := range []map[string]string{
		{"Referer": "https://remote.example/app/providers", "Sec-Fetch-Site": "same-origin"},
		{"Sec-Fetch-Mode": "cors", "Sec-Fetch-Site": "same-origin"},
		{"Referer": "http://localhost.evil.example/app/providers"},
	} {
		if code, _ := request("GET", statusPath, "", headers); code != 403 {
			t.Fatal("proxy made a remote/unknown browser local", code)
		}
	}
	if code, _ := request("GET", statusPath, "", map[string]string{"Referer": "http://localhost:5173/app/providers", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Site": "same-origin"}); code != 200 {
		t.Fatal("local frontend proxy polling rejected", code)
	}
	if code, _ := request("DELETE", statusPath, "https://remote.example", map[string]string{"Referer": "https://remote.example/app/providers"}); code != 403 {
		t.Fatal("remote proxy cancellation accepted", code)
	}
	code, body = request("GET", statusPath, "")
	if code != 200 || !strings.Contains(string(body), `"status":"pending"`) {
		t.Fatalf("status = %d %s", code, body)
	}
	for _, forbidden := range []string{"token", "authorization_url", "code_challenge", "nonce", "verifier"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("secret in status: %s", body)
		}
	}
	if code, _ := request("DELETE", "/agents/local/providers/chatgpt/connect/"+view.SessionID, ""); code != 200 {
		t.Fatal("cancel failed", code)
	}
	code, body = request("GET", "/agents/local/providers/chatgpt/connect/"+view.SessionID, "")
	if code != 200 || !strings.Contains(string(body), `"status":"failed"`) {
		t.Fatalf("cancel status = %d %s", code, body)
	}
}
