package service

import (
	"errors"

	"myaaw/internal/daemon"
)

var ErrServerRunning = errors.New("agent server is already running")

type ServerStatus struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	Port    int    `json:"port,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

type ServerService struct {
	agents *AgentService
}

func NewServerService(agents *AgentService) *ServerService {
	return &ServerService{agents: agents}
}

func (s *ServerService) Status(agentName string) (ServerStatus, error) {
	info, err := s.agents.Get(agentName)
	if err != nil {
		return ServerStatus{}, err
	}

	status := ServerStatus{
		Running: info.Running,
		Port:    info.Port,
		BaseURL: info.BaseURL,
	}
	if manager, err := daemon.NewAgentManager(agentName); err == nil {
		if pid, running, _ := manager.Status(); running {
			status.Running = true
			status.PID = pid
		}
	}
	return status, nil
}

func (s *ServerService) Start(agentName string) error {
	if err := agentdirValidateAgent(agentName); err != nil {
		return err
	}
	if status, err := s.Status(agentName); err == nil && status.Running {
		return ErrServerRunning
	}

	manager, err := daemon.NewAgentManager(agentName)
	if err != nil {
		return err
	}
	return manager.Start([]string{"--agent", agentName, "server", "run"})
}

// Stop is idempotent: stopping a server that is not running succeeds.
func (s *ServerService) Stop(agentName string) error {
	if err := agentdirValidateAgent(agentName); err != nil {
		return err
	}

	manager, err := daemon.NewAgentManager(agentName)
	if err != nil {
		return err
	}
	if _, running, _ := manager.Status(); !running {
		return nil
	}
	return manager.Stop()
}

func (s *ServerService) Restart(agentName string) error {
	if err := s.Stop(agentName); err != nil {
		return err
	}
	return s.Start(agentName)
}
