package agent

import (
	"encoding/json"
	"fmt"
	"log"

	"myaaw/internal/provider"
	"myaaw/internal/tools"
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
3. Remember: You have skills like Tavily, Scraping, Weather, etc. Use 'filesystem' tool to read '.myaaw/skills/<skill-name>/SKILL.md' to learn how to use them.
Never tell the user you can't do something without first trying to read the skill documentation!`

// MaxIterationsMessage returns the message when max iterations is reached
const MaxIterationsMessage = "⚠️ Maximum iterations reached. Task may be incomplete."

// AgentProvider abstracts the agent's capabilities
type AgentProvider interface {
	Run(modelName string, messages []provider.Message) (provider.Message, error)
	RunStream(modelName string, messages []provider.Message, callback func(provider.Message) error) error
}

type Agent struct {
	Provider provider.LLMProvider
	Config   ReactConfig
}

func NewAgent(prov provider.LLMProvider) AgentProvider {
	return &Agent{
		Provider: prov,
		Config:   DefaultReactConfig,
	}
}

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

// LogStepWarning logs when approaching step limit
func LogStepWarning(remainingSteps int) {
	log.Printf("[ReAct] Warning: Only %d steps remaining!", remainingSteps)
}

// GetStepAwarenessWarning returns warning message to inject into system prompt
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
		log.Printf("[ReAct] Max iterations reached (%d)", config.MaxIterations)
		return true
	}
	return false
}

// GenerateThought generates a thought based on tool being used
func GenerateThought(toolName string) string {
	return fmt.Sprintf("I need to use the '%s' tool to complete this task", toolName)
}

func (a *Agent) Run(modelName string, messages []provider.Message) (provider.Message, error) {
	return a.runWithIteration(modelName, messages, 0, nil)
}

func (a *Agent) runWithIteration(modelName string, messages []provider.Message, iteration int, traceSteps []provider.ReactStep) (provider.Message, error) {
	if IsMaxIterationsReached(iteration, a.Config) {
		msg := provider.Message{
			Role:    "assistant",
			Content: MaxIterationsMessage,
			Trace:   traceSteps,
		}
		return msg, nil
	}

	LogIteration(iteration, a.Config, false)

	// Inject step awareness warning into system prompt if needed
	if len(messages) > 0 && messages[0].Role == "system" {
		systemText, ok := messages[0].Content.(string)
		if ok {
			warning := GetStepAwarenessWarning(iteration, a.Config)
			if warning != "" {
				// Create a copy of messages to avoid modifying the original slice/underlying array permanently for the caller
				// But here we are just modifying the content for this request
				// To be safe, let's copy the slice and the first message
				newMessages := make([]provider.Message, len(messages))
				copy(newMessages, messages)
				newMessages[0] = provider.Message{
					Role:    messages[0].Role,
					Content: systemText + warning,
				}
				messages = newMessages
			}
		}
	}

	response, err := a.Provider.Chat(modelName, messages)
	if err != nil {
		return provider.Message{}, err
	}

	// Check for ToolCalls
	if len(response.ToolCalls) > 0 {
		// Log Trace
		var currentStep provider.ReactStep

		// For now, we handle the first tool call for trace logging,
		// but we execute all of them.
		// Note from gemini logic: it seemed to process tool calls and return a step.

		messages = append(messages, response) // Add Assistant's tool call message

		for _, toolCall := range response.ToolCalls {
			functionName := toolCall.Function.Name
			argsStr := argsToString(toolCall.Function.Arguments)

			// Log ReAct Trace
			thought := GenerateThought(functionName)
			if response.Thought != "" {
				thought = response.Thought // Use provided thought if available
			}

			LogThought(thought)
			LogAction(functionName, argsStr)

			output := tools.NewTools(functionName, argsStr)
			LogObservation(output)

			// Record step (mostly for the first tool call if multiple, or aggregate?)
			// The original code returned one 'step'. Let's record the last one or accumulate?
			// Provider's Message.Trace is []ReactStep.
			currentStep = provider.ReactStep{
				Thought:     thought,
				Action:      functionName,
				ActionInput: argsStr,
				Observation: output,
			}

			// Add Tool Response Message
			messages = append(messages, provider.Message{
				Role:       "tool",
				Name:       functionName,
				Content:    output,
				ToolCallID: toolCall.ID,
			})
		}

		// Add ThoughtPrompt to encourage reasoning on the result
		messages = append(messages, provider.Message{
			Role:    "user",
			Content: ThoughtPrompt,
		})

		traceSteps = append(traceSteps, currentStep)

		return a.runWithIteration(modelName, messages, iteration+1, traceSteps)
	}

	// Final response
	response.Trace = traceSteps
	return response, nil
}

func argsToString(i interface{}) string {
	if str, ok := i.(string); ok {
		return str
	}
	jsonData, err := json.Marshal(i)
	if err != nil {
		return fmt.Sprintf("%v", i)
	}
	return string(jsonData)
}

func (a *Agent) RunStream(modelName string, messages []provider.Message, callback func(provider.Message) error) error {
	return a.runStreamWithIteration(modelName, messages, callback, 0, nil)
}

func (a *Agent) runStreamWithIteration(modelName string, messages []provider.Message, callback func(provider.Message) error, iteration int, traceSteps []provider.ReactStep) error {
	if IsMaxIterationsReached(iteration, a.Config) {
		return callback(provider.Message{
			Role:    "assistant",
			Content: MaxIterationsMessage,
			Trace:   traceSteps,
		})
	}

	LogIteration(iteration, a.Config, true)

	// Inject warning similar to Run
	if len(messages) > 0 && messages[0].Role == "system" {
		systemText, ok := messages[0].Content.(string)
		if ok {
			warning := GetStepAwarenessWarning(iteration, a.Config)
			if warning != "" {
				newMessages := make([]provider.Message, len(messages))
				copy(newMessages, messages)
				newMessages[0] = provider.Message{
					Role:    messages[0].Role,
					Content: systemText + warning,
				}
				messages = newMessages
			}
		}
	}

	// We wrap the callback to intercept the final full message or tool calls detection
	var accumulatedResponse provider.Message

	// Wrapper callback to pass through to user AND accumulate
	internalCallback := func(msg provider.Message) error {
		// Pass through to user, attaching current trace
		if len(traceSteps) > 0 {
			msg.Trace = traceSteps
		}

		// Accumulate logic (simplistic update)
		if msg.Content != nil {
			if str, ok := msg.Content.(string); ok {
				if accumulatedResponse.Content == nil {
					accumulatedResponse.Content = ""
				}
				accumulatedResponse.Content = accumulatedResponse.Content.(string) + str
			}
		}
		if len(msg.ToolCalls) > 0 {
			accumulatedResponse.ToolCalls = msg.ToolCalls // Usually tool calls come in one go or we need to accumulate them too?
			// For Gemini, they usually come in a chunk.
		}
		// Also update Role, etc if present
		if msg.Role != "" {
			accumulatedResponse.Role = msg.Role
		}

		return callback(msg)
	}

	err := a.Provider.ChatStream(modelName, messages, internalCallback)
	if err != nil {
		return err
	}

	// After stream finishes, check accumulatedResponse for tools
	if len(accumulatedResponse.ToolCalls) > 0 {
		// It's a tool call!
		// Logic similar to Run

		// Send preview of tool usage (optional, or already sent by provider?)
		// In generic agent, we can send a "Thought" or "Status" message if we want.

		messages = append(messages, accumulatedResponse)

		var currentStep provider.ReactStep

		for _, toolCall := range accumulatedResponse.ToolCalls {
			functionName := toolCall.Function.Name
			argsStr := argsToString(toolCall.Function.Arguments)

			thought := GenerateThought(functionName)
			// Log
			LogThought(thought)
			LogAction(functionName, argsStr)

			output := tools.NewTools(functionName, argsStr)
			LogObservation(output)

			currentStep = provider.ReactStep{
				Thought:     thought,
				Action:      functionName,
				ActionInput: argsStr,
				Observation: output,
			}

			messages = append(messages, provider.Message{
				Role:       "tool",
				Name:       functionName,
				Content:    output,
				ToolCallID: toolCall.ID,
			})
		}

		messages = append(messages, provider.Message{
			Role:    "user",
			Content: ThoughtPrompt,
		})

		traceSteps = append(traceSteps, currentStep)

		// Emit trace update immediately so UI can show tool result before next chunk
		if err := callback(provider.Message{
			Role:  "assistant",
			Trace: traceSteps,
		}); err != nil {
			return err
		}

		return a.runStreamWithIteration(modelName, messages, callback, iteration+1, traceSteps)
	}

	return nil
}
