package python

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
	pipExec := "pip"

	cwd, err := os.Getwd()
	if err == nil {
		venvPython := filepath.Join(cwd, ".venv", "bin", "python")
		venvPip := filepath.Join(cwd, ".venv", "bin", "pip")

		if _, err := os.Stat(venvPython); err == nil {
			pythonExec = venvPython
			pipExec = venvPip
			// log.Printf("Using virtual environment: %s", venvPython)
		}
	}

	// Install packages if needed
	if args.Packages != "" {
		packages := strings.Split(args.Packages, ",")
		for _, pkg := range packages {
			pkg = strings.TrimSpace(pkg)
			if pkg != "" {
				cmd := exec.Command(pipExec, "install", pkg)
				cmd.Dir = tempDir
				// Capture output to debug installation errors if needed
				if output, err := cmd.CombinedOutput(); err != nil {
					return fmt.Sprintf("Error installing package %s: %v\nOutput: %s", pkg, err, string(output))
				}
			}
		}
	}

	// Execute Python code
	cmd := exec.Command(pythonExec, scriptPath)
	if args.Input != "" {
		cmd.Stdin = strings.NewReader(args.Input)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error executing Python code: %v\nOutput: %s", err, string(output))
	}

	return string(output)
}
