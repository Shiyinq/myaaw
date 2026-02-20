//go:build !windows
// +build !windows

package daemon

import (
	"os"
	"os/exec"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}

func terminateProcess(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}

func isProcessRunning(proc *os.Process) bool {
	err := proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errno, ok := err.(syscall.Errno); ok && errno == syscall.EPERM {
		return true
	}
	return false
}
