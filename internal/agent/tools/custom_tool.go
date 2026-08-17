package tools

import (
	"bytes"
	"context"
	"fmt"

	"os/exec"
	"time"
)

type CustomTool struct {
	name    string
	command string
	args    []string
	schema  []byte
}

func NewCustomTool(name string, command string, args []string, schema []byte) *CustomTool {
	return &CustomTool{
		name:    name,
		command: command,
		args:    args,
		schema:  schema,
	}
}

func (t *CustomTool) ToolDefinition() []byte {
	return t.schema
}

func (t *CustomTool) CallTool(arguments string, ctx *ToolsContext) string {
	// Prepare the command
	cmdArgs := append([]string{}, t.args...)
	cmdArgs = append(cmdArgs, "--execute", arguments)

	ToolsLogger.Printf("Executing CLI Tool '%s': %s %v", t.name, t.command, cmdArgs)

	// Add timeout context
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctxTimeout, resolveCommand(t.command), cmdArgs...)
	cmd.Env = getUserEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctxTimeout.Err() == context.DeadlineExceeded {
			return fmt.Sprintf("Error: CLI Tool '%s' execution timed out after 2 minutes", t.name)
		}
		return fmt.Sprintf("Error executing CLI Tool '%s': %v\nStderr: %s", t.name, err, stderr.String())
	}

	res := stdout.String()
	if len(res) == 0 {
		return "Success (no output)"
	}
	return res
}
