package bash

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type BashTool struct{}

type BashArgs struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"` // Timeout in seconds
}

// Simple blacklist of dangerous commands/keywords
var dangerousCommands = []string{
	"rm ", "rm -", // Deletion
	"sudo", "su ", // Privilege escalation
	"shutdown", "reboot", "halt", "poweroff", // System control
	"mkfs", "dd ", // Disk operations
	":(){:|:&};:",                // Fork bomb
	"> /dev/sda",                 // Device overwriting
	"mv /",                       // Move root (unlikely but dangerous)
	"chmod -R 777 /", "chown -R", // Permission destruction
	"wget ", "curl ", // Downloading scripts (can be used for legitimate purposes, but risky in this context without review)
	// Add more as needed
}

func NewBashTool() *BashTool {
	return &BashTool{}
}

func isCommandSafe(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	for _, dangerous := range dangerousCommands {
		if strings.Contains(cmd, dangerous) {
			return false
		}
	}
	return true
}

func (b *BashTool) CallTool(arguments string) string {
	var args BashArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	if args.Command == "" {
		return "Error: 'command' argument is required."
	}

	if !isCommandSafe(args.Command) {
		return fmt.Sprintf("Error: Command contains forbidden/dangerous keywords. Blocked for security.")
	}

	// Default timeout to 60 seconds if not specified
	timeout := 60 * time.Second
	if args.Timeout > 0 {
		timeout = time.Duration(args.Timeout) * time.Second
	}

	// Create command execution context
	// Using "bash -c" to allow complex commands (pipes, redirects, etc)
	// If bash is not available, sh could be a fallback, but user requested "bash"
	cmd := exec.Command("bash", "-c", args.Command)

	// Create a timer to kill the process if it runs too long
	// Simple implementation without context for now, or use time.AfterFunc
	// A robust implementation would use context.WithTimeout

	// Let's use a channel to handle timeout/completion
	done := make(chan error, 1)

	// Buffers to capture output
	// cmd.CombinedOutput() is simpler if we don't need separate stdout/stderr

	var output []byte
	var err error

	go func() {
		output, err = cmd.CombinedOutput()
		done <- err
	}()

	select {
	case <-time.After(timeout):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return fmt.Sprintf("Error: Command execution timed out after %v seconds.", args.Timeout)
	case err := <-done:
		if err != nil {
			// If it's an exit code error, we still want the output
			return fmt.Sprintf("Error: %v\nOutput:\n%s", err, string(output))
		}
		return string(output)
	}
}
