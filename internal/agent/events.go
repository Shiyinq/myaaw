package agent

import "myaaw/internal/provider"

// EventType identifies an agent lifecycle event.
type EventType int

const (
	EventAgentStart EventType = iota
	EventAgentEnd
	EventTurnStart
	EventTurnEnd
	EventMessageStart
	EventMessageUpdate
	EventMessageEnd
	EventToolExecutionStart
	EventToolExecutionUpdate
	EventToolExecutionEnd
	EventTraceUpdate
	EventUsage
)

// AgentEvent is emitted by the agent loop. Consumers should treat it as a
// discriminated union keyed by Type.
type AgentEvent struct {
	Type EventType

	Message *provider.Message // message lifecycle

	Delta         string // EventMessageUpdate (user-visible text)
	ThinkingDelta string // EventMessageUpdate (reasoning text)

	ToolCallID    string      // tool execution lifecycle
	ToolName      string      // tool execution lifecycle
	ToolArguments string      // raw JSON arguments (tool execution start)
	ToolResult    *ToolResult // tool execution lifecycle

	Trace []provider.ReactStep // EventTraceUpdate / tool execution

	Usage      provider.Usage      // EventUsage / EventAgentEnd (aggregated)
	StopReason provider.StopReason // EventAgentEnd
	Err        error               // EventAgentEnd
}

// RunResult is the final state of one agent run.
type RunResult struct {
	// Transcript is the full context after the run, including the leading
	// system message. It is safe to persist after dropping the system message.
	Transcript []provider.Message
	// NewMessages contains only the messages produced by this run (prompts,
	// assistant turns, and tool results).
	NewMessages []provider.Message
	// Final is the last assistant message of the run.
	Final provider.Message
	// Usage is aggregated across every provider request in the run.
	Usage provider.Usage
	// StopReason explains why the run ended.
	StopReason provider.StopReason
	// Err is set for provider errors and cancellations.
	Err error
}
