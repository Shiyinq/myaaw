package a2ainput

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"myaaw/internal/agent/tools"
)

const (
	envTaskDir        = "MYAAW_A2A_TASK_DIR"
	inputRequiredFile = "input-required.json"
)

// Tool lets an agent ask the A2A caller for more information. The A2A service
// turns the marker file into an input-required task state.
type Tool struct{}

func init() {
	tools.RegisterBuiltin("a2a_input", New())
}

func New() *Tool {
	return &Tool{}
}

func (t *Tool) ToolDefinition() []byte {
	return []byte(`{
		"type": "function",
		"function": {
			"name": "a2a_input",
			"description": "Ask the A2A caller for missing information. Use only while handling an A2A task. The task pauses in input-required state until the caller replies.",
			"parameters": {
				"type": "object",
				"properties": {
					"question": {
						"type": "string",
						"description": "Question for the caller"
					}
				},
				"required": ["question"]
			}
		}
	}`)
}

func (t *Tool) CallTool(arguments string, _ *tools.ToolsContext) string {
	var args struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}
	question := strings.TrimSpace(args.Question)
	if question == "" {
		return "Error: question is required"
	}

	dir := strings.TrimSpace(os.Getenv(envTaskDir))
	if dir == "" {
		return "Error: a2a_input is only available while handling an A2A task"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Sprintf("Error preparing task directory: %v", err)
	}
	data, err := json.Marshal(map[string]string{"question": question})
	if err != nil {
		return fmt.Sprintf("Error encoding question: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, inputRequiredFile), data, 0o644); err != nil {
		return fmt.Sprintf("Error recording question: %v", err)
	}
	return "Input request recorded. Stop working and wait for the caller's answer."
}
