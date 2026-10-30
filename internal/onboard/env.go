package onboard

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"myaaw/internal/agentdir"
)

// PrepareEnv makes sure an agent has a usable .env: the file exists, PORT is
// assigned from the first free port, and the base URL/allowed origins match
// that port. An existing PORT is preserved so re-running onboarding never moves
// a running agent to a different port.
func PrepareEnv(name string, desiredPort int) error {
	envPath, err := agentdir.EnvPath(name)
	if err != nil {
		return err
	}

	// Port assignment is serialized so two concurrent onboarding runs cannot
	// pick the same free port.
	release, err := lockPortAssignment()
	if err != nil {
		return err
	}
	defer release()

	created, err := ensureEnvFile(envPath)
	if err != nil {
		return fmt.Errorf("create .env: %w", err)
	}

	existingPort := ""
	if !created {
		existingPort = ReadEnvValue(envPath, "PORT")
	}
	if desiredPort > 0 {
		if err := ensurePortFree(name, existingPort, desiredPort); err != nil {
			return err
		}
	}

	port := desiredPort
	if port <= 0 {
		if parsed, parseErr := strconv.Atoi(existingPort); parseErr == nil {
			port = parsed
		}
	}
	if port <= 0 {
		port = agentdir.SuggestedPort(name)
	}
	if port <= 0 {
		return fmt.Errorf("no free port is available")
	}
	if err := agentdir.UpsertEnvValue(envPath, "PORT", strconv.Itoa(port)); err != nil {
		return fmt.Errorf("set PORT: %w", err)
	}

	baseURL := fmt.Sprintf("http://localhost:%d", port)
	if err := agentdir.UpsertEnvValue(envPath, "MYAAW_BASE_URL", baseURL); err != nil {
		return fmt.Errorf("set MYAAW_BASE_URL: %w", err)
	}
	if err := agentdir.UpsertEnvValue(envPath, "ALLOWED_ORIGINS", "http://localhost:5050,"+baseURL); err != nil {
		return fmt.Errorf("set ALLOWED_ORIGINS: %w", err)
	}
	return nil
}

// ensurePortFree rejects an explicit port that another agent already uses or
// that something else is listening on. Keeping the current port is always
// allowed so re-running onboarding does not fight the daemon.
func ensurePortFree(name, currentPort string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d is out of range (1-65535)", port)
	}
	if currentPort == strconv.Itoa(port) {
		return nil
	}

	if registry, err := agentdir.LoadRegistry(); err == nil {
		for other := range registry.Agents {
			if other == name {
				continue
			}
			otherEnv, err := agentdir.EnvPath(other)
			if err != nil {
				continue
			}
			if ReadEnvValue(otherEnv, "PORT") == strconv.Itoa(port) {
				return fmt.Errorf("port %d is already assigned to agent %q", port, other)
			}
		}
	}

	if isPortListening(port) {
		return fmt.Errorf("port %d is already in use by another process", port)
	}
	return nil
}

func isPortListening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// lockPortAssignment serializes port selection across processes. A stale lock
// older than 30 seconds is removed so a crashed run cannot block onboarding.
func lockPortAssignment() (func(), error) {
	root, err := agentdir.MyaawRoot()
	if err != nil {
		return func() {}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return func() {}, err
	}

	path := filepath.Join(root, ".ports.lock")
	deadline := time.Now().Add(5 * time.Second)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return func() {}, err
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > 30*time.Second {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return func() {}, fmt.Errorf("timed out waiting for the port lock at %s", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// ReadEnvValue returns the value of key in a .env file. Missing keys and
// missing files both yield an empty string.
func ReadEnvValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if name, value, ok := strings.Cut(line, "="); ok && strings.TrimSpace(name) == key {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
