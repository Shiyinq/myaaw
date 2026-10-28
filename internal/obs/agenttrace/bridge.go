// Package agenttrace connects the agent loop to the observability recorder.
//
// The recorder itself only understands obs.Event, which keeps the agent loop
// free of any observability dependency. This package owns the single
// translation in the other direction, so every caller (chat channels, the CLI,
// and nested subagent runs) records the same way.
package agenttrace

import (
	"context"

	"myaaw/internal/agent"
	"myaaw/internal/obs"
	"myaaw/internal/provider"
)

// Trace records one agent run. A nil *Trace is valid and ignores every call, so
// callers never need to branch on "observability disabled".
type Trace struct {
	recorder *obs.Recorder
	store    *obs.Store
	pricer   *obs.Pricer
	config   obs.Config
	exporter *obs.Exporter
}

// Start opens a run and returns its trace. A nil store disables recording.
func Start(store *obs.Store, pricer *obs.Pricer, config obs.Config, start obs.RunStart) *Trace {
	if store == nil {
		return nil
	}
	recorder := obs.New(store, pricer, config)
	recorder.Start(start)
	return &Trace{recorder: recorder, store: store, pricer: pricer, config: config}
}

// StartChild opens a nested run under the current trace: a subagent becomes a
// run span inside the tool call that invoked it.
func (t *Trace) StartChild(start obs.RunStart) *Trace {
	if t == nil || t.store == nil {
		return nil
	}
	if start.ParentRunID == "" {
		start.ParentRunID = t.RunID()
	}
	child := Start(t.store, t.pricer, t.config, start)
	child.SetExporter(t.exporter)
	return child
}

// SetExporter forwards finished runs to OTLP when it is configured.
func (t *Trace) SetExporter(exporter *obs.Exporter) {
	if t == nil {
		return
	}
	t.exporter = exporter
	t.recorder.SetExporter(exporter)
}

// RunID returns the run identifier, or "".
func (t *Trace) RunID() string {
	if t == nil {
		return ""
	}
	return t.recorder.RunID()
}

// ToolSpanID returns the span recorded for a tool call.
func (t *Trace) ToolSpanID(callID string) string {
	if t == nil {
		return ""
	}
	return t.recorder.ToolSpanID(callID)
}

// CurrentSpanID returns the span nested work should attach to.
func (t *Trace) CurrentSpanID() string {
	if t == nil {
		return ""
	}
	return t.recorder.CurrentSpanID()
}

// Event records one engine event.
func (t *Trace) Event(event agent.AgentEvent) {
	if t == nil {
		return
	}
	if trace, ok := FromEvent(event); ok {
		t.recorder.OnEvent(trace)
	}
}

// End records the run result.
func (t *Trace) End(result agent.RunResult) {
	if t == nil {
		return
	}
	t.recorder.OnEvent(EndEvent(result))
}

// Close stops the writer and flushes pending spans. The store is shared and
// stays open.
func (t *Trace) Close() error {
	if t == nil {
		return nil
	}
	return t.recorder.Close()
}

// Flush waits until everything queued is written.
func (t *Trace) Flush(ctx context.Context) error {
	if t == nil {
		return nil
	}
	return t.recorder.Flush(ctx)
}

// FromEvent translates one engine event. The second result is false for events
// the trace does not use.
func FromEvent(event agent.AgentEvent) (obs.Event, bool) {
	switch event.Type {
	case agent.EventStepStart:
		return obs.Event{Type: obs.EventStepStart, Time: event.Time, StepIndex: event.StepIndex}, true

	case agent.EventLLMRequestStart:
		return obs.Event{
			Type:      obs.EventLLMRequestStart,
			Time:      event.Time,
			StepIndex: event.StepIndex,
			Model:     event.Model,
			Messages:  event.Messages,
		}, true

	case agent.EventLLMRetry:
		errorText := ""
		if event.Err != nil {
			errorText = event.Err.Error()
		}
		return obs.Event{
			Type:      obs.EventLLMRetry,
			Time:      event.Time,
			StepIndex: event.StepIndex,
			Attempt:   event.Attempt,
			ErrorText: errorText,
		}, true

	case agent.EventContextCompacted:
		return obs.Event{
			Type:            obs.EventCompaction,
			Time:            event.Time,
			StepIndex:       event.StepIndex,
			TokensBefore:    event.Compact.TokensBefore,
			TokensAfter:     event.Compact.TokensAfter,
			Budget:          event.Compact.Budget,
			DroppedMessages: event.Compact.DroppedMessages,
			DroppedTokens:   event.Compact.DroppedTokens,
		}, true

	case agent.EventMessageUpdate:
		if event.Delta == "" && event.ThinkingDelta == "" {
			return obs.Event{}, false
		}
		return obs.Event{Type: obs.EventFirstToken, Time: event.Time, StepIndex: event.StepIndex}, true

	case agent.EventMessageEnd:
		if event.Message == nil {
			return obs.Event{}, false
		}
		return obs.Event{
			Type:       obs.EventLLMEnd,
			Time:       event.Time,
			StepIndex:  event.StepIndex,
			Usage:      event.Message.Usage,
			StopReason: event.Message.StopReason,
			IsError:    event.Message.IsError,
			ErrorText:  event.Message.ErrorMessage,
			Output:     provider.MessageText(*event.Message),
			Thought:    event.Message.Thought,
		}, true

	case agent.EventToolExecutionStart:
		return obs.Event{
			Type:          obs.EventToolStart,
			Time:          event.Time,
			StepIndex:     event.StepIndex,
			ToolCallID:    event.ToolCallID,
			ToolName:      event.ToolName,
			ToolArguments: event.ToolArguments,
		}, true

	case agent.EventToolExecutionEnd:
		if event.ToolResult == nil {
			return obs.Event{}, false
		}
		trace := obs.Event{
			Type:           obs.EventToolEnd,
			Time:           event.Time,
			StepIndex:      event.StepIndex,
			ToolCallID:     event.ToolCallID,
			ToolName:       event.ToolName,
			ToolOutput:     agent.ToolResultText(*event.ToolResult),
			ToolDurationMs: event.ToolResult.DurationMs,
			IsError:        event.ToolResult.IsError,
		}
		if event.ToolResult.IsError {
			trace.ErrorText = "tool returned an error"
		}
		return trace, true

	case agent.EventStepEnd:
		return obs.Event{Type: obs.EventStepEnd, Time: event.Time, StepIndex: event.StepIndex}, true
	}
	return obs.Event{}, false
}

// EndEvent builds the terminal event from a run result.
func EndEvent(result agent.RunResult) obs.Event {
	event := obs.Event{
		Type:       obs.EventRunEnd,
		Time:       result.EndedAt,
		Usage:      result.Usage,
		StopReason: result.StopReason,
		Status:     obs.StatusFromStopReason(result.StopReason, result.Err),
	}
	if result.Err != nil {
		event.Err = result.Err.Error()
	}
	return event
}

// SystemPrompt returns the first system message's text, or "".
func SystemPrompt(messages []provider.Message) string {
	for _, message := range messages {
		if message.Role == "system" {
			return provider.MessageText(message)
		}
	}
	return ""
}

type contextKey struct{}

// WithTrace attaches a trace so nested work (a subagent tool call) can record
// itself under the run that started it. A nil trace leaves ctx unchanged.
func WithTrace(ctx context.Context, trace *Trace) context.Context {
	if trace == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, trace)
}

// FromContext returns the trace attached to ctx, or nil.
func FromContext(ctx context.Context) *Trace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(contextKey{}).(*Trace)
	return trace
}
