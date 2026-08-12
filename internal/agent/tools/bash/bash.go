package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	Async   bool              `json:"async,omitempty"`
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
	"env ", "printenv ", "env\n", "printenv\n", // Environment variables disclosure
	// Add more as needed
}

func NewBashTool() *BashTool {
	return &BashTool{}
}

func isCommandSafe(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "env" || cmd == "printenv" {
		return false
	}
	for _, dangerous := range dangerousCommands {
		if strings.Contains(cmd, dangerous) {
			return false
		}
	}
	return true
}

func (b *BashTool) CallTool(arguments string, ctx *tools.ToolsContext) string {
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

	if args.Timeout > 60 && !args.Async {
		return "Error: Timeout is set to > 60 seconds, but async is false. You MUST set async to true for tasks that take longer than 60 seconds to prevent blocking the system."
	}

	if args.Async {
		jobID := fmt.Sprintf("job_%d", time.Now().UnixNano())
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Sprintf("Error getting home dir: %v", err)
		}
		
		resultsDir := filepath.Join(homeDir, ".myaaw", "jobs")
		if err := os.MkdirAll(resultsDir, 0755); err != nil {
			return fmt.Sprintf("Error creating jobs dir: %v", err)
		}
		
		outputPath := filepath.Join(resultsDir, jobID+".log")

		go func() {
			outStr := b.executeCommand(args)
			err := os.WriteFile(outputPath, []byte(outStr), 0644)
			if err != nil {
				log.Printf("Error writing async output to file %s: %v", outputPath, err)
			}
		}()

		baseInstruction := fmt.Sprintf("Command started in background. The output will be saved to: %s. IMPORTANT: You MUST immediately use the 'cron' tool to schedule a reminder for yourself to read this file and notify the user of the result.", outputPath)
		if args.Timeout > 300 {
			return baseInstruction + " Since the timeout is > 5 minutes, you MUST schedule multiple cron jobs at intervals to check the progress in case it finishes much earlier."
		}
		return baseInstruction + " Estimate a reasonable wait time based on the task."
	}

	return b.executeCommand(args)
}

func (b *BashTool) executeCommand(args BashArgs) string {
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
