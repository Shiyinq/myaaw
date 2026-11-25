package service

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/config"
	"myaaw/internal/daemon"
)

var (
	ErrAgentNotFound    = errors.New("agent not found")
	ErrAgentUnavailable = errors.New("agent server is not reachable")
	ErrInvalidAgentName = errors.New("invalid agent name")
	ErrDefaultAgent     = errors.New("default agent cannot be deleted")
)

// AgentInfo is the public shape returned by GET /agents.
type AgentInfo struct {
	Name      string `json:"name"`
	Default   bool   `json:"default"`
	Running   bool   `json:"running"`
	Port      int    `json:"port,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	CreatedAt string `json:"created_at"`
	// AgentColor is always resolved: an agent registered before colours existed
	// reads back as the palette default, so clients never have to know it.
	AgentColor    string `json:"agent_color"`
	RuntimeType   string `json:"runtime_type,omitempty"`
	RuntimePreset string `json:"runtime_preset,omitempty"`
}

// AgentService resolves per-agent paths, ports, and reachability.
type AgentService struct {
	reachTimeout time.Duration
	runtimeMgr   *RuntimeManageService
}

func NewAgentService() *AgentService {
	return &AgentService{reachTimeout: 150 * time.Millisecond}
}

// SetRuntimeManager attaches the shared RuntimeManageService so agent onboarding
// can inherit global runtime defaults stored in controlplane.db.
func (s *AgentService) SetRuntimeManager(mgr *RuntimeManageService) {
	s.runtimeMgr = mgr
}

// Delete removes an agent directory and unregisters it. The default agent is
// protected. A running server is stopped first on a best-effort basis.
func (s *AgentService) Delete(name string) error {
	if err := agentdir.ValidateName(name); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAgentName, err)
	}
	if name == agentdir.DefaultAgentName {
		return ErrDefaultAgent
	}

	registry, err := agentdir.LoadRegistry()
	if err != nil {
		return err
	}
	if registry.Default == name {
		return ErrDefaultAgent
	}

	root, err := agentdir.AgentRoot(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return ErrAgentNotFound
	}

	if manager, err := daemon.NewAgentManager(name); err == nil {
		if _, running, _ := manager.Status(); running {
			_ = manager.Stop()
		}
	}

	// Drop the deleted agent from every remaining agent's contacts before the
	// directory disappears, so no peer keeps a stale reference.
	if err := agentdir.RemoveA2AContacts(name); err != nil {
		return err
	}

	if err := os.RemoveAll(root); err != nil {
		return err
	}
	return agentdir.UnregisterAgent(name)
}

// SetDefault marks an initialized agent as the registry default.
func (s *AgentService) SetDefault(name string) error {
	if err := agentdirValidateAgent(name); err != nil {
		return err
	}
	return agentdir.SetDefault(name)
}

func (s *AgentService) List() ([]AgentInfo, error) {
	names, err := agentdir.ListAgents()
	if err != nil {
		return nil, err
	}

	registry, err := agentdir.LoadRegistry()
	if err != nil {
		return nil, err
	}

	agents := make([]AgentInfo, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		idx := i
		agentName := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			rtType, rtPreset := resolveAgentRuntime(agentName)
			info := AgentInfo{
				Name:          agentName,
				Default:       agentName == registry.Default,
				CreatedAt:     createdAtRFC3339(registry, agentName),
				AgentColor:    agentdir.AgentColorOrDefault(registry.Agents[agentName].AgentColor),
				RuntimeType:   rtType,
				RuntimePreset: rtPreset,
			}
			baseURL, port, rErr := s.resolve(agentName)
			if rErr == nil {
				info.BaseURL = baseURL
				info.Port = port
				info.Running = s.reachable(baseURL, port)
			}
			agents[idx] = info
		}()
	}
	wg.Wait()
	return agents, nil
}

// Get returns one initialized agent.
func (s *AgentService) Get(name string) (AgentInfo, error) {
	if err := agentdir.ValidateName(name); err != nil {
		return AgentInfo{}, ErrAgentNotFound
	}
	if !agentdir.AgentInitialized(name) {
		return AgentInfo{}, ErrAgentNotFound
	}

	registry, err := agentdir.LoadRegistry()
	if err != nil {
		return AgentInfo{}, err
	}

	rtType, rtPreset := resolveAgentRuntime(name)
	info := AgentInfo{
		Name:          name,
		Default:       name == registry.Default,
		CreatedAt:     createdAtRFC3339(registry, name),
		AgentColor:    agentdir.AgentColorOrDefault(registry.Agents[name].AgentColor),
		RuntimeType:   rtType,
		RuntimePreset: rtPreset,
	}
	baseURL, port, err := s.resolve(name)
	if err != nil {
		return info, nil
	}
	info.BaseURL = baseURL
	info.Port = port
	info.Running = s.reachable(baseURL, port)
	return info, nil
}

// createdAtRFC3339 normalizes a registry timestamp to RFC3339 UTC. Older
// entries stored only a date, which is read as UTC midnight so they sort before
// any agent created afterwards.
func createdAtRFC3339(registry *agentdir.Registry, name string) string {
	raw := ""
	if registry != nil {
		if entry, ok := registry.Agents[name]; ok {
			raw = entry.CreatedAt
		}
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	if parsed, err := time.ParseInLocation("2006-01-02", raw, time.UTC); err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	return raw
}

// BaseURL returns the agent's HTTP base URL and port.
func (s *AgentService) BaseURL(name string) (string, int, error) {
	if err := agentdir.ValidateName(name); err != nil {
		return "", 0, ErrAgentNotFound
	}
	if !agentdir.AgentInitialized(name) {
		return "", 0, ErrAgentNotFound
	}
	return s.resolve(name)
}

func (s *AgentService) resolve(name string) (string, int, error) {
	envPath, err := agentdir.EnvPath(name)
	if err != nil {
		return "", 0, err
	}
	env := readEnvFile(envPath)

	if raw := strings.TrimSpace(env["MYAAW_BASE_URL"]); raw != "" {
		parsed, err := url.Parse(raw)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			port := 0
			if p := parsed.Port(); p != "" {
				port, _ = strconv.Atoi(p)
			}
			if port == 0 {
				if parsed.Scheme == "https" {
					port = 443
				} else {
					port = 80
				}
			}
			return strings.TrimRight(raw, "/"), port, nil
		}
	}

	rawPort := strings.TrimSpace(env["PORT"])
	if rawPort == "" {
		return "", 0, fmt.Errorf("agent %q has no PORT or MYAAW_BASE_URL", name)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port <= 0 {
		return "", 0, fmt.Errorf("agent %q has an invalid PORT %q", name, rawPort)
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port), port, nil
}

func (s *AgentService) reachable(baseURL string, port int) bool {
	host := "127.0.0.1"
	if parsed, err := url.Parse(baseURL); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), s.reachTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func resolveAgentRuntime(name string) (string, string) {
	path, err := agentdir.ConfigPath(name)
	if err != nil {
		return config.RuntimeTypeMyaaw, "myaaw"
	}
	cfg, err := config.LoadConfigFile(path)
	if err != nil || cfg == nil || cfg.Runtime == nil {
		return config.RuntimeTypeMyaaw, "myaaw"
	}
	merged := config.MergeRuntime(cfg.Runtime)
	return merged.Type, merged.Preset
}

func readEnvFile(path string) map[string]string {
	values := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return values
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key != "" {
			values[key] = value
		}
	}
	return values
}
