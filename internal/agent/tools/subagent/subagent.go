package subagent

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/agent/tools"
	"myaaw/internal/config"
	"myaaw/internal/provider"

	"github.com/go-resty/resty/v2"
)

func init() {
	tools.Register("subagent", NewSubAgentTool())
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

func (t *SubAgentTool) CallTool(arguments string, ctx *tools.ToolsContext) string {
	var args SubAgentArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	if len(args.Tasks) == 0 {
		return "Error: 'tasks' array cannot be empty."
	}

	if ctx == nil || ctx.Channel == "" || ctx.UserID == "" || ctx.BaseURL == "" {
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

	// Spawn a goroutine for each task
	for i, task := range args.Tasks {
		go t.runSubAgent(task, ctx.Channel, ctx.UserID, ctx.BaseURL, batchID, i+1, totalTasks)
	}

	return fmt.Sprintf("Successfully started %d sub-agents in background (Batch ID: %s). You will receive notifications as they complete. You do not need to wait or set a cron job.", totalTasks, batchID)
}

func (t *SubAgentTool) runSubAgent(task SubAgentTask, channelName, userID, baseURL, batchID string, taskNum, totalTasks int) {
	log.Printf("[SubAgent %s] Starting task %d/%d for user %s", task.Name, taskNum, totalTasks, userID)

	prov, err := provider.CreateLLMProvider(config.LLMProviderName, config.LLMProviderAPIKey)
	if err != nil {
		log.Printf("[SubAgent %s] Failed to create LLM provider: %v", task.Name, err)
		t.sendHeartbeat(task, channelName, userID, baseURL, batchID, taskNum, totalTasks, fmt.Sprintf("Error: Failed to create LLM provider - %v", err))
		return
	}

	subAgent := agent.NewAgent(prov)

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
		systemPrompt += agent.GetSkillsInstruction() // In a real scenario we'd filter this, but for now we append all and tell it to focus
	} else {
		systemPrompt += agent.GetSkillsInstruction()
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

	res, err := subAgent.Run(config.LLMDefaultModel, messages)
	var report string
	if err != nil {
		report = fmt.Sprintf("Error executing sub-agent task: %v", err)
	} else {
		report = res.Content.(string)
	}

	// Save to log file
	homeDir, _ := os.UserHomeDir()
	logPath := filepath.Join(homeDir, ".myaaw", "jobs", fmt.Sprintf("subagent_%s_%d.log", batchID, taskNum))
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

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	
	skillsDir := filepath.Join(homeDir, ".myaaw", "skills")
	skills := strings.Split(skillsStr, ",")

	for _, s := range skills {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		skillPath := filepath.Join(skillsDir, s)
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			return fmt.Errorf("Skill '%s' not found. Please ensure it is installed in ~/.myaaw/skills/", s)
		}
	}

	return nil
}
