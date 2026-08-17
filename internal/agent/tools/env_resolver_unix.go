//go:build !windows

package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	userPath     string
	userPathOnce sync.Once
)

// resolveUserPath detects the user's full $PATH from their login shell.
// This is critical for background services (LaunchAgent/systemd) which
// don't inherit the user's shell environment (NVM, Homebrew, GVM, etc.).
func resolveUserPath() string {
	userPathOnce.Do(func() {
		currentPath := os.Getenv("PATH")
		// If PATH already contains user directories (Homebrew, /usr/local/bin, nvm, etc.), use it directly
		if strings.Contains(currentPath, "/opt/homebrew") ||
			strings.Contains(currentPath, "/usr/local/bin") ||
			strings.Contains(currentPath, ".nvm") ||
			strings.Contains(currentPath, ".cargo") ||
			len(filepath.SplitList(currentPath)) > 4 {
			userPath = currentPath
			return
		}

		shell := os.Getenv("SHELL")
		if shell == "" {
			if _, err := os.Stat("/bin/zsh"); err == nil {
				shell = "/bin/zsh"
			} else {
				shell = "/bin/sh"
			}
		}

		// Use -ilc (interactive + login) to source .zshrc/.bashrc which often contain nvm/npx paths.
		// cmd.Stdin = strings.NewReader("") prevents SIGTTIN (suspended tty input) hangs.
		marker := "__MYAAW_PATH__"
		cmd := exec.Command(shell, "-ilc", "echo "+marker+"=$PATH")
		cmd.Stdin = strings.NewReader("")

		out, err := cmd.Output()
		if err != nil {
			ToolsLogger.Printf("[WARN] Failed to resolve user PATH from %s: %v", shell, err)
			userPath = currentPath
			return
		}

		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, marker+"=") {
				userPath = strings.TrimPrefix(line, marker+"=")
				return
			}
		}

		// Fallback
		userPath = currentPath
	})

	return userPath
}

// resolveCommand resolves a command name to its absolute path using the user's
// full PATH. If the command is already an absolute path, it is returned as-is.
func resolveCommand(command string) string {
	if filepath.IsAbs(command) {
		return command
	}

	fullPath := resolveUserPath()
	for _, dir := range filepath.SplitList(fullPath) {
		candidate := filepath.Join(dir, command)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate
		}
	}

	// Fallback: return original, let exec.Command handle the error
	return command
}

// getUserEnv returns the current process environment with PATH patched
// to include the user's full shell PATH.
func getUserEnv() []string {
	fullPath := resolveUserPath()
	env := os.Environ()

	found := false
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + fullPath
			found = true
			break
		}
	}
	if !found {
		env = append(env, "PATH="+fullPath)
	}

	return env
}
