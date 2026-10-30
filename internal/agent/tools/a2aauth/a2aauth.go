package a2aauth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"myaaw/internal/agent/tools"
)

const (
	envTaskDir       = "MYAAW_A2A_TASK_DIR"
	authRequiredFile = "auth-required.json"
)

// Tool lets an agent that is serving an A2A task request credentials or an
// authorization step from the caller. The A2A service turns the marker file
// into an auth-required task state (spec §7.6).
type Tool struct{}

func init() {
	tools.RegisterBuiltin("a2a_auth_request", New())
}

func New() *Tool {
	return &Tool{}
}

func (t *Tool) ToolDefinition() []byte {
	return []byte(`{
		"type": "function",
		"function": {
			"name": "a2a_auth_request",
			"description": "Ask the A2A caller to authenticate or authorize before the task can continue. Use only while handling an A2A task. The task pauses in auth-required state until the caller resumes it.",
			"parameters": {
				"type": "object",
				"properties": {
					"message": {
						"type": "string",
						"description": "What the caller must do, for example which credential or scope is required"
					}
				},
				"required": ["message"]
			}
		}
	}`)
}

func (t *Tool) CallTool(arguments string, _ *tools.ToolsContext) string {
	var args struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}
	message := strings.TrimSpace(args.Message)
	if message == "" {
		return "Error: message is required"
	}

	dir := strings.TrimSpace(os.Getenv(envTaskDir))
	if dir == "" {
		return "Error: a2a_auth_request is only available while handling an A2A task"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Sprintf("Error preparing task directory: %v", err)
	}
	data, err := json.Marshal(map[string]string{"message": message})
	if err != nil {
		return fmt.Sprintf("Error encoding message: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, authRequiredFile), data, 0o644); err != nil {
		return fmt.Sprintf("Error recording auth request: %v", err)
	}
	return "Authentication request recorded. Stop working and wait for the caller to authenticate."
}
