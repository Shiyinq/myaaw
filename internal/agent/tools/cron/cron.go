package cron

import (
	"encoding/json"
	"fmt"
	"myaaw/internal/agent/tools"
	"os"
	"os/exec"
	"strings"
)

const toolSchema = `{
    "type": "function",
    "function": {
        "name": "cron",
        "description": "Manage scheduled jobs (cron) for the agent. You can list, add, remove, run, and view history of jobs. Jobs can restart generic prompts or messages on a schedule.",
        "parameters": {
            "type": "object",
            "properties": {
                "action": {
                    "type": "string",
                    "enum": [
                        "list",
                        "add",
                        "remove",
                        "run",
                        "history"
                    ],
                    "description": "The action to perform."
                },
                "name": {
                    "type": "string",
                    "description": "Name of the job (required for 'add')."
                },
                "cron": {
                    "type": "string",
                    "description": "Cron expression e.g. '0 7 * * *' (optional for 'add')."
                },
                "every": {
                    "type": "string",
                    "description": "Interval e.g. '1h30m' (optional for 'add')."
                },
                "at": {
                    "type": "string",
                    "description": "Specific time or delay e.g. '10m' or '2023-01-01T10:00:00' (optional for 'add')."
                },
                "message": {
                    "type": "string",
                    "description": "The content/prompt to send (required for 'add')."
                },
                "channel": {
                    "type": "string",
                    "description": "Target channel (telegram, discord) (required for 'add')."
                },
                "to": {
                    "type": "string",
                    "description": "Target recipient ID (user ID from the related channel) (required for 'add')."
                },
                "tz": {
                    "type": "string",
                    "description": "Timezone e.g. 'Asia/Jakarta' (optional)."
                },
                "agent": {
                    "type": "string",
                    "description": "Agent ID to handle the job (default: main)."
                },
                "id": {
                    "type": "string",
                    "description": "Job ID (required for 'remove' and 'run')."
                },
                "job_id": {
                    "type": "string",
                    "description": "Job ID for history (optional for 'history', defaults to global)."
                },
                "limit": {
                    "type": "number",
                    "description": "Limit number of history entries (optional)."
                }
            },
            "required": [
                "action"
            ]
        }
    }
}`

func init() {
	tools.Register("cron", NewCronTool())
}

type CronTool struct{}

func NewCronTool() *CronTool {
	return &CronTool{}
}

func (t *CronTool) ToolDefinition() []byte {
	return []byte(toolSchema)
}

func (t *CronTool) CallTool(arguments string, ctx *tools.ToolsContext) string {
	var args map[string]interface{}
	err := json.Unmarshal([]byte(arguments), &args)
	if err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	action, ok := args["action"].(string)
	if !ok {
		return "Error: action argument is required. Available actions: list, add, remove, run, history"
	}

	// Prepare the command
	cmdArgs := []string{"cron", action}

	switch action {
	case "list":
		// No extra args needed
	case "add":
		if name, ok := args["name"].(string); ok && name != "" {
			cmdArgs = append(cmdArgs, "--name", name)
		}
		if cronExpr, ok := args["cron"].(string); ok && cronExpr != "" {
			cmdArgs = append(cmdArgs, "--cron", cronExpr)
		}
		if every, ok := args["every"].(string); ok && every != "" {
			cmdArgs = append(cmdArgs, "--every", every)
		}
		if at, ok := args["at"].(string); ok && at != "" {
			cmdArgs = append(cmdArgs, "--at", at)
		}
		if message, ok := args["message"].(string); ok && message != "" {
			cmdArgs = append(cmdArgs, "--message", message)
		}
		if channel, ok := args["channel"].(string); ok && channel != "" {
			cmdArgs = append(cmdArgs, "--channel", channel)
		}
		if to, ok := args["to"].(string); ok && to != "" {
			cmdArgs = append(cmdArgs, "--to", to)
		}
		if tz, ok := args["tz"].(string); ok && tz != "" {
			cmdArgs = append(cmdArgs, "--tz", tz)
		}
		if agent, ok := args["agent"].(string); ok && agent != "" {
			cmdArgs = append(cmdArgs, "--agent", agent)
		}

	case "remove", "run":
		if id, ok := args["id"].(string); ok && id != "" {
			cmdArgs = append(cmdArgs, id)
		} else {
			return fmt.Sprintf("Error: id is required for %s", action)
		}

	case "history":
		if jobID, ok := args["job_id"].(string); ok && jobID != "" {
			cmdArgs = append(cmdArgs, jobID)
		}
		if limit, ok := args["limit"].(float64); ok && limit > 0 {
			cmdArgs = append(cmdArgs, "--limit", fmt.Sprintf("%d", int(limit)))
		}

	default:
		return fmt.Sprintf("Unknown action: %s", action)
	}

	// Execute command
	// Relies on 'myaaw' being in PATH or the current running binary
	bin := "myaaw"

	cmd := exec.Command(bin, cmdArgs...)

	// Fallback to finding executable if 'myaaw' not in path
	if _, err := exec.LookPath(bin); err != nil {
		if exe, err := os.Executable(); err == nil {
			if strings.Contains(exe, "myaaw") {
				cmd = exec.Command(exe, cmdArgs...)
			}
		}
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error executing command '%s %v': %v\nOutput: %s", bin, cmdArgs, err, string(output))
	}

	return string(output)
}
