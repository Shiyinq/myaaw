package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"myaaw/internal/agent/tools"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	tools.Register("bash", NewBashTool())
}

type BashTool struct{}

type BashArgs struct {
	Command string            `json:"command"`
	Timeout int               `json:"timeout,omitempty"` // Timeout in seconds
	Env     map[string]string `json:"env,omitempty"`     // Environment variables
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
		return "Error: Command contains forbidden/dangerous keywords. Blocked for security."
	}

	// Default timeout to 60 seconds if not specified
	timeout := 60 * time.Second
	if args.Timeout > 0 {
		timeout = time.Duration(args.Timeout) * time.Second
	}

	// Create command execution context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Using "bash -c" to allow complex commands (pipes, redirects, etc)
	cmd := exec.CommandContext(ctx, "bash", "-c", args.Command)

	// Set environment variables
	cmd.Env = os.Environ() // Inherit current environment
	for k, v := range args.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set working directory to ~/.myaaw/home to contain execution
	if homeDir, err := os.UserHomeDir(); err == nil {
		myaawHome := strings.ReplaceAll(filepath.Join(homeDir, ".myaaw", "home"), "\\", "/")
		if info, err := os.Stat(myaawHome); err == nil && info.IsDir() {
			cmd.Dir = myaawHome
		}
	}

	output, err := cmd.CombinedOutput()

	// Truncate output if it exceeds 32KB to avoid crashing LLMs
	const maxOutputSize = 32 * 1024
	outStr := string(output)
	if len(outStr) > maxOutputSize {
		// Save full output to a file
		logPath := "unknown"
		if homeDir, err := os.UserHomeDir(); err == nil {
			logsDir := filepath.Join(homeDir, ".myaaw", "home", ".logs")
			os.MkdirAll(logsDir, 0755)
			if f, err := os.CreateTemp(logsDir, "bash-output-*.log"); err == nil {
				f.Write(output)
				logPath = f.Name()
				f.Close()
			}
		}

		outStr = outStr[:maxOutputSize] + fmt.Sprintf("\n\n... [Output truncated because it exceeded 32KB limit. Full output saved to: %s. Use 'read_file' tool with start_line and end_line to read it.] ...", logPath)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("Error: Command execution timed out after %v.\nPartial Output:\n%s", timeout, outStr)
	}

	if err != nil {
		return fmt.Sprintf("Error: %v\nOutput:\n%s", err, outStr)
	}

	return outStr
}
