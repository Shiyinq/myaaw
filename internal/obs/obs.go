// Package obs records what the agent actually did: one trace per run, split
// into spans for the model requests, the tool calls, and the context work
// around them, together with token usage and model cost.
//
// The package is a camera, not a referee. It observes the loop through plain
// events, never changes a result, and never blocks a caller on I/O: spans are
// written by a background writer and flushed on shutdown.
package obs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"myaaw/internal/provider"
)

// SpanKind identifies one level of the trace tree.
//
// A run maps to one inbound message (the engine's RunEvents call) — the same
// unit deepseek-harness calls a turn. A step maps to one loop iteration: one
// provider request plus the tool calls it produced. llm and tool are the work
// inside a step.
type SpanKind string

const (
	KindRun        SpanKind = "run"
	KindUser       SpanKind = "user"
	KindSystem     SpanKind = "system"
	KindCompaction SpanKind = "compaction"
	KindStep       SpanKind = "step"
	KindLLM        SpanKind = "llm"
	KindTool       SpanKind = "tool"
)

// Status is the outcome of a run, step, or span.
type Status string

const (
	StatusRunning     Status = "running"
	StatusCompleted   Status = "completed"
	StatusError       Status = "error"
	StatusCancelled   Status = "cancelled"
	StatusTruncated   Status = "truncated"
	StatusMaxIters    Status = "max_iterations"
	StatusInterrupted Status = "interrupted"
	// StatusRetried marks an attempt that failed and was retried. It is not an
	// error: the step and the run can still complete normally.
	StatusRetried Status = "retried"
)

// StatusFromStopReason maps an engine stop reason to a trace status.
func StatusFromStopReason(reason provider.StopReason, err error) Status {
	if err != nil {
		if reason == provider.StopAborted {
			return StatusCancelled
		}
		return StatusError
	}
	switch reason {
	case provider.StopError:
		return StatusError
	case provider.StopAborted:
		return StatusCancelled
	case provider.StopLength:
		return StatusTruncated
	case provider.StopMaxIterations:
		return StatusMaxIters
	default:
		return StatusCompleted
	}
}

// Run is one recorded agent invocation.
type Run struct {
	ID             string `json:"id"`
	StartedAt      int64  `json:"started_at"`
	EndedAt        int64  `json:"ended_at,omitempty"`
	Channel        string `json:"channel,omitempty"`
	UserID         string `json:"user_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	Provider       string `json:"provider,omitempty"`
	Model          string `json:"model,omitempty"`
	Agent          string `json:"agent,omitempty"`
	// ToolCostUSD is what the tool calls of this run cost at the configured
	// per-call prices. It is kept apart from ModelCostUSD on purpose.
	ToolCostUSD float64 `json:"tool_cost_usd,omitempty"`
	// Trigger says what started the run: a user message ("user"), a cron job
	// ("cron"), or a subagent ("subagent"). Heartbeats are not recorded.
	Trigger string `json:"trigger,omitempty"`
	// ParentRunID and ParentSpanID link a nested run (a subagent) into the
	// trace that started it.
	ParentRunID    string  `json:"parent_run_id,omitempty"`
	ParentSpanID   string  `json:"parent_span_id,omitempty"`
	Status         Status  `json:"status"`
	Error          string  `json:"error,omitempty"`
	InputTokens    int     `json:"input_tokens,omitempty"`
	OutputTokens   int     `json:"output_tokens,omitempty"`
	CacheHitTokens int     `json:"cache_hit_tokens,omitempty"`
	ModelCostUSD   float64 `json:"model_cost_usd,omitempty"`
	TotalMs        int64   `json:"total_ms,omitempty"`
	FirstTokenMs   int64   `json:"first_token_ms,omitempty"`
}

// Span is one node of the trace tree.
type Span struct {
	ID             string   `json:"id"`
	RunID          string   `json:"run_id,omitempty"`
	ParentID       string   `json:"parent_id,omitempty"`
	Kind           SpanKind `json:"kind"`
	Name           string   `json:"name,omitempty"`
	StepIndex      int      `json:"step_index,omitempty"`
	RequestIndex   int      `json:"request_index,omitempty"`
	StartedAt      int64    `json:"started_at"`
	EndedAt        int64    `json:"ended_at,omitempty"`
	DurationMs     int64    `json:"duration_ms,omitempty"`
	FirstTokenMs   int64    `json:"first_token_ms,omitempty"`
	GenerationMs   int64    `json:"generation_ms,omitempty"`
	InputTokens    int      `json:"input_tokens,omitempty"`
	OutputTokens   int      `json:"output_tokens,omitempty"`
	CacheHitTokens int      `json:"cache_hit_tokens,omitempty"`
	CostUSD        float64  `json:"cost_usd,omitempty"`
	Status         Status   `json:"status,omitempty"`
	Error          string   `json:"error,omitempty"`
	InputJSON      string   `json:"input_json,omitempty"`
	OutputJSON     string   `json:"output_json,omitempty"`
}

// Throughput returns output tokens per second over the generation phase, or 0
// when the phase was not measured.
func (s Span) Throughput() float64 {
	if s.GenerationMs <= 0 || s.OutputTokens <= 0 {
		return 0
	}
	return float64(s.OutputTokens) / (float64(s.GenerationMs) / 1000)
}

// RunStart is the metadata the bot service already has when a run begins.
type RunStart struct {
	// RunID ties the trace to the engine run. Empty generates a new id.
	RunID          string
	Channel        string
	UserID         string
	ConversationID string
	Provider       string
	Model          string
	Agent          string
	UserMessage    string
	SystemPrompt   string
	// Trigger labels what started the run; empty is recorded as "user".
	Trigger string
	// ParentRunID and ParentSpanID nest this run under another trace, which is
	// how a subagent appears inside the run that invoked it.
	ParentRunID  string
	ParentSpanID string
}

// Config controls content capture.
type Config struct {
	// MaxContentChars caps every stored content blob. Zero uses the default.
	MaxContentChars int
	// StoreContent keeps prompts, thinking, responses, and tool arguments. When
	// false only metadata and token counts are recorded.
	StoreContent bool
	// RedactPatterns masks secrets in stored content. Nil uses the built-in
	// patterns; an explicit empty slice disables redaction.
	RedactPatterns []string
	// ToolPrices is USD per successful call, keyed by tool name. Tools cannot
	// report usage, so the call is the only unit that can be priced.
	ToolPrices map[string]float64
	// Now overrides the clock in tests.
	Now func() time.Time
}

const defaultMaxContentChars = 16384

// NewID returns a short random identifier for a run or span.
func NewID(prefix string) string {
	var raw [8]byte
	suffix := ""
	if _, err := rand.Read(raw[:]); err != nil {
		suffix = strconv.FormatInt(time.Now().UnixNano(), 16)
	} else {
		suffix = hex.EncodeToString(raw[:])
	}
	if prefix == "" {
		return suffix
	}
	return prefix + "_" + suffix
}

// TruncateContent caps s at max bytes without splitting a UTF-8 rune and marks
// how much was dropped.
func TruncateContent(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf("…[truncated %d chars]", len(s)-cut)
}
