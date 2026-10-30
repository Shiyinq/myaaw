package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupToolsAgent(t *testing.T) string {
	t.Helper()
	return setupConfigAgent(t, nil)
}

func readToolsFile(t *testing.T, agentRoot string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(agentRoot, "tools", "tools.json"))
	if err != nil {
		t.Fatalf("read tools.json: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal tools.json: %v", err)
	}
	return parsed
}

func TestToolsServiceListDefaults(t *testing.T) {
	agentRoot := setupToolsAgent(t)
	svc := NewToolsService()

	view, err := svc.List("myfriend")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(view.Builtin) != 9 {
		t.Fatalf("builtin = %+v", view.Builtin)
	}
	for _, tool := range view.Builtin {
		if !tool.Enabled {
			t.Fatalf("builtin %s should default to enabled", tool.Name)
		}
	}
	// The list must come from the registry, so every registered tool is present
	// (a2a_auth_request used to be missing from a hardcoded copy).
	registered := false
	for _, tool := range view.Builtin {
		if tool.Name == "a2a_auth_request" {
			registered = true
			if tool.Description == "" {
				t.Fatal("a2a_auth_request has no description")
			}
		}
	}
	if !registered {
		t.Fatal("a2a_auth_request missing from the builtin list")
	}
	if len(view.Custom) != 0 || len(view.MCPServers) != 0 {
		t.Fatalf("view = %+v", view)
	}

	// Reading must not create tools.json.
	if _, err := os.Stat(filepath.Join(agentRoot, "tools", "tools.json")); !os.IsNotExist(err) {
		t.Fatalf("List should not create tools.json, err = %v", err)
	}
}

func TestToolsServiceToggleBuiltin(t *testing.T) {
	agentRoot := setupToolsAgent(t)
	svc := NewToolsService()

	if err := svc.ToggleBuiltin("myfriend", "bash", false); err != nil {
		t.Fatalf("ToggleBuiltin: %v", err)
	}

	parsed := readToolsFile(t, agentRoot)
	builtin, _ := parsed["builtin_tools"].(map[string]any)
	if builtin["bash"] != false {
		t.Fatalf("builtin_tools = %+v", builtin)
	}

	view, err := svc.List("myfriend")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	enabled := map[string]bool{}
	for _, tool := range view.Builtin {
		enabled[tool.Name] = tool.Enabled
	}
	if enabled["bash"] || !enabled["filesystem"] {
		t.Fatalf("enabled = %+v", enabled)
	}

	// python is an alias for execute_python in the loader.
	if err := svc.ToggleBuiltin("myfriend", "python", false); err != nil {
		t.Fatalf("ToggleBuiltin python: %v", err)
	}
	view, err = svc.List("myfriend")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, tool := range view.Builtin {
		if tool.Name == "execute_python" && tool.Enabled {
			t.Fatalf("execute_python should follow the python alias, builtin = %+v", view.Builtin)
		}
	}

	if err := svc.ToggleBuiltin("myfriend", "nope", true); !errors.Is(err, ErrUnknownBuiltin) {
		t.Fatalf("err = %v, want ErrUnknownBuiltin", err)
	}
}

func TestToolsServiceCustomLifecycle(t *testing.T) {
	agentRoot := setupToolsAgent(t)
	svc := NewToolsService()

	view, err := svc.CreateCustom("myfriend", CustomToolInput{
		Name:    "hello_helper",
		Command: "node",
		Args:    []string{"/path/script.js"},
		Schema:  json.RawMessage(`{"type":"function","function":{"name":"hello_helper"}}`),
	})
	if err != nil {
		t.Fatalf("CreateCustom: %v", err)
	}
	if view.Name != "hello_helper" || !view.Enabled || view.Command != "node" {
		t.Fatalf("view = %+v", view)
	}
	if len(view.Schema) == 0 {
		t.Fatalf("inline schema should be returned: %+v", view)
	}

	if _, err := svc.CreateCustom("myfriend", CustomToolInput{Name: "hello_helper", Command: "node"}); !errors.Is(err, ErrToolNameTaken) {
		t.Fatalf("err = %v, want ErrToolNameTaken", err)
	}
	if _, err := svc.CreateCustom("myfriend", CustomToolInput{Name: "HELLO_HELPER", Command: "node"}); !errors.Is(err, ErrToolNameTaken) {
		t.Fatalf("case-insensitive duplicate err = %v, want ErrToolNameTaken", err)
	}
	if _, err := svc.CreateCustom("myfriend", CustomToolInput{Name: "bash", Command: "node"}); !errors.Is(err, ErrInvalidToolName) {
		t.Fatalf("err = %v, want ErrInvalidToolName", err)
	}
	if _, err := svc.CreateCustom("myfriend", CustomToolInput{Name: "tool", Command: " "}); !errors.Is(err, ErrInvalidToolConfig) {
		t.Fatalf("err = %v, want ErrInvalidToolConfig", err)
	}

	// Schema validation follows the OpenAI function format from the docs.
	for _, bad := range []json.RawMessage{
		json.RawMessage(`not json`),
		json.RawMessage(`{"type":"object"}`),
		json.RawMessage(`{"type":"function","function":{}}`),
		json.RawMessage(`{"type":"function","function":{"name":"other"}}`),
	} {
		if _, err := svc.CreateCustom("myfriend", CustomToolInput{Name: "schema_test", Command: "node", Schema: bad}); !errors.Is(err, ErrInvalidToolConfig) {
			t.Errorf("schema %s err = %v, want ErrInvalidToolConfig", bad, err)
		}
	}
	validSchema := json.RawMessage(`{"type":"function","function":{"name":"hello_helper"}}`)
	if _, err := svc.PatchCustom("myfriend", "hello_helper", CustomToolPatch{Schema: &validSchema}); err != nil {
		t.Fatalf("PatchCustom valid schema: %v", err)
	}

	// A custom tool name colliding with an MCP server prefix is rejected, and
	// the reverse direction is checked when creating the MCP server.
	if _, err := svc.CreateMCP("myfriend", MCPServerInput{Name: "hello", Command: "npx"}); !errors.Is(err, ErrInvalidToolName) {
		t.Fatalf("mcp prefix collision err = %v, want ErrInvalidToolName", err)
	}
	if _, err := svc.CreateMCP("myfriend", MCPServerInput{Name: "play", Command: "npx"}); err != nil {
		t.Fatalf("CreateMCP: %v", err)
	}
	if _, err := svc.CreateCustom("myfriend", CustomToolInput{Name: "play_tool", Command: "node"}); !errors.Is(err, ErrInvalidToolName) {
		t.Fatalf("custom prefix collision err = %v, want ErrInvalidToolName", err)
	}

	patched, err := svc.PatchCustom("myfriend", "hello_helper", CustomToolPatch{
		Enabled: boolPtr(false),
		Command: strPtr("python3"),
		Args:    &[]string{"run.py"},
	})
	if err != nil {
		t.Fatalf("PatchCustom: %v", err)
	}
	if patched.Enabled || patched.Command != "python3" || len(patched.Args) != 1 {
		t.Fatalf("patched = %+v", patched)
	}

	// A null schema removes the inline schema.
	nullSchema := json.RawMessage("null")
	if _, err := svc.PatchCustom("myfriend", "hello_helper", CustomToolPatch{Schema: &nullSchema}); err != nil {
		t.Fatalf("PatchCustom schema: %v", err)
	}
	raw := readToolsFile(t, agentRoot)
	if strings.Contains(string(mustJSON(t, raw)), `"schema"`) {
		t.Fatalf("schema should be removed: %s", string(mustJSON(t, raw)))
	}

	if _, err := svc.PatchCustom("myfriend", "missing", CustomToolPatch{Enabled: boolPtr(true)}); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("err = %v, want ErrToolNotFound", err)
	}

	if err := svc.DeleteCustom("myfriend", "hello_helper"); err != nil {
		t.Fatalf("DeleteCustom: %v", err)
	}
	if err := svc.DeleteCustom("myfriend", "hello_helper"); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("err = %v, want ErrToolNotFound", err)
	}
}

func TestToolsServiceMCPLifecycle(t *testing.T) {
	setupToolsAgent(t)
	svc := NewToolsService()

	view, err := svc.CreateMCP("myfriend", MCPServerInput{
		Name:    "playwright",
		Command: "npx",
		Args:    []string{"-y", "@playwright/mcp"},
		Env:     map[string]string{"TOKEN": "secret"},
	})
	if err != nil {
		t.Fatalf("CreateMCP: %v", err)
	}
	if view.Name != "playwright" || !view.Enabled {
		t.Fatalf("view = %+v", view)
	}
	if !view.Env.Configured || view.Env.Masked == "" || strings.Contains(view.Env.Masked, "secret") {
		t.Fatalf("env should be masked: %+v", view.Env)
	}

	if _, err := svc.CreateMCP("myfriend", MCPServerInput{Name: "playwright", Command: "npx"}); !errors.Is(err, ErrToolNameTaken) {
		t.Fatalf("err = %v, want ErrToolNameTaken", err)
	}

	patched, err := svc.PatchMCP("myfriend", "playwright", MCPServerPatch{
		Command: strPtr("bunx"),
		Env:     &map[string]string{},
	})
	if err != nil {
		t.Fatalf("PatchMCP: %v", err)
	}
	if patched.Command != "bunx" || patched.Env.Configured {
		t.Fatalf("patched = %+v", patched)
	}

	if err := svc.DeleteMCP("myfriend", "playwright"); err != nil {
		t.Fatalf("DeleteMCP: %v", err)
	}
	if err := svc.DeleteMCP("myfriend", "playwright"); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("err = %v, want ErrToolNotFound", err)
	}
}

func TestToolsServicePreservesSectionsAndFreshRead(t *testing.T) {
	agentRoot := setupToolsAgent(t)
	toolsDir := filepath.Join(agentRoot, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}
	seed := `{
		"custom_tools": [{"name": "keep", "command": "node", "enabled": true}],
		"mcp_servers": [{"name": "mcp-keep", "command": "npx"}]
	}`
	if err := os.WriteFile(filepath.Join(toolsDir, "tools.json"), []byte(seed), 0o644); err != nil {
		t.Fatalf("write tools.json: %v", err)
	}

	svc := NewToolsService()
	if err := svc.ToggleBuiltin("myfriend", "cron", false); err != nil {
		t.Fatalf("ToggleBuiltin: %v", err)
	}

	parsed := readToolsFile(t, agentRoot)
	custom, _ := parsed["custom_tools"].([]any)
	mcp, _ := parsed["mcp_servers"].([]any)
	if len(custom) != 1 || len(mcp) != 1 {
		t.Fatalf("sections not preserved: %s", string(mustJSON(t, parsed)))
	}

	// Simulate a terminal edit: remove the custom entry directly in the file.
	edited := strings.Replace(seed, `{"name": "keep", "command": "node", "enabled": true}`, ``, 1)
	if err := os.WriteFile(filepath.Join(toolsDir, "tools.json"), []byte(edited), 0o644); err != nil {
		t.Fatalf("rewrite tools.json: %v", err)
	}
	view, err := svc.List("myfriend")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(view.Custom) != 0 {
		t.Fatalf("List should reflect terminal edits: %+v", view.Custom)
	}
	if len(view.MCPServers) != 1 {
		t.Fatalf("mcp = %+v", view.MCPServers)
	}
}

func TestToolsServiceInvalidJSONDefaults(t *testing.T) {
	agentRoot := setupToolsAgent(t)
	toolsDir := filepath.Join(agentRoot, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}
	if err := os.WriteFile(filepath.Join(toolsDir, "tools.json"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("write tools.json: %v", err)
	}

	svc := NewToolsService()
	view, err := svc.List("myfriend")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(view.Builtin) != 9 || len(view.Custom) != 0 {
		t.Fatalf("view = %+v", view)
	}

	if err := svc.ToggleBuiltin("myfriend", "bash", false); err != nil {
		t.Fatalf("ToggleBuiltin: %v", err)
	}
	parsed := readToolsFile(t, agentRoot)
	builtin, _ := parsed["builtin_tools"].(map[string]any)
	if builtin["bash"] != false {
		t.Fatalf("broken file should be replaced: %+v", parsed)
	}
}

func boolPtr(v bool) *bool    { return &v }
func strPtr(v string) *string { return &v }
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
