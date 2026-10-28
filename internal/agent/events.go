package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"myaaw/internal/provider"
)

// EventType identifies an agent lifecycle event.
type EventType int

const (
	EventAgentStart EventType = iota
	EventAgentEnd
	EventStepStart
	EventStepEnd
	EventMessageStart
	EventMessageUpdate
	EventMessageEnd
	EventToolExecutionStart
	EventToolExecutionUpdate
	EventToolExecutionEnd
	EventTraceUpdate
	EventUsage
	// EventLLMRequestStart fires immediately before a provider request and
	// carries the exact transcript that request sends.
	EventLLMRequestStart
	// EventLLMRetry fires before a retried provider request. Attempt is 1-based.
	EventLLMRetry
	// EventContextCompacted fires when the transcript was trimmed to fit the
	// context budget, before the request that used the shortened transcript.
	EventContextCompacted
)

// AgentEvent is emitted by the agent loop. Consumers should treat it as a
// discriminated union keyed by Type.
type AgentEvent struct {
	Type EventType

	// RunID identifies the RunEvents invocation that emitted the event, and
	// Time is when it was emitted. Both are filled by the engine.
	RunID     string
	Time      time.Time
	StepIndex int

	Message *provider.Message // message lifecycle

	Delta         string // EventMessageUpdate (user-visible text)
	ThinkingDelta string // EventMessageUpdate (reasoning text)

	ToolCallID    string      // tool execution lifecycle
	ToolName      string      // tool execution lifecycle
	ToolArguments string      // raw JSON arguments (tool execution start)
	ToolResult    *ToolResult // tool execution lifecycle

	// Model and Messages are set on EventLLMRequestStart.
	Model    string
	Messages []provider.Message
	// Attempt is set on EventLLMRetry.
	Attempt int
	// Compact is set on EventContextCompacted.
	Compact CompactStats

	Trace []provider.ReactStep // EventTraceUpdate / tool execution

	Usage      provider.Usage      // EventUsage / EventAgentEnd (aggregated)
	StopReason provider.StopReason // EventAgentEnd
	Err        error               // EventAgentEnd
}

// RunResult is the final state of one agent run.
type RunResult struct {
	// RunID identifies this run across its events and traces.
	RunID string
	// StartedAt and EndedAt bracket the run.
	StartedAt time.Time
	EndedAt   time.Time
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

// newRunID returns a short random identifier for one agent run.
func newRunID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("run_%d", time.Now().UnixNano())
	}
	return "run_" + hex.EncodeToString(raw[:])
}
