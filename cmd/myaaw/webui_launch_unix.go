//go:build !windows

package main

import "syscall"

// launchNode replaces the current process with Node on Unix, so the daemon's
// tracked PID is the Node server itself. That makes `webui stop` terminate the
// server cleanly without signal-forwarding plumbing.
func launchNode(node, entry string, env []string) error {
	return syscall.Exec(node, []string{node, entry}, env)
}
