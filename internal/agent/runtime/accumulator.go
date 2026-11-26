package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/channel"
	"myaaw/internal/provider"
)

const transcriptV2 = 2

type pendingTool struct {
	id        string
	name      string
	args      string
	stepGroup int
	thought   string
	startedAt time.Time
}

// turnAccumulator turns normalized external runtime events into Myaaw's exact
// AgentEvent sequence and Transcript v2 messages, preserving pre-tool text
// separately from the final assistant answer.
type turnAccumulator struct {
	mu sync.Mutex

	ctx       context.Context
	cfg       agent.RunConfig
	runID     string
	startedAt time.Time
	emitRaw   func(agent.AgentEvent) error

	transcript    []provider.Message
	newMessages   []provider.Message
	trace         []provider.ReactStep
	usage         provider.Usage
	attachments   []channel.Attachment
	extraMessages []provider.ExtraMessage

	stepIndex      int
	stepOpen       bool
	messageStarted bool
	curText        strings.Builder
	curThought     strings.Builder
	curToolCalls   []provider.ToolCall
	openTools      map[string]*pendingTool
	openToolOrder  []string
	completedTools map[string]bool
	toolSeq        int
	lastFinal      provider.Message
}

func newTurnAccumulator(
	ctx context.Context,
	cfg agent.RunConfig,
	history []provider.Message,
	emitRaw func(agent.AgentEvent) error,
) *turnAccumulator {
	if emitRaw == nil {
		emitRaw = func(agent.AgentEvent) error { return nil }
	}
	runID := cfg.RunID
	if runID == "" {
		runID = newRuntimeRunID()
	}
	acc := &turnAccumulator{
		ctx:            ctx,
		cfg:            cfg,
		runID:          runID,
		startedAt:      time.Now(),
		emitRaw:        emitRaw,
		transcript:     agent.MigrateLegacyTranscript(history),
		openTools:      make(map[string]*pendingTool),
		completedTools: make(map[string]bool),
	}
	acc.emitLocked(agent.AgentEvent{Type: agent.EventAgentStart})
	acc.checkpointLocked()
	acc.startStepLocked()
	return acc
}

func newRuntimeRunID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("run_%d", time.Now().UnixNano())
	}
	return "run_" + hex.EncodeToString(raw[:])
}

func (a *turnAccumulator) emitLocked(ev agent.AgentEvent) {
	ev.RunID = a.runID
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	if ev.StepIndex == 0 {
		ev.StepIndex = a.stepIndex
	}
	_ = a.emitRaw(ev)
}

func (a *turnAccumulator) checkpointLocked() {
	if a.cfg.OnCheckpoint == nil {
		return
	}
	snapshot := agent.CloneMessages(a.transcript)
	defer func() { _ = recover() }()
	a.cfg.OnCheckpoint(a.ctx, snapshot)
}

func (a *turnAccumulator) startStepLocked() {
	if a.stepOpen {
		return
	}
	a.stepIndex++
	a.stepOpen = true
	a.messageStarted = false
	a.curText.Reset()
	a.curThought.Reset()
	a.curToolCalls = nil
	a.emitLocked(agent.AgentEvent{Type: agent.EventStepStart})
}

func (a *turnAccumulator) ensureMessageStartedLocked() {
	if !a.stepOpen {
		a.startStepLocked()
	}
	if a.messageStarted {
		return
	}
	a.messageStarted = true
	partial := provider.Message{Role: "assistant", Version: transcriptV2}
	a.emitLocked(agent.AgentEvent{Type: agent.EventMessageStart, Message: &partial})
}

func (a *turnAccumulator) AddTextDelta(delta string) {
	if delta == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sealToolCallsBeforeNarrationLocked()
	a.ensureMessageStartedLocked()
	a.curText.WriteString(delta)
	a.emitLocked(agent.AgentEvent{Type: agent.EventMessageUpdate, Delta: delta})
}

func (a *turnAccumulator) AddThinkingDelta(delta string) {
	if delta == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sealToolCallsBeforeNarrationLocked()
	a.ensureMessageStartedLocked()
	a.curThought.WriteString(delta)
	a.emitLocked(agent.AgentEvent{Type: agent.EventMessageUpdate, ThinkingDelta: delta})
}

// sealToolCallsBeforeNarrationLocked persists the assistant message that holds
// already-issued tool calls (with its own pre-tool text/thought) as soon as the
// model narrates or thinks again. Anything streamed afterwards belongs to a new
// message, so ordering stays thinking -> text -> tool -> thinking -> text -> final
// even when the CLI never reports a terminal status for a tool.
func (a *turnAccumulator) sealToolCallsBeforeNarrationLocked() {
	if len(a.curToolCalls) == 0 {
		return
	}
	a.flushAssistantTurnLocked(provider.StopNormal, nil)
}

func (a *turnAccumulator) StartTool(id, name, argsJSON string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id != "" && a.completedTools[id] {
		return id
	}
	a.ensureMessageStartedLocked()
	if id == "" {
		a.toolSeq++
		id = fmt.Sprintf("ext_call_%d_%d", a.stepIndex, a.toolSeq)
	}
	if existing, ok := a.openTools[id]; ok {
		if name != "" {
			existing.name = name
		}
		if argsJSON != "" && argsJSON != "{}" {
			existing.args = argsJSON
		}
		return id
	}
	if name == "" {
		name = "tool"
	}
	if argsJSON == "" {
		argsJSON = "{}"
	}
	a.curToolCalls = append(a.curToolCalls, provider.ToolCall{
		ID:   id,
		Type: "function",
		Function: provider.FunctionCall{
			Name:      name,
			Arguments: argsJSON,
		},
	})
	a.openTools[id] = &pendingTool{
		id:        id,
		name:      name,
		args:      argsJSON,
		stepGroup: a.stepIndex,
		thought:   a.curThought.String(),
		startedAt: time.Now(),
	}
	a.openToolOrder = append(a.openToolOrder, id)
	a.emitLocked(agent.AgentEvent{
		Type:          agent.EventToolExecutionStart,
		ToolCallID:    id,
		ToolName:      name,
		ToolArguments: argsJSON,
	})
	return id
}

func (a *turnAccumulator) UpdateToolArgs(id, name, argsJSON string) {
	if id == "" || argsJSON == "" || argsJSON == "{}" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if p, ok := a.openTools[id]; ok {
		if name != "" {
			p.name = name
		}
		p.args = argsJSON
	}
	for i := range a.curToolCalls {
		if a.curToolCalls[i].ID == id {
			if name != "" {
				a.curToolCalls[i].Function.Name = name
			}
			a.curToolCalls[i].Function.Arguments = argsJSON
			return
		}
	}
}

func (a *turnAccumulator) UpdateTool(id, name, text string, isError bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == "" {
		return
	}
	if p, ok := a.openTools[id]; ok && name == "" {
		name = p.name
	}
	res := agent.ToolResult{
		Content: []provider.ContentItem{{Type: "text", Text: text}},
		IsError: isError,
	}
	a.emitLocked(agent.AgentEvent{
		Type:       agent.EventToolExecutionUpdate,
		ToolCallID: id,
		ToolName:   name,
		ToolResult: &res,
	})
}

func (a *turnAccumulator) EndTool(id, name, output string, isError bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.endToolLocked(id, name, output, isError, true)
}

func (a *turnAccumulator) endToolLocked(id, name, output string, isError bool, openNextStep bool) {
	if id != "" && a.completedTools[id] {
		// Ignore duplicate terminal updates (e.g. repeated ACP tool_call_update
		// completed frames) so completed tools are never re-created after the
		// assistant's final answer.
		return
	}

	pending := a.openTools[id]
	if pending == nil {
		// Some protocols report completed tool calls without an explicit start event.
		id = a.startToolUnlocked(id, name, "{}")
		pending = a.openTools[id]
	}
	delete(a.openTools, id)
	a.completedTools[id] = true

	// Flush the assistant message carrying this step's tool calls before writing
	// the tool result message, matching Transcript v2 ordering.
	if len(a.curToolCalls) > 0 {
		a.flushAssistantTurnLocked(provider.StopNormal, nil)
	}

	durationMs := time.Since(pending.startedAt).Milliseconds()
	res := agent.ToolResult{
		Content:    []provider.ContentItem{{Type: "text", Text: output}},
		IsError:    isError,
		DurationMs: durationMs,
	}
	a.emitLocked(agent.AgentEvent{
		Type:       agent.EventToolExecutionEnd,
		ToolCallID: pending.id,
		ToolName:   pending.name,
		ToolResult: &res,
	})

	a.trace = append(a.trace, provider.ReactStep{
		Thought:     pending.thought,
		Action:      pending.name,
		ActionInput: pending.args,
		Observation: output,
		StepGroup:   pending.stepGroup,
	})
	toolMsg := provider.Message{
		Role:       "tool",
		Name:       pending.name,
		Content:    output,
		ToolCallID: pending.id,
		IsError:    isError,
		DurationMs: durationMs,
		Version:    transcriptV2,
	}
	a.transcript = append(a.transcript, toolMsg)
	a.newMessages = append(a.newMessages, toolMsg)

	a.emitLocked(agent.AgentEvent{Type: agent.EventTraceUpdate, Trace: a.trace})
	a.checkpointLocked()

	// If every tool in this step completed and no narration is pending, close the
	// step and open a fresh one for the follow-up. Pending text/thought (streamed
	// after the tool-call message was sealed) must never be reset by a rollover.
	if len(a.openTools) == 0 && a.stepOpen && a.curText.Len() == 0 && a.curThought.Len() == 0 {
		a.openToolOrder = nil
		finalRef := a.lastFinal
		a.emitLocked(agent.AgentEvent{Type: agent.EventStepEnd, Message: &finalRef, Trace: a.trace})
		a.stepOpen = false
		if openNextStep {
			a.startStepLocked()
		}
	}
}

func (a *turnAccumulator) startToolUnlocked(id, name, argsJSON string) string {
	a.ensureMessageStartedLocked()
	if id == "" {
		a.toolSeq++
		id = fmt.Sprintf("ext_call_%d_%d", a.stepIndex, a.toolSeq)
	}
	if name == "" {
		name = "tool"
	}
	a.curToolCalls = append(a.curToolCalls, provider.ToolCall{
		ID:   id,
		Type: "function",
		Function: provider.FunctionCall{
			Name:      name,
			Arguments: argsJSON,
		},
	})
	a.openTools[id] = &pendingTool{
		id:        id,
		name:      name,
		args:      argsJSON,
		stepGroup: a.stepIndex,
		thought:   a.curThought.String(),
		startedAt: time.Now(),
	}
	a.openToolOrder = append(a.openToolOrder, id)
	a.emitLocked(agent.AgentEvent{
		Type:          agent.EventToolExecutionStart,
		ToolCallID:    id,
		ToolName:      name,
		ToolArguments: argsJSON,
	})
	return id
}

func (a *turnAccumulator) flushAssistantTurnLocked(stopReason provider.StopReason, runErr error) {
	msg := provider.Message{
		Role:       "assistant",
		Content:    a.curText.String(),
		Thought:    a.curThought.String(),
		ToolCalls:  append([]provider.ToolCall(nil), a.curToolCalls...),
		StopReason: stopReason,
		Version:    transcriptV2,
	}
	if runErr != nil && !agent.StoppedByClient(a.ctx) {
		msg.IsError = true
		msg.ErrorMessage = runErr.Error()
		if strings.TrimSpace(provider.MessageText(msg)) == "" {
			msg.Content = "⚠️ Agent error: " + runErr.Error()
		}
	}
	a.ensureMessageStartedLocked()
	a.emitLocked(agent.AgentEvent{Type: agent.EventMessageEnd, Message: &msg})
	a.transcript = append(a.transcript, msg)
	a.newMessages = append(a.newMessages, msg)
	a.lastFinal = msg

	a.curText.Reset()
	a.curThought.Reset()
	a.curToolCalls = nil
	a.messageStarted = false
}

func (a *turnAccumulator) AddUsage(usage provider.Usage) {
	if usage.TotalTokens == 0 && usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		return
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.usage.PromptTokens += usage.PromptTokens
	a.usage.CompletionTokens += usage.CompletionTokens
	a.usage.TotalTokens += usage.TotalTokens
	a.usage.ThoughtsTokens += usage.ThoughtsTokens
	a.usage.CachedInputTokens += usage.CachedInputTokens
	a.emitLocked(agent.AgentEvent{Type: agent.EventUsage, Usage: usage})
}

func (a *turnAccumulator) AddExtraMessages(items []provider.ExtraMessage) {
	if len(items) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cloned := provider.CloneExtraMessages(items)
	a.extraMessages = append(a.extraMessages, cloned...)
	a.emitLocked(agent.AgentEvent{Type: agent.EventExtraMessages, ExtraMessages: cloned})
}

func (a *turnAccumulator) AddAttachments(items []channel.Attachment) {
	if len(items) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.attachments = append(a.attachments, items...)
}

func (a *turnAccumulator) Finish(stopReason provider.StopReason, runErr error) (agent.RunResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if stopReason == "" {
		switch {
		case a.ctx.Err() != nil:
			stopReason = provider.StopAborted
		case runErr != nil:
			stopReason = provider.StopError
		default:
			stopReason = provider.StopNormal
		}
	}

	// Close any dangling open tools (e.g. CLIs that emitted tool_call without a
	// terminal tool_call_update) before flushing the final assistant answer so
	// tool calls never get merged onto the final answer message.
	if len(a.openTools) > 0 {
		remaining := append([]string(nil), a.openToolOrder...)
		for _, id := range remaining {
			if p, ok := a.openTools[id]; ok {
				a.endToolLocked(p.id, p.name, "", stopReason == provider.StopError, false)
			}
		}
		for _, p := range a.openTools {
			a.endToolLocked(p.id, p.name, "", stopReason == provider.StopError, false)
		}
	}

	// Flush any remaining assistant text/thought or terminal error message.
	hasPending := a.curText.Len() > 0 || a.curThought.Len() > 0 || len(a.curToolCalls) > 0 || (runErr != nil && !agent.StoppedByClient(a.ctx))
	if hasPending || a.lastFinal.Role == "" {
		a.flushAssistantTurnLocked(stopReason, runErr)
	} else {
		a.lastFinal.StopReason = stopReason
	}

	if a.stepOpen {
		finalRef := a.lastFinal
		a.emitLocked(agent.AgentEvent{Type: agent.EventStepEnd, Message: &finalRef, Trace: a.trace})
		a.stepOpen = false
	}

	a.checkpointLocked()

	final := a.lastFinal
	if final.Role == "" {
		final = provider.Message{Role: "assistant", Version: transcriptV2}
	}
	final.Trace = a.trace
	final.Usage = a.usage
	final.StopReason = stopReason
	final.Version = transcriptV2

	for i := len(a.transcript) - 1; i >= 0; i-- {
		if a.transcript[i].Role == "assistant" {
			a.transcript[i].Trace = a.trace
			a.transcript[i].Usage = a.usage
			break
		}
	}
	for i := len(a.newMessages) - 1; i >= 0; i-- {
		if a.newMessages[i].Role == "assistant" {
			a.newMessages[i].Trace = a.trace
			a.newMessages[i].Usage = a.usage
			break
		}
	}

	a.emitLocked(agent.AgentEvent{
		Type:       agent.EventAgentEnd,
		Message:    &final,
		Trace:      a.trace,
		Usage:      a.usage,
		StopReason: stopReason,
		Err:        runErr,
	})

	return agent.RunResult{
		RunID:         a.runID,
		StartedAt:     a.startedAt,
		EndedAt:       time.Now(),
		Transcript:    a.transcript,
		NewMessages:   a.newMessages,
		Final:         final,
		Usage:         a.usage,
		StopReason:    stopReason,
		Err:           runErr,
		Attachments:   a.attachments,
		ExtraMessages: a.extraMessages,
	}, runErr
}
