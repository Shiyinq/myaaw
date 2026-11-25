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
