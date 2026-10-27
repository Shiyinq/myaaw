package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"myaaw/internal/agentdir"
)

// prepareAgentEnv makes sure an agent has a usable .env: the file exists, PORT
// is assigned from the first free port, and the base URL/allowed origins match
// that port. An existing PORT is preserved so re-running onboarding never moves
// a running agent to a different port.
func prepareAgentEnv(name string) error {
	envPath, err := agentdir.EnvPath(name)
	if err != nil {
		return err
	}
	created, err := ensureEnvFile(envPath)
	if err != nil {
		return fmt.Errorf("create .env: %w", err)
	}

	port := 0
	if !created {
		port, _ = strconv.Atoi(readEnvValue(envPath, "PORT"))
	}
	if port <= 0 {
		port = agentdir.SuggestedPort(name)
		if err := agentdir.UpsertEnvValue(envPath, "PORT", strconv.Itoa(port)); err != nil {
			return fmt.Errorf("set PORT: %w", err)
		}
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

// readEnvValue returns the value of key in a .env file. Missing keys and
// missing files both yield an empty string.
func readEnvValue(path, key string) string {
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

// installBinary copies the running executable into ~/.local/bin so `myaaw`
// keeps working after the build directory is cleaned. Copying (instead of
// moving) keeps the original artifact usable during development.
func installBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	targetDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", targetDir, err)
	}
	target := filepath.Join(targetDir, "myaaw")

	tmp := target + ".tmp"
	if err := copyFile(exe, tmp); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("install binary: %w", err)
	}
	return target, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return fmt.Errorf("copy binary: %w", err)
	}
	return out.Close()
}

// pathContains reports whether dir appears in PATH.
func pathContains(dir string) bool {
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry != "" && filepath.Clean(entry) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}
