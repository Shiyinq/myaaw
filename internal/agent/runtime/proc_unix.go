//go:build !windows

package runtime

import (
	"os/exec"
	"syscall"
	"time"
)

func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-pid, 0); err != nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
