package service

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"myaaw/internal/agentdir"
)

var (
	ErrAgentNotFound    = errors.New("agent not found")
	ErrAgentUnavailable = errors.New("agent server is not reachable")
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

// List returns all initialized agents with their default flag and status.
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
