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

	// Create temporary directory for Python code
	tempDir, err := os.MkdirTemp("", "python-exec-*")
	if err != nil {
		return fmt.Sprintf("Error creating temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create Python file
	scriptPath := filepath.Join(tempDir, "script.py")
	err = os.WriteFile(scriptPath, []byte(args.Code), 0644)
	if err != nil {
		return fmt.Sprintf("Error writing Python script: %v", err)
	}

	// Determine python and pip executables
	pythonExec := "python3"
	pipExec := "pip3"
	venvUsed := false

	cwd, err := os.Getwd()
	if err == nil {
		// 1. Check CWD (Project)
		venvDir := filepath.Join(cwd, ".venv")
		venvPython := filepath.Join(venvDir, "bin", "python")
		venvPip := filepath.Join(venvDir, "bin", "pip")

		if _, err := os.Stat(venvDir); err == nil {
			pythonExec = venvPython
			pipExec = venvPip
			venvUsed = true
		} else {
			// 2. Check Sandbox Home (Global)
			if homeDir, err := os.UserHomeDir(); err == nil {
				myaawHome := filepath.Join(homeDir, ".myaaw", "home")
				venvHomeDir := filepath.Join(myaawHome, ".venv")
				venvPythonHome := filepath.Join(venvHomeDir, "bin", "python")
				venvPipHome := filepath.Join(venvHomeDir, "bin", "pip")

				if _, err := os.Stat(venvHomeDir); err == nil {
					pythonExec = venvPythonHome
					pipExec = venvPipHome
					venvUsed = true
				} else {
					// Auto-create global venv if neither exists
					log.Printf("Python tool: No .venv found. Creating global venv at %s...", venvHomeDir)
					os.MkdirAll(myaawHome, 0755)
					createCmd := exec.Command("python3", "-m", "venv", venvHomeDir)
					if err := createCmd.Run(); err == nil {
						pythonExec = venvPythonHome
						pipExec = venvPipHome
						venvUsed = true
					} else {
						log.Printf("Python tool: Failed to create venv: %v. Falling back to global python.", err)
					}
				}
			}
		}
	}

	// Install packages if needed
	if args.Packages != "" && venvUsed {
		packages := strings.Split(args.Packages, ",")
		for _, pkg := range packages {
			pkg = strings.TrimSpace(pkg)
			if pkg != "" {
				// Check if package is already installed to optimize execution
				checkCmd := exec.Command(pythonExec, "-c", fmt.Sprintf("import %s", pkg))
				if err := checkCmd.Run(); err != nil {
					log.Printf("Python tool: Installing package %s...", pkg)
					cmd := exec.Command(pipExec, "install", pkg)
					cmd.Dir = tempDir
					if output, err := cmd.CombinedOutput(); err != nil {
						return fmt.Sprintf("Error installing package %s: %v\nOutput: %s", pkg, err, string(output))
					}
				}
			}
		}
	} else if args.Packages != "" && !venvUsed {
		log.Printf("Python tool: Warning: Packages requested but no venv active. Skipping installation to prevent global pollution.")
	}

	// Default timeout to 60 seconds if not specified
	timeout := 60 * time.Second
	if args.Timeout > 0 {
		timeout = time.Duration(args.Timeout) * time.Second
	}

	// Create command execution context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Execute Python code
	cmd := exec.CommandContext(ctx, pythonExec, scriptPath)

	// Set working directory to ~/.myaaw/home
	if homeDir, err := os.UserHomeDir(); err == nil {
		myaawHome := strings.ReplaceAll(filepath.Join(homeDir, ".myaaw", "home"), "\\", "/")
		if info, err := os.Stat(myaawHome); err == nil && info.IsDir() {
			cmd.Dir = myaawHome
		}
	}

	if args.Input != "" {
		cmd.Stdin = strings.NewReader(args.Input)
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
			if f, err := os.CreateTemp(logsDir, "python-output-*.log"); err == nil {
				f.Write(output)
				logPath = f.Name()
				f.Close()
			}
		}

		outStr = outStr[:maxOutputSize] + fmt.Sprintf("\n\n... [Output truncated because it exceeded 32KB limit. Full output saved to: %s. Use 'read_file' tool with start_line and end_line to read it.] ...", logPath)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("Error: Python execution timed out after %v.\nPartial Output:\n%s", timeout, outStr)
	}

	if err != nil {
		return fmt.Sprintf("Error executing Python code: %v\nOutput: %s", err, outStr)
	}

	return outStr
}
