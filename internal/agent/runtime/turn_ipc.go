package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"myaaw/internal/agent/tools"
	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	"myaaw/internal/provider"
)

// TurnSideEffects receives structured outputs (extra_messages, attachments)
// pushed by `myaaw mcp-bridge` while an external CLI turn is running.
type TurnSideEffects struct {
	ToolsContext    *tools.ToolsContext
	OnExtraMessages func([]provider.ExtraMessage)
	OnAttachments   func([]channel.Attachment)
}

// BridgeSideEffectPayload is POSTed by `myaaw mcp-bridge` to the local
// per-runtime loopback server when a Myaaw ecosystem tool produces rich blocks
// or file attachments.
type BridgeSideEffectPayload struct {
	ExtraMessages []provider.ExtraMessage `json:"extra_messages,omitempty"`
	Attachments   []channel.Attachment    `json:"attachments,omitempty"`
}

// TurnBridgeServer is a loopback-only, bearer-authenticated IPC server owned by
// an external runtime so `myaaw mcp-bridge` can query the active turn's
// ToolsContext (channel/chat/conversation/user) and stream rich UI blocks back.
type TurnBridgeServer struct {
	url    string
	token  string
	server *http.Server

	mu     sync.RWMutex
	active *TurnSideEffects
}

// NewTurnBridgeServer starts a loopback HTTP server bound to 127.0.0.1:0.
func NewTurnBridgeServer() (*TurnBridgeServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		_ = ln.Close()
		return nil, err
	}
	token := hex.EncodeToString(raw[:])

	s := &TurnBridgeServer{
		url:   "http://" + ln.Addr().String(),
		token: token,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/context", s.handleContext)
	mux.HandleFunc("/side-effects", s.handleSideEffects)
	s.server = &http.Server{Handler: mux}

	go func() {
		_ = s.server.Serve(ln)
	}()
	return s, nil
}

func (s *TurnBridgeServer) URL() string   { return s.url }
func (s *TurnBridgeServer) Token() string { return s.token }

func (s *TurnBridgeServer) SetActive(se *TurnSideEffects) func() {
	s.mu.Lock()
	s.active = se
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		if s.active == se {
			s.active = nil
		}
		s.mu.Unlock()
	}
}

func (s *TurnBridgeServer) Close() error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Close()
}

func (s *TurnBridgeServer) authorized(r *http.Request) bool {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	return auth == "Bearer "+s.token
}

func (s *TurnBridgeServer) handleContext(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s.mu.RLock()
	active := s.active
	s.mu.RUnlock()
	tctx := tools.ToolsContext{}
	if active != nil && active.ToolsContext != nil {
		tctx = *active.ToolsContext
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tctx)
}

func (s *TurnBridgeServer) handleSideEffects(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var payload BridgeSideEffectPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.RLock()
	active := s.active
	s.mu.RUnlock()
	if active != nil {
		if len(payload.ExtraMessages) > 0 && active.OnExtraMessages != nil {
			active.OnExtraMessages(payload.ExtraMessages)
		}
		if len(payload.Attachments) > 0 && active.OnAttachments != nil {
			active.OnAttachments(payload.Attachments)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// MCPServerSpec is the standard MCP server entry understood by ACP, Claude Code
// (`--mcp-config`), and other CLI agents.
type MCPServerSpec struct {
	Name    string            `json:"name,omitempty"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// BuildMCPServers returns the Myaaw MCP bridge entry (when enabled) plus any
// user-configured stdio MCP servers from the active agent's tools/tools.json.
func BuildMCPServers(enabled bool, bridgeURL, bridgeToken string) []MCPServerSpec {
	var servers []MCPServerSpec
	if enabled {
		if exe, err := os.Executable(); err == nil && exe != "" {
			agentName := agentdir.Active()
			env := map[string]string{
				"MYAAW_AGENT": agentName,
			}
			if root, err := agentdir.ActiveRoot(); err == nil && root != "" {
				env["MYAAW_AGENT_ROOT"] = root
			}
			if bridgeURL != "" {
				env["MYAAW_BRIDGE_URL"] = bridgeURL
			}
			if bridgeToken != "" {
				env["MYAAW_BRIDGE_TOKEN"] = bridgeToken
			}
			servers = append(servers, MCPServerSpec{
				Name:    "myaaw",
				Command: exe,
				Args:    []string{"--agent", agentName, "mcp-bridge"},
				Env:     env,
			})
		}
	}

	// Forward user-configured stdio MCP servers from ~/.myaaw/agents/<name>/tools/tools.json.
	if toolsDir, err := agentdir.ActiveToolsDir(); err == nil && toolsDir != "" {
		raw, err := os.ReadFile(filepath.Join(toolsDir, "tools.json"))
		if err == nil {
			var doc struct {
				MCPServers map[string]struct {
					Command  string            `json:"command"`
					Args     []string          `json:"args"`
					Env      map[string]string `json:"env"`
					Disabled bool              `json:"disabled"`
				} `json:"mcp_servers"`
			}
			if json.Unmarshal(raw, &doc) == nil {
				for name, srv := range doc.MCPServers {
					if srv.Disabled || strings.TrimSpace(srv.Command) == "" || name == "myaaw" {
						continue
					}
					servers = append(servers, MCPServerSpec{
						Name:    name,
						Command: srv.Command,
						Args:    srv.Args,
						Env:     srv.Env,
					})
				}
			}
		}
	}

	return servers
}

// WriteMCPConfigFile writes ~/.myaaw/agents/<name>/runtime/mcp-bridge.json in
// the standard {"mcpServers": {...}} format (used by Claude Code --mcp-config).
func WriteMCPConfigFile(servers []MCPServerSpec) (string, error) {
	if len(servers) == 0 {
		return "", nil
	}
	dir, err := agentdir.ActiveRuntimeDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	type fileEntry struct {
		Command string            `json:"command"`
		Args    []string          `json:"args,omitempty"`
		Env     map[string]string `json:"env,omitempty"`
	}
	payload := struct {
		MCPServers map[string]fileEntry `json:"mcpServers"`
	}{
		MCPServers: make(map[string]fileEntry, len(servers)),
	}
	for _, s := range servers {
		payload.MCPServers[s.Name] = fileEntry{
			Command: s.Command,
			Args:    s.Args,
			Env:     s.Env,
		}
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, "mcp-bridge.json")
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return "", fmt.Errorf("write mcp-bridge.json: %w", err)
	}
	return target, nil
}

// BuildCodexMCPArgs encodes MCP server specifications as repeatable
// `-c mcp_servers.<name>.*=...` TOML override flags for `codex app-server`.
// This injects per-agent Myaaw MCP bridge settings without modifying ~/.codex/config.toml.
func BuildCodexMCPArgs(servers []MCPServerSpec) []string {
	var args []string
	for _, s := range servers {
		name := strings.TrimSpace(s.Name)
		cmd := strings.TrimSpace(s.Command)
		if name == "" || cmd == "" {
			continue
		}
		key := formatTOMLKey(name)
		args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.command=%s", key, marshalTOMLString(cmd)))
		if len(s.Args) > 0 {
			args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.args=%s", key, marshalTOMLStringArray(s.Args)))
		}
		if len(s.Env) > 0 {
			keys := make([]string, 0, len(s.Env))
			for k := range s.Env {
				if strings.TrimSpace(k) != "" {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			if len(keys) > 0 {
				parts := make([]string, 0, len(keys))
				for _, k := range keys {
					parts = append(parts, fmt.Sprintf("%s=%s", formatTOMLKey(k), marshalTOMLString(s.Env[k])))
				}
				args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.env={%s}", key, strings.Join(parts, ",")))
			}
		}
	}
	return args
}

func formatTOMLKey(key string) string {
	if key == "" {
		return `""`
	}
	for i := 0; i < len(key); i++ {
		ch := key[i]
		isBare := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-'
		if !isBare {
			return marshalTOMLString(key)
		}
	}
	return key
}

func marshalTOMLString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func marshalTOMLStringArray(items []string) string {
	b, _ := json.Marshal(items)
	return string(b)
}

// SyncPiProjectMCPConfig merges Myaaw's MCP server entries into
// <workingDir>/.pi/mcp.json with `exposure: "direct"` so Pi (`pi --mode rpc --approve`)
// attaches Myaaw ecosystem tools directly while preserving any user-defined servers.
// When `servers` does not include "myaaw", any stale "myaaw" entry is removed.
func SyncPiProjectMCPConfig(workingDir string, servers []MCPServerSpec) (string, error) {
	wd := strings.TrimSpace(workingDir)
	if wd == "" {
		return "", nil
	}
	target := filepath.Join(wd, ".pi", "mcp.json")
	existingBytes, readErr := os.ReadFile(target)
	if len(servers) == 0 && os.IsNotExist(readErr) {
		return "", nil
	}

	root := make(map[string]any)
	if readErr == nil && len(existingBytes) > 0 {
		_ = json.Unmarshal(existingBytes, &root)
	}
	mcpMap, _ := root["mcpServers"].(map[string]any)
	if mcpMap == nil {
		mcpMap = make(map[string]any)
	}

	hasMyaaw := false
	for _, s := range servers {
		name := strings.TrimSpace(s.Name)
		cmd := strings.TrimSpace(s.Command)
		if name == "" || cmd == "" {
			continue
		}
		if name == "myaaw" {
			hasMyaaw = true
		}
		entry := map[string]any{
			"command":  cmd,
			"exposure": "direct",
		}
		if len(s.Args) > 0 {
			entry["args"] = s.Args
		}
		if len(s.Env) > 0 {
			entry["env"] = s.Env
		}
		mcpMap[name] = entry
	}
	if !hasMyaaw {
		delete(mcpMap, "myaaw")
	}
	root["mcpServers"] = mcpMap

	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", fmt.Errorf("create .pi dir: %w", err)
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(target, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write .pi/mcp.json: %w", err)
	}
	return target, nil
}

// FetchTurnContext is used by `myaaw mcp-bridge` to read the active turn's
// ToolsContext from the running agent's TurnBridgeServer.
func FetchTurnContext(ctx context.Context, bridgeURL, bridgeToken string) *tools.ToolsContext {
	if bridgeURL == "" || bridgeToken == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(bridgeURL, "/")+"/context", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+bridgeToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var tctx tools.ToolsContext
	if err := json.NewDecoder(resp.Body).Decode(&tctx); err != nil {
		return nil
	}
	return &tctx
}

// PushTurnSideEffects is used by `myaaw mcp-bridge` to stream ExtraMessages and
// Attachments from a tool call back into the live RunEvents stream.
func PushTurnSideEffects(ctx context.Context, bridgeURL, bridgeToken string, payload BridgeSideEffectPayload) {
	if bridgeURL == "" || bridgeToken == "" {
		return
	}
	if len(payload.ExtraMessages) == 0 && len(payload.Attachments) == 0 {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(bridgeURL, "/")+"/side-effects", strings.NewReader(string(data)))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+bridgeToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}
