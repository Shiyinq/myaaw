package agent

import (
	"encoding/json"
	"fmt"
	"log"

	"myaaw/internal/agent/tools"
	"myaaw/internal/provider"
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

// reconstructHistory expands messages that contain a Trace into their constituent
// assistant thought/tool call and tool response messages. This ensures the LLM
// has full context of previous reasoning without leaking internal trace tags into content.
func reconstructHistory(messages []provider.Message) []provider.Message {
	var expanded []provider.Message
	for _, msg := range messages {
		// We only reconstruct from assistant messages that have a trace.
		if len(msg.Trace) > 0 && msg.Role == "assistant" {
			groups := groupTraceSteps(msg.Trace)
			for _, group := range groups {
				// 1. Build ONE assistant message with ALL ToolCalls in this group
				var toolCalls []provider.ToolCall
				thought := group[0].Thought

				for i, step := range group {
					var args interface{}
					if step.ActionInput != "" {
						args = step.ActionInput
					}

					toolID := fmt.Sprintf("trace-%s-%d", step.Action, i)

					// ThoughtSignature only on the first ToolCall
					sig := ""
					if i == 0 {
						sig = step.ThoughtSignature
					}

					toolCalls = append(toolCalls, provider.ToolCall{
						ID:   toolID,
						Type: "function",
						Function: provider.FunctionCall{
							Name:             step.Action,
							Arguments:        args,
							ThoughtSignature: sig,
						},
					})
				}

				expanded = append(expanded, provider.Message{
					Role:      "assistant",
					Thought:   thought,
					ToolCalls: toolCalls,
				})

				// 2. Build ALL tool responses for this group
				for i, step := range group {
					toolID := fmt.Sprintf("trace-%s-%d", step.Action, i)
					expanded = append(expanded, provider.Message{
						Role:       "tool",
						Name:       step.Action,
						Content:    step.Observation,
						ToolCallID: toolID,
					})
				}
			}
		}
		// Add the original message
		expanded = append(expanded, msg)
	}

	return expanded
}

// groupTraceSteps groups trace steps by StepGroup.
// Steps with the same non-zero StepGroup are parallel (same iteration).
// Steps with StepGroup=0 (old data) are treated as sequential (each is its own group).
func groupTraceSteps(steps []provider.ReactStep) [][]provider.ReactStep {
	if len(steps) == 0 {
		return nil
	}

	var groups [][]provider.ReactStep
	var currentGroup []provider.ReactStep
	currentGroupID := -1

	for _, step := range steps {
		if step.StepGroup == 0 {
			// Old data: each step is its own group (sequential)
			if len(currentGroup) > 0 {
				groups = append(groups, currentGroup)
				currentGroup = nil
				currentGroupID = -1
			}
			groups = append(groups, []provider.ReactStep{step})
		} else if step.StepGroup != currentGroupID {
			// New group
			if len(currentGroup) > 0 {
				groups = append(groups, currentGroup)
			}
			currentGroup = []provider.ReactStep{step}
			currentGroupID = step.StepGroup
		} else {
			// Same group (parallel)
			currentGroup = append(currentGroup, step)
		}
	}
	if len(currentGroup) > 0 {
		groups = append(groups, currentGroup)
	}

	return groups
}

func (a *Agent) Run(modelName string, messages []provider.Message) (provider.Message, error) {
	messages = reconstructHistory(messages)
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
		messages = append(messages, response) // Add Assistant's tool call message

		// Use real thought from model if available
		thought := ""
		if response.Thought != "" {
			thought = response.Thought
		}

		// Capture ThoughtSignature from the first tool call that has one
		var stepSignature string
		for _, tc := range response.ToolCalls {
			if tc.Function.ThoughtSignature != "" {
				stepSignature = tc.Function.ThoughtSignature
				break
			}
		}

		stepGroup := iteration + 1

		for i, toolCall := range response.ToolCalls {
			functionName := toolCall.Function.Name
			argsStr := argsToString(toolCall.Function.Arguments)

			if thought == "" {
				thought = GenerateThought(functionName)
			}

			LogThought(thought)
			LogAction(functionName, argsStr)

			output := tools.NewTools(functionName, argsStr)
			LogObservation(output)

			// ThoughtSignature only on the first step of the group
			sig := ""
			if i == 0 {
				sig = stepSignature
			}

			traceSteps = append(traceSteps, provider.ReactStep{
				Thought:          thought,
				Action:           functionName,
				ActionInput:      argsStr,
				Observation:      output,
				ThoughtSignature: sig,
				StepGroup:        stepGroup,
			})

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
	messages = reconstructHistory(messages)
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
		if msg.Thought != "" {
			accumulatedResponse.Thought += msg.Thought
		}

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

		// Capture Usage
		if msg.Usage.TotalTokens > 0 {
			accumulatedResponse.Usage = msg.Usage
		}

		return callback(msg)
	}

	err := a.Provider.ChatStream(modelName, messages, internalCallback)
	if err != nil {
		return err
	}

	// After stream finishes, check accumulatedResponse for tools
	if len(accumulatedResponse.ToolCalls) > 0 {
		messages = append(messages, accumulatedResponse)

		// Use real thought from model if available
		thought := ""
		if accumulatedResponse.Thought != "" {
			thought = accumulatedResponse.Thought
		}

		// Capture ThoughtSignature from the first tool call that has one
		var stepSignature string
		for _, tc := range accumulatedResponse.ToolCalls {
			if tc.Function.ThoughtSignature != "" {
				stepSignature = tc.Function.ThoughtSignature
				break
			}
		}

		stepGroup := iteration + 1

		for i, toolCall := range accumulatedResponse.ToolCalls {
			functionName := toolCall.Function.Name
			argsStr := argsToString(toolCall.Function.Arguments)

			if thought == "" {
				thought = GenerateThought(functionName)
			}

			LogThought(thought)
			LogAction(functionName, argsStr)

			output := tools.NewTools(functionName, argsStr)
			LogObservation(output)

			// ThoughtSignature only on the first step of the group
			sig := ""
			if i == 0 {
				sig = stepSignature
			}

			traceSteps = append(traceSteps, provider.ReactStep{
				Thought:          thought,
				Action:           functionName,
				ActionInput:      argsStr,
				Observation:      output,
				ThoughtSignature: sig,
				StepGroup:        stepGroup,
			})

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
