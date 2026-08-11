package python

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
	tools.Register("execute_python", NewPythonTool())
}

type PythonTool struct{}

type PythonArgs struct {
	Code     string `json:"code"`
	Timeout  int    `json:"timeout,omitempty"`
	Input    string `json:"input,omitempty"`
	Packages string `json:"packages,omitempty"`
	Async    bool   `json:"async,omitempty"`
}

func NewPythonTool() *PythonTool {
	return &PythonTool{}
}

func (p *PythonTool) CallTool(arguments string) string {
	var args PythonArgs
	err := json.Unmarshal([]byte(arguments), &args)
	if err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
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
			outStr := p.executePython(args)
			err := os.WriteFile(outputPath, []byte(outStr), 0644)
			if err != nil {
				log.Printf("Error writing async output to file %s: %v", outputPath, err)
			}
		}()

		baseInstruction := fmt.Sprintf("Python script started in background. The output will be saved to: %s. IMPORTANT: You MUST immediately use the 'cron' tool to schedule a reminder for yourself to read this file and notify the user of the result.", outputPath)
		if args.Timeout > 300 {
			return baseInstruction + " Since the timeout is > 5 minutes, you MUST schedule multiple cron jobs at intervals to check the progress in case it finishes much earlier."
		}
		return baseInstruction + " Estimate a reasonable wait time based on the task."
	}

	return p.executePython(args)
}

func (p *PythonTool) executePython(args PythonArgs) string {
	// Create temporary directory for Python code
	tempDir, err := os.MkdirTemp("", "python-exec-*")
	if err != nil {
		return fmt.Sprintf("Error creating temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	scriptPath := filepath.Join(tempDir, "script.py")
	err = os.WriteFile(scriptPath, []byte(args.Code), 0644)
	if err != nil {
		return fmt.Sprintf("Error writing script file: %v", err)
	}

	// 1. Check/Setup Virtual Environment
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Sprintf("Error getting home dir: %v", err)
	}

	myaawDir := filepath.Join(homeDir, ".myaaw", "home")
	if err := os.MkdirAll(myaawDir, 0755); err != nil {
		return fmt.Sprintf("Error creating myaaw home dir: %v", err)
	}

	venvDir := filepath.Join(myaawDir, ".venv")
	pythonBin := filepath.Join(venvDir, "bin", "python")
	pipBin := filepath.Join(venvDir, "bin", "pip")

	// Create venv if it doesn't exist
	if _, err := os.Stat(pythonBin); os.IsNotExist(err) {
		log.Println("Creating Python virtual environment...")
		cmd := exec.Command("python3", "-m", "venv", venvDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Sprintf("Error creating virtual environment: %v\nOutput: %s", err, string(output))
		}
	}

	// 2. Install Packages if requested
	if args.Packages != "" {
		packages := strings.Split(args.Packages, ",")
		for i := range packages {
			packages[i] = strings.TrimSpace(packages[i])
		}

		if len(packages) > 0 {
			log.Printf("Installing Python packages: %v", packages)
			// Using pip install from venv
			pipArgs := append([]string{"install"}, packages...)
			cmd := exec.Command(pipBin, pipArgs...)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Sprintf("Error installing packages: %v\nOutput: %s", err, string(output))
			}
		}
	}

	// 3. Execute Script
	timeout := 60 * time.Second // Default timeout
	if args.Timeout > 0 {
		timeout = time.Duration(args.Timeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonBin, scriptPath)

	// Set Working Directory to myaaw home
	cmd.Dir = myaawDir

	// Handle Stdin
	if args.Input != "" {
		cmd.Stdin = strings.NewReader(args.Input)
	}

	output, err := cmd.CombinedOutput()

	// 4. Handle Output & Truncation
	const maxOutputSize = 32 * 1024
	outStr := string(output)

	if len(outStr) > maxOutputSize {
		// Save full output to a file
		logPath := "unknown"
		logsDir := filepath.Join(myaawDir, ".logs")
		os.MkdirAll(logsDir, 0755)
		if f, err := os.CreateTemp(logsDir, "python-output-*.log"); err == nil {
			f.Write(output)
			logPath = f.Name()
			f.Close()
		}

		outStr = outStr[:maxOutputSize] + fmt.Sprintf("\n\n... [Output truncated because it exceeded 32KB limit. Full output saved to: %s. Use 'read_file' tool with start_line and end_line to read it.] ...", logPath)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("Error: Script execution timed out after %v.\nPartial Output:\n%s", timeout, outStr)
	}

	if err != nil {
		return fmt.Sprintf("Error executing script: %v\nOutput:\n%s", err, outStr)
	}

	return outStr
}
