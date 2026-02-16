package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type Manager struct {
	Name    string
	PIDFile string
	LogFile string
}

func NewManager(name string) (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home dir: %w", err)
	}

	baseDir := filepath.Join(home, ".myaaw")
	pidDir := filepath.Join(baseDir, "pids")
	logDir := filepath.Join(baseDir, "logs")

	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create pid dir: %w", err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log dir: %w", err)
	}

	return &Manager{
		Name:    name,
		PIDFile: filepath.Join(pidDir, name+".pid"),
		LogFile: filepath.Join(logDir, name+".log"),
	}, nil
}

func (m *Manager) Start(args []string) error {
	if pid, running, _ := m.Status(); running {
		return fmt.Errorf("%s is already running with PID %d", m.Name, pid)
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, args...)

	logFile, err := os.OpenFile(m.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	if err := os.WriteFile(m.PIDFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	fmt.Printf("✅ %s started in background (PID: %d)\n", m.Name, cmd.Process.Pid)
	fmt.Printf("📄 Logs: %s\n", m.LogFile)

	return nil
}

func (m *Manager) Stop() error {
	pid, running, err := m.Status()
	if err != nil {
		return err
	}
	if !running {
		_ = os.Remove(m.PIDFile)
		return fmt.Errorf("%s is not running", m.Name)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// If permission denied or other error
		return err
	}

	if err := os.Remove(m.PIDFile); err != nil {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}

	fmt.Printf("🛑 %s stopped (PID: %d)\n", m.Name, pid)
	return nil
}

func (m *Manager) Status() (int, bool, error) {
	data, err := os.ReadFile(m.PIDFile)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, err
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, false, fmt.Errorf("invalid PID in file: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return 0, false, nil
	}

	err = proc.Signal(syscall.Signal(0))
	if err != nil {
		if errors.Is(err, os.ErrProcessDone) || err.Error() == "os: process already finished" || strings.Contains(err.Error(), "process already finished") {
			return 0, false, nil
		}
		if sysErr, ok := err.(syscall.Errno); ok {
			if sysErr == syscall.ESRCH {
				return 0, false, nil
			}
		}
		if sysErr, ok := err.(syscall.Errno); ok {
			if sysErr == syscall.EPERM {
				return pid, true, nil
			}
		}

		return 0, false, nil
	}

	return pid, true, nil
}
