package provider

import (
	"fmt"
	"log"
)

// ReactConfig holds ReAct loop configuration
type ReactConfig struct {
	MaxIterations    int
	WarningThreshold int
}

// DefaultReactConfig is the default configuration for ReAct loop
var DefaultReactConfig = ReactConfig{
	MaxIterations:    20,
	WarningThreshold: 17,
}

// ThoughtPrompt is sent after observation to prompt LLM to think about next step
const ThoughtPrompt = `Based on the tool result above, analyze what happened:
1. If the tool succeeded: Do you have enough information to answer the user? If yes, provide the final answer. If not, plan your next step carefully.
2. If the tool failed or returned an error: DON'T GIVE UP! Check your available skills in the system prompt. Read the SKILL.md documentation using filesystem tool to understand how to use them correctly.
3. Remember: You have skills like Tavily, Scraping, Weather, etc. Use 'filesystem' tool to read '.teo/skills/<skill-name>/SKILL.md' to learn how to use them.
Never tell the user you can't do something without first trying to read the skill documentation!`

// LogThought logs ReAct thought to console
func LogThought(thought string) {
	log.Printf("[ReAct] Thought: %s", thought)
}

// LogAction logs ReAct action to console
func LogAction(action, input string) {
	log.Printf("[ReAct] Action: %s", action)
	log.Printf("[ReAct] Action Input: %s", input)
}

// LogObservation logs ReAct observation to console
func LogObservation(observation string) {
	log.Printf("[ReAct] Observation: %s", observation)
}

// LogIteration logs current iteration number
func LogIteration(iteration int, config ReactConfig, isStream bool) {
	if isStream {
		log.Printf("[ReAct] Stream Iteration %d/%d", iteration+1, config.MaxIterations)
	} else {
		log.Printf("[ReAct] Iteration %d/%d", iteration+1, config.MaxIterations)
	}
}

// LogMaxIterationsReached logs when max iterations is reached
func LogMaxIterationsReached(config ReactConfig) {
	log.Printf("[ReAct] Max iterations reached (%d)", config.MaxIterations)
}

// LogStepWarning logs when approaching step limit
func LogStepWarning(remainingSteps int) {
	log.Printf("[ReAct] Warning: Only %d steps remaining!", remainingSteps)
}

// GetStepAwarenessWarning returns warning message to inject into system prompt
// Returns empty string if not near limit
func GetStepAwarenessWarning(iteration int, config ReactConfig) string {
	if iteration >= config.WarningThreshold {
		remainingSteps := config.MaxIterations - iteration
		LogStepWarning(remainingSteps)
		return fmt.Sprintf("\n\n⚠️ URGENT: You have only %d steps remaining! You MUST provide a final answer to the user NOW. Do not use any more tools unless absolutely critical.", remainingSteps)
	}
	return ""
}

// IsMaxIterationsReached checks if max iterations is reached
func IsMaxIterationsReached(iteration int, config ReactConfig) bool {
	if iteration >= config.MaxIterations {
		LogMaxIterationsReached(config)
		return true
	}
	return false
}

// MaxIterationsMessage returns the message when max iterations is reached
const MaxIterationsMessage = "⚠️ Maximum iterations reached. Task may be incomplete."

// GenerateThought generates a thought based on tool being used
func GenerateThought(toolName string) string {
	return fmt.Sprintf("I need to use the '%s' tool to complete this task", toolName)
}
