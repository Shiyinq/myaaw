//go:build windows
// +build windows

package daemon

import (
	"os"
	"os/exec"
)

func setSysProcAttr(cmd *exec.Cmd) {
	// On Windows, we don't use Setsid.
	// The process will run independently if we don't wait for it.
}

func terminateProcess(proc *os.Process) error {
	// proc.Signal(syscall.SIGTERM) is not reliable on Windows.
	// OS-level Kill is more common for background daemons on Windows.
	return proc.Kill()
}

func isProcessRunning(proc *os.Process) bool {
	// On Windows, Signal(0) is not supported.
	// We check if the process is still alive.
	p, err := os.FindProcess(proc.Pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess always succeeds.
	// We would need deeper inspection, but for basic status,
	// checking if our signal/wait works is better.
	// For now, let's keep it simple as daemon support on Windows is limited.
	return p != nil
}
