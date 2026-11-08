package controlplane

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatGPTEndpointsRequireAuthentication(t *testing.T) {
	setupTestAgent(t, "http://127.0.0.1:1")
	app := mustNewApp(t)
	for _, tc := range []struct{ method, path string }{
		{"POST", "/agents/myfriend/providers/chatgpt/connect"},
		{"GET", "/agents/myfriend/providers/chatgpt/connect/unknown"},
		{"DELETE", "/agents/myfriend/providers/chatgpt/connect/unknown"},
		{"POST", "/agents/myfriend/providers/subscription/disconnect"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("%s %s = %d, want 401", tc.method, tc.path, resp.StatusCode)
		}
	}
}
