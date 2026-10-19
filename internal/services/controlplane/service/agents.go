package service

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	myaaw "myaaw"
	"myaaw/internal/agentdir"
	"myaaw/internal/daemon"
)

var (
	ErrAgentNotFound    = errors.New("agent not found")
	ErrAgentUnavailable = errors.New("agent server is not reachable")
	ErrAgentExists      = errors.New("agent already exists")
	ErrInvalidAgentName = errors.New("invalid agent name")
	ErrDefaultAgent     = errors.New("default agent cannot be deleted")
)

// AgentInfo is the public shape returned by GET /agents.
type AgentInfo struct {
	Name    string `json:"name"`
	Default bool   `json:"default"`
	Running bool   `json:"running"`
	Port    int    `json:"port,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

// AgentService resolves per-agent paths, ports, and reachability.
type AgentService struct {
	reachTimeout time.Duration
}

func NewAgentService() *AgentService {
	return &AgentService{reachTimeout: 150 * time.Millisecond}
}

// Create provisions a new isolated agent from the embedded template.
func (s *AgentService) Create(name string) (AgentInfo, error) {
	if err := agentdir.ValidateName(name); err != nil {
		return AgentInfo{}, fmt.Errorf("%w: %v", ErrInvalidAgentName, err)
	}
	if err := agentdir.EnsureLayout(); err != nil {
		return AgentInfo{}, err
	}
	if agentdir.AgentInitialized(name) {
		return AgentInfo{}, ErrAgentExists
	}

	root, err := agentdir.AgentRoot(name)
	if err != nil {
		return AgentInfo{}, err
	}
	if err := extractEmbedDir(".myaaw", root); err != nil {
		return AgentInfo{}, err
	}
	for _, dir := range []string{
		filepath.Join(root, "home", "workspace", "projects"),
		filepath.Join(root, "home", "workspace", "downloads"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return AgentInfo{}, err
		}
	}

	envPath, err := agentdir.EnvPath(name)
	if err != nil {
		return AgentInfo{}, err
	}
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		if err := os.WriteFile(envPath, []byte(myaaw.DefaultEnvExample), 0o644); err != nil {
			return AgentInfo{}, err
		}
	}

	port := agentdir.SuggestedPort(name)
	if err := agentdir.UpsertEnvValue(envPath, "PORT", strconv.Itoa(port)); err != nil {
		return AgentInfo{}, err
	}
	if err := agentdir.UpsertEnvValue(envPath, "MYAAW_BASE_URL", fmt.Sprintf("http://localhost:%d", port)); err != nil {
		return AgentInfo{}, err
	}

	_ = agentdir.RewriteLegacyPaths(root, name)
	if err := agentdir.RegisterAgent(name); err != nil {
		return AgentInfo{}, err
	}

	return s.Get(name)
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

func extractEmbedDir(src, dest string) error {
	entries, err := myaaw.DefaultMyaawDir.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := path.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())
		if entry.IsDir() {
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return err
			}
			if err := extractEmbedDir(srcPath, destPath); err != nil {
				return err
			}
			continue
		}
		data, err := myaaw.DefaultMyaawDir.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return err
		}
	}
	return nil
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

	agents := make([]AgentInfo, 0, len(names))
	for _, name := range names {
		info := AgentInfo{Name: name, Default: name == registry.Default}
		baseURL, port, err := s.resolve(name)
		if err == nil {
			info.BaseURL = baseURL
			info.Port = port
			info.Running = s.reachable(baseURL, port)
		}
		agents = append(agents, info)
	}
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

	info := AgentInfo{Name: name, Default: name == registry.Default}
	baseURL, port, err := s.resolve(name)
	if err != nil {
		return info, nil
	}
	info.BaseURL = baseURL
	info.Port = port
	info.Running = s.reachable(baseURL, port)
	return info, nil
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
