//go:build windows

package runtime

import (
	"os/exec"
	"strconv"
)

func configureProcessGroup(cmd *exec.Cmd) {}

func terminateProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	if pid > 0 {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	}
	_ = cmd.Process.Kill()
}
