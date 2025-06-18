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

	// Install packages if needed
	if args.Packages != "" {
		packages := strings.Split(args.Packages, ",")
		for _, pkg := range packages {
			pkg = strings.TrimSpace(pkg)
			if pkg != "" {
				cmd := exec.Command("pip", "install", pkg)
				cmd.Dir = tempDir
				if err := cmd.Run(); err != nil {
					return fmt.Sprintf("Error installing package %s: %v", pkg, err)
				}
			}
		}
	}

	// Execute Python code
	cmd := exec.Command("python3", scriptPath)
	if args.Input != "" {
		cmd.Stdin = strings.NewReader(args.Input)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error executing Python code: %v\nOutput: %s", err, string(output))
	}

	return string(output)
}
