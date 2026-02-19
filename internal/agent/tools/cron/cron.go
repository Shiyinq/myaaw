package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type CronTool struct{}

func NewCronTool() *CronTool {
	return &CronTool{}
}

func (t *CronTool) CallTool(arguments string) string {
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
