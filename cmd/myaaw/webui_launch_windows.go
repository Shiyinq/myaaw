//go:build windows

package main

import (
	"os"
	"os/exec"
)

// launchNode spawns Node and waits for it. Windows has no process-replacing
// exec, so the daemon tracks this wrapper; stopping it kills the child as well
// on the supported PID-based fallback.
func launchNode(node, entry string, env []string) error {
	cmd := exec.Command(node, entry)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
