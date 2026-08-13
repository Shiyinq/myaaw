//go:build windows

package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

var (
	userPath     string
	userPathOnce sync.Once
)

// resolveUserPath on Windows simply returns os.Getenv("PATH").
// Windows services typically inherit the system PATH which already
// includes globally installed tools like Node.js.
func resolveUserPath() string {
	userPathOnce.Do(func() {
		userPath = os.Getenv("PATH")
	})
	return userPath
}

// resolveCommand resolves a command name to its absolute path.
// On Windows, also checks common extensions (.exe, .cmd, .bat).
func resolveCommand(command string) string {
	if filepath.IsAbs(command) {
		return command
	}

	// Try exec.LookPath first (uses current PATH)
	if resolved, err := exec.LookPath(command); err == nil {
		return resolved
	}

	return command
}

// getUserEnv returns the current process environment as-is on Windows.
func getUserEnv() []string {
	return os.Environ()
}
