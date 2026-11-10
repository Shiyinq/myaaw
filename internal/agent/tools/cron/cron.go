package cron

import (
	"encoding/json"
	"fmt"
	"myaaw/internal/agent/tools"
	"myaaw/internal/agentdir"
	"os"
	"os/exec"
	"strings"
)

const toolSchema = `{
    "type": "function",
    "function": {
        "name": "cron",
        "description": "Manage scheduled jobs (cron) for the agent. You can list, add, remove, run, and view the history of jobs. A history call without a job id lists recent runs; pass the job id to read that job's runs in full, including the prompt and where each one was delivered. Jobs can restart generic prompts or messages on a schedule.",
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
                    "description": "Target channel (telegram, discord, api, cli). Defaults to the current chat when omitted."
                },
                "to": {
                    "type": "string",
                    "description": "Target id: user id for DMs (api, cli, Discord DM), chat id for Telegram, channel id for a Discord guild or thread. Defaults to the current chat when omitted."
                },
                "thread_id": {
                    "type": "string",
                    "description": "Telegram forum message_thread_id. Defaults to the current topic."
                },
                "session_id": {
                    "type": "string",
                    "description": "Exact conversation id to deliver into. Defaults to the current session; required for api/cli."
                },
                "chat_type": {
                    "type": "string",
                    "description": "dm or guild (discord only). A dm target is the user id, never the DM channel id."
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
                    "description": "Job ID (required for 'remove' and 'run'; optional for 'list' to show one job in full)."
                },
                "job_id": {
                    "type": "string",
                    "description": "Job ID for history. Omit it to list recent runs; pass it to read that job's runs in full (prompt, delivery target, topic, session)."
                },
                "limit": {
                    "type": "number",
                    "description": "Maximum runs to list. Only used when job_id is omitted; a job id always returns every run of that job."
                }
            },
            "required": [
                "action"
            ]
        }
    }
}`

func init() {
	tools.RegisterBuiltin("cron", NewCronTool())
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
		// With an id the CLI prints that job in full instead of the table.
		if id := argString(args, "id"); id != "" {
			cmdArgs = append(cmdArgs, id)
		}
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

		// Default the delivery target to the chat the model is talking in, so a
		// simple "remind me in 5 minutes" needs no ids from the user. A job for
		// another chat must not inherit this room's thread, session, or chat type:
		// those belong to the chat the request came from, not to the target.
		channel := argString(args, "channel")
		to := argString(args, "to")
		threadID := argString(args, "thread_id")
		sessionID := argString(args, "session_id")
		chatType := argString(args, "chat_type")
		explicitChatType := chatType != ""
		sameChat := true
		sameRoom := true
		if ctx != nil {
			if to != "" && to != ctx.ChatID && to != ctx.UserID {
				sameChat = false
			}
			sameRoom = sameChat && (threadID == "" || threadID == ctx.TopicID) && (sessionID == "" || sessionID == ctx.ConversationID)

			if channel == "" {
				channel = ctx.Channel
			}
			if to == "" {
				if ctx.Scope != "" {
					to = ctx.ChatID
				} else {
					to = ctx.UserID
				}
			}
			if threadID == "" && sameChat {
				threadID = ctx.TopicID
			}
			if sessionID == "" && sameRoom {
				sessionID = ctx.ConversationID
			}
			if chatType == "" && sameChat {
				chatType = ctx.ChatType
			}
			// A Discord DM is addressed by user id, but <userinfo> shows the DM
			// channel id as ChatID and models copy it. Rewrite it to the speaker so
			// the job opens the right DM instead of failing "Invalid Recipient(s)".
			if strings.EqualFold(chatType, "dm") && to != "" && to == ctx.ChatID && ctx.UserID != "" {
				to = ctx.UserID
			}
		}

		// chat_type describes the *target*, not the chat this message came from.
		// Telegram derives the room kind from the chat id, so only Discord needs
		// it, and only for the room this request came from. An explicit value from
		// the model is always kept.
		if !explicitChatType && (!strings.EqualFold(channel, "discord") || !sameChat) {
			chatType = ""
		}

		if channel != "" {
			cmdArgs = append(cmdArgs, "--channel", channel)
		}
		if to != "" {
			cmdArgs = append(cmdArgs, "--to", to)
		}
		if threadID != "" {
			cmdArgs = append(cmdArgs, "--thread-id", threadID)
		}
		if sessionID != "" {
			cmdArgs = append(cmdArgs, "--session-id", sessionID)
		}
		if chatType != "" {
			cmdArgs = append(cmdArgs, "--chat-type", chatType)
		}
		if tz, ok := args["tz"].(string); ok && tz != "" {
			cmdArgs = append(cmdArgs, "--tz", tz)
		}
		// Always bind the job to the active agent so jobs.json and agent_id never
		// disagree (the old CLI default was the hard-coded "main").
		cmdArgs = append(cmdArgs, "--agent", agentdir.Active())

	case "remove", "run":
		if id, ok := args["id"].(string); ok && id != "" {
			cmdArgs = append(cmdArgs, id)
		} else {
			return fmt.Sprintf("Error: id is required for %s", action)
		}

	case "history":
		// A job id is a detail request: the CLI returns every run of that job, so
		// sending --limit there would only be ignored. Keep limit for the list.
		if jobID := argString(args, "job_id"); jobID != "" {
			cmdArgs = append(cmdArgs, jobID)
		} else if limit, ok := args["limit"].(float64); ok && limit > 0 {
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

	// Set the environment after the binary is chosen: reassigning cmd drops Env,
	// and the child must resolve the same agent as the running server.
	cmd.Env = tools.SetEnv(os.Environ(), "MYAAW_AGENT", agentdir.Active())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error executing command '%s %v': %v\nOutput: %s", bin, cmdArgs, err, string(output))
	}

	return string(output)
}

func argString(args map[string]interface{}, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}
