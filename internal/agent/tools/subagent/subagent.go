package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/agent/skills"
	"myaaw/internal/agent/tools"
	"myaaw/internal/agentdir"
	"myaaw/internal/config"
	"myaaw/internal/obs"
	"myaaw/internal/obs/agenttrace"
	"myaaw/internal/provider"

	"github.com/go-resty/resty/v2"
)

const toolSchema = `{
    "type": "function",
    "function": {
        "name": "subagent",
        "description": "Delegate one or more complex tasks to background sub-agents. Each task will run independently in its own ReAct loop. Sub-agents have access to all tools (bash, python, filesystem) and skills. Use this when tasks are complex, time-consuming, or can run in parallel.",
        "parameters": {
            "type": "object",
            "properties": {
                "tasks": {
                    "type": "array",
                    "description": "List of tasks to delegate to sub-agents.",
                    "items": {
                        "type": "object",
                        "properties": {
                            "name": {
                                "type": "string",
                                "description": "Short descriptive name for this sub-agent task (e.g. 'fix-bug', 'run-tests')."
                            },
                            "instruction": {
                                "type": "string",
                                "description": "Detailed instruction of what the sub-agent should accomplish."
                            },
                            "skills": {
                                "type": "string",
                                "description": "Optional. Comma-separated list of skill names to focus on."
                            }
                        },
                        "required": ["name", "instruction"]
                    }
                }
            },
            "required": ["tasks"]
        }
    }
}`

func init() {
	tools.RegisterBuiltin("subagent", NewSubAgentTool())
}

type SubAgentTask struct {
	Name        string `json:"name"`
	Instruction string `json:"instruction"`
	Skills      string `json:"skills"`
}

type SubAgentArgs struct {
	Tasks []SubAgentTask `json:"tasks"`
}

type SubAgentTool struct{}

func NewSubAgentTool() *SubAgentTool {
	return &SubAgentTool{}
}

func (t *SubAgentTool) ToolDefinition() []byte {
	return []byte(toolSchema)
}

func (t *SubAgentTool) CallTool(arguments string, tctx *tools.ToolsContext) string {
	return t.CallToolContext(context.Background(), arguments, tctx)
}

func (t *SubAgentTool) ExecutionMode() tools.ExecutionMode {
	return tools.ExecutionSequential
}

const maxConcurrentSubAgents = 4

var subAgentSlots = make(chan struct{}, maxConcurrentSubAgents)

func (t *SubAgentTool) CallToolContext(ctx context.Context, arguments string, tctx *tools.ToolsContext) string {
	var args SubAgentArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	if len(args.Tasks) == 0 {
		return "Error: 'tasks' array cannot be empty."
	}

	if tctx == nil || tctx.Channel == "" || tctx.UserID == "" || tctx.BaseURL == "" {
		return "Error: Missing context (MYAAW_CHANNEL, MYAAW_USER_ID, MYAAW_BASE_URL). Cannot spawn sub-agents."
	}

	batchID := fmt.Sprintf("batch_%d", time.Now().Unix())
	totalTasks := len(args.Tasks)

	// Validate skills for all tasks before starting
	for _, task := range args.Tasks {
		if err := validateSkills(task.Skills); err != nil {
			return fmt.Sprintf("Error in task '%s': %v", task.Name, err)
		}
	}

	// Background sub-agents are expected to outlive the originating request.
	bgCtx := context.WithoutCancel(ctx)

	// Spawn a goroutine for each task, bounded by maxConcurrentSubAgents.
	for i, task := range args.Tasks {
		go t.runSubAgent(bgCtx, task, tctx.Channel, tctx.UserID, tctx.BaseURL, batchID, i+1, totalTasks)
	}

	return fmt.Sprintf("Successfully started %d sub-agents in background (Batch ID: %s). You will receive notifications as they complete. You do not need to wait or set a cron job.", totalTasks, batchID)
}

// startTrace opens a child trace for this sub-agent task, nested under the tool
// call that spawned it. It returns nil when the parent run is not traced.
func (t *SubAgentTool) startTrace(ctx context.Context, task SubAgentTask, systemPrompt string) *agenttrace.Trace {
	parent := agenttrace.FromContext(ctx)
	if parent == nil {
		return nil
	}

	parentSpan := parent.ToolSpanID(agent.ToolCallIDFromContext(ctx))
	if parentSpan == "" {
		parentSpan = parent.CurrentSpanID()
	}

	return parent.StartChild(obs.RunStart{
		Channel:      "subagent",
		Agent:        task.Name,
		Provider:     config.LLMProviderName,
		Model:        config.LLMDefaultModel,
		Trigger:      "subagent",
		UserMessage:  task.Instruction,
		SystemPrompt: systemPrompt,
		ParentSpanID: parentSpan,
	})
}

// runTraced runs the sub-agent while recording a child trace. It mirrors
// RunContext's error handling so callers see the same result.
func (t *SubAgentTool) runTraced(
	ctx context.Context,
	subAgent agent.AgentProvider,
	task SubAgentTask,
	systemPrompt string,
	messages []provider.Message,
) (provider.Message, error) {
	child := t.startTrace(ctx, task, systemPrompt)

	cfg := agent.DefaultRunConfig(config.LLMDefaultModel)
	if child != nil {
		cfg.RunID = child.RunID()
	}

	result, runErr := subAgent.RunEvents(ctx, cfg, messages, func(event agent.AgentEvent) error {
		child.Event(event)
		return nil
	})

	if child != nil {
		child.End(result)
		if err := child.Close(); err != nil {
			log.Printf("[SubAgent %s] Failed to write trace: %v", task.Name, err)
		}
	}

	if runErr != nil {
		return result.Final, runErr
	}
	if result.StopReason == provider.StopError || result.StopReason == provider.StopAborted {
		if result.Final.ErrorMessage != "" {
			return result.Final, errors.New(result.Final.ErrorMessage)
		}
		return result.Final, fmt.Errorf("agent stopped: %s", result.StopReason)
	}
	return result.Final, nil
}

func (t *SubAgentTool) runSubAgent(ctx context.Context, task SubAgentTask, channelName, userID, baseURL, batchID string, taskNum, totalTasks int) {
	select {
	case subAgentSlots <- struct{}{}:
		defer func() { <-subAgentSlots }()
	case <-ctx.Done():
		log.Printf("[SubAgent %s] Canceled before start: %v", task.Name, ctx.Err())
		return
	}
	log.Printf("[SubAgent %s] Starting task %d/%d for user %s", task.Name, taskNum, totalTasks, userID)

	prov, err := provider.CreateLLMProvider(config.LLMProviderName, config.LLMProviderAPIKey)
	if err != nil {
		log.Printf("[SubAgent %s] Failed to create LLM provider: %v", task.Name, err)
		t.sendHeartbeat(task, channelName, userID, baseURL, batchID, taskNum, totalTasks, fmt.Sprintf("Error: Failed to create LLM provider - %v", err))
		return
	}

	subAgent := agent.NewAgent(prov).WithExcludedTools("subagent", "provider", "memory")

	systemPrompt := fmt.Sprintf(`You are a specialized sub-agent running in the background.
Your name is: %s
Your specific task is: %s

CRITICAL INSTRUCTIONS:
1. Work autonomously to accomplish the task. You have access to tools.
2. If a tool you use runs asynchronously and provides a log file path (e.g. bash/python), wait a few seconds (using bash sleep) and then use the filesystem tool to read the log file yourself. DO NOT set cron jobs.
3. When you have completed the task, formulate a final comprehensive report. Your final text response will be delivered directly to the user as your report.
4. DO NOT attempt to spawn other sub-agents.
5. If you fail, explain why.`, task.Name, task.Instruction)

	if task.Skills != "" {
		systemPrompt += fmt.Sprintf("\n\nYou should focus on using the following skills if applicable: %s", task.Skills)
		// GetSkillsInstruction already filters out disabled skills (skills.json / frontmatter);
		// all remaining enabled skills are appended and the focus line above prioritizes them.
		systemPrompt += skills.GetSkillsInstruction()
	} else {
		systemPrompt += skills.GetSkillsInstruction()
	}

	messages := []provider.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: "Begin working on your task now.",
		},
	}

	res, err := t.runTraced(ctx, subAgent, task, systemPrompt, messages)
	report := provider.MessageText(res)
	if err != nil {
		if strings.TrimSpace(report) != "" {
			report += fmt.Sprintf("\n\n⚠️ Sub-agent ended with an error: %v", err)
		} else {
			report = fmt.Sprintf("Error executing sub-agent task: %v", err)
		}
	}
	if strings.TrimSpace(report) == "" {
		report = "Sub-agent finished without producing a textual report."
	}

	// Save to log file
	jobsDir, err := agentdir.ActiveJobsDir()
	if err != nil {
		log.Printf("[SubAgent %s] Failed to resolve jobs dir: %v", task.Name, err)
		jobsDir = os.TempDir()
	}
	logPath := filepath.Join(jobsDir, fmt.Sprintf("subagent_%s_%d.log", batchID, taskNum))
	os.MkdirAll(filepath.Dir(logPath), 0755)
	os.WriteFile(logPath, []byte(report), 0644)

	// Send result via heartbeat
	t.sendHeartbeat(task, channelName, userID, baseURL, batchID, taskNum, totalTasks, report)
}

func (t *SubAgentTool) sendHeartbeat(task SubAgentTask, channelName, userID, baseURL, batchID string, taskNum, totalTasks int, report string) {
	client := resty.New()
	client.SetTimeout(30 * time.Second)

	formattedReport := fmt.Sprintf("[SUB-AGENT RESULT (%d/%d): %s]\n%s", taskNum, totalTasks, task.Name, report)

	payload := map[string]interface{}{
		"prompt":  formattedReport,
		"to":      userID,
		"channel": channelName,
		"trigger": "subagent",
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(fmt.Sprintf("%s/heartbeat", baseURL))

	if err != nil {
		log.Printf("[SubAgent %s] Failed to send heartbeat: %v", task.Name, err)
	} else if resp.IsError() {
		log.Printf("[SubAgent %s] Heartbeat returned error: %s", task.Name, resp.Status())
	} else {
		log.Printf("[SubAgent %s] Result delivered successfully", task.Name)
	}
}

func validateSkills(skillsStr string) error {
	if skillsStr == "" {
		return nil
	}

	skillsDir, err := agentdir.ActiveSkillsDir()
	if err != nil {
		return err
	}

	for s := range strings.SplitSeq(skillsStr, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		skillPath := filepath.Join(skillsDir, s)
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			return fmt.Errorf("Skill '%s' not found. Please ensure it is installed in the active agent's skills directory (%s)", s, skillsDir)
		}
		if !skills.IsSkillEnabled(s) {
			return fmt.Errorf("Skill '%s' is disabled. Enable it in %s", s, filepath.Join(skillsDir, "skills.json"))
		}
	}

	return nil
}
