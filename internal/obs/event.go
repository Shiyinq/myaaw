package obs

import (
	"time"

	"myaaw/internal/provider"
)

// EventType identifies one step of the trace state machine. The bot service
// translates engine events into these, so this package stays independent of the
// agent loop.
type EventType int

const (
	EventRunStart EventType = iota
	EventStepStart
	EventLLMRequestStart
	EventLLMRetry
	EventCompaction
	EventFirstToken
	EventLLMEnd
	EventToolStart
	EventToolEnd
	EventStepEnd
	EventRunEnd
)

// Event carries everything the recorder needs to place one span.
type Event struct {
	Type EventType
	Time time.Time

	StepIndex int
	Model     string
	// Messages is the exact request sent to the provider.
	Messages []provider.Message
	// Attempt is the 1-based request number carried by EventLLMRetry.
	Attempt int
	// Compaction numbers carried by EventCompaction.
	TokensBefore    int
	TokensAfter     int
	Budget          int
	DroppedMessages int
	DroppedTokens   int

	Usage      provider.Usage
	StopReason provider.StopReason
	IsError    bool
	ErrorText  string

	// Output and Thought are the assistant text and reasoning of a finished
	// model request (EventLLMEnd).
	Output  string
	Thought string

	ToolCallID     string
	ToolName       string
	ToolArguments  string
	ToolOutput     string
	ToolDurationMs int64

	Status Status
	Err    string
}
