package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"myaaw/internal/agent/tools"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

// MaxIterationsMessage is returned when the loop hits its safety cap.
const MaxIterationsMessage = "⚠️ Maximum iterations reached. Task may be incomplete."

var (
	userIDPattern  = regexp.MustCompile(`User ID:\s*(\d+)`)
	channelPattern = regexp.MustCompile(`Channel Platform:\s*([\w-]+)`)
)

// RunEvents runs the agent loop and emits lifecycle events.
//
// Provider failures are recorded in the returned RunResult (StopReason
// StopError) and also returned as an error. The RunResult remains valid, so
// callers should persist the partial transcript and surface the final message.
// Context cancellation behaves the same way (StopAborted).
func (a *Agent) RunEvents(
	ctx context.Context,
	cfg RunConfig,
	history []provider.Message,
	emitRaw func(AgentEvent) error,
) (RunResult, error) {
	if emitRaw == nil {
		emitRaw = func(AgentEvent) error { return nil }
	}
	cfg = a.applyDefaults(cfg)
	if cfg.RunTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.RunTimeout)
		defer cancel()
	}

	runID := cfg.RunID
	if runID == "" {
		runID = newRunID()
	}
	startedAt := time.Now()
	var stepIndex int
	// Every event carries the run identity, its timestamp, and the step it
	// belongs to, so consumers never have to reconstruct that context.
	emit := func(event AgentEvent) error {
		event.RunID = runID
		if event.Time.IsZero() {
			event.Time = time.Now()
		}
		if event.StepIndex == 0 {
			event.StepIndex = stepIndex
		}
		return emitRaw(event)
	}

	transcript := MigrateLegacyTranscript(history)
	newMessages := make([]provider.Message, 0, 4)
	var trace []provider.ReactStep
	var usage provider.Usage

	if cfg.ToolsContext == nil {
		extracted := extractToolsContext(transcript)
		cfg.ToolsContext = &extracted
	}

	toolsList := buildTools(cfg.ExcludedTools, cfg.ToolsContext)
	registry := toolMap(toolsList)
	schemas := toolSchemas(toolsList)

	_ = emit(AgentEvent{Type: EventAgentStart})

	checkpoint := func() {
		if cfg.OnCheckpoint == nil {
			return
		}
		snapshot := make([]provider.Message, len(transcript))
		copy(snapshot, transcript)
		safeCheckpoint(ctx, cfg.OnCheckpoint, snapshot)
	}

	// Persist the user prompt before the first provider call.
	checkpoint()

	var (
		final      provider.Message
		stopReason = provider.StopNormal
		runErr     error
		iteration  int
		runSeq     = time.Now().UnixNano()
		repeatSeen = map[string]int{}
		nextWarn   string
	)

	for {
		if err := ctx.Err(); err != nil {
			text := "⚠️ Request aborted."
			if errors.Is(err, context.DeadlineExceeded) {
				text = fmt.Sprintf("⚠️ Agent run timed out after %s. Task may be incomplete.", cfg.RunTimeout)
			}
			final = terminalMessage(text, provider.StopAborted, err)
			transcript = append(transcript, final)
			newMessages = append(newMessages, final)
			emitAssistantMessage(emit, final)
			stopReason = provider.StopAborted
			runErr = err
			break
		}

		if cfg.MaxIterations > 0 && iteration >= cfg.MaxIterations {
			final = terminalMessage(MaxIterationsMessage, provider.StopMaxIterations, nil)
			transcript = append(transcript, final)
			newMessages = append(newMessages, final)
			emitAssistantMessage(emit, final)
			stopReason = provider.StopMaxIterations
			break
		}

		requestIndex := iteration
		iteration++
		stepIndex = iteration

		_ = emit(AgentEvent{Type: EventStepStart})

		requestMessages := transcript
		warning := stepAwarenessWarning(requestIndex, cfg)
		if nextWarn != "" {
			if warning != "" {
				warning += "\n\n" + nextWarn
			} else {
				warning = nextWarn
			}
			nextWarn = ""
		}
		requestMessages = injectSystemWarning(requestMessages, warning)
		if cfg.TransformContext != nil {
			requestMessages = cfg.TransformContext(ctx, requestMessages)
		}
		if cfg.ContextMaxTokens > 0 {
			var compactStats CompactStats
			requestMessages, compactStats = CompactTranscriptWithStats(requestMessages, cfg.ContextMaxTokens)
			if compactStats.Changed {
				_ = emit(AgentEvent{Type: EventContextCompacted, Compact: compactStats})
			}
		}

		// Hand the exact request transcript to observers before it is sent.
		_ = emit(AgentEvent{
			Type:     EventLLMRequestStart,
			Model:    cfg.Model,
			Messages: append([]provider.Message(nil), requestMessages...),
		})

		var (
			streamedText    strings.Builder
			streamedThought strings.Builder
			turnUsage       provider.Usage
			finalFromStream *provider.Message
			streamStop      provider.StopReason
			streamErr       error
			messageStarted  bool
			sawTextDelta    bool
			sawThoughtDelta bool
		)

		startMessage := func() {
			if messageStarted {
				return
			}
			messageStarted = true
			partial := provider.Message{Role: "assistant", Version: transcriptVersion}
			_ = emit(AgentEvent{Type: EventMessageStart, Message: &partial})
		}

		providerErr := a.Provider.ChatStreamEvents(ctx, cfg.Model, requestMessages, schemas, func(event provider.StreamEvent) error {
			switch event.Type {
			case provider.StreamTextDelta:
				if event.Text == "" {
					return nil
				}
				sawTextDelta = true
				startMessage()
				streamedText.WriteString(event.Text)
				return emit(AgentEvent{Type: EventMessageUpdate, Delta: event.Text})
			case provider.StreamThinkingDelta:
				if event.Thought == "" {
					return nil
				}
				sawThoughtDelta = true
				startMessage()
				streamedThought.WriteString(event.Thought)
				return emit(AgentEvent{Type: EventMessageUpdate, ThinkingDelta: event.Thought})
			case provider.StreamAttempt:
				return emit(AgentEvent{
					Type:    EventLLMRetry,
					Attempt: event.Attempt,
					Err:     event.Err,
				})
			case provider.StreamUsage:
				turnUsage = event.Usage
				return emit(AgentEvent{Type: EventUsage, Usage: event.Usage})
			case provider.StreamDone:
				if event.Message != nil {
					message := *event.Message
					finalFromStream = &message
				}
				streamStop = event.StopReason
				streamErr = event.Err
			}
			return nil
		})

		if providerErr != nil {
			switch {
			case ctx.Err() != nil:
				streamStop = provider.StopAborted
				streamErr = ctx.Err()
			case streamErr == nil:
				streamStop = provider.StopError
				streamErr = providerErr
			}
		}
		if ctx.Err() != nil {
			streamStop = provider.StopAborted
			streamErr = ctx.Err()
		}

		if finalFromStream != nil {
			final = *finalFromStream
		} else {
			final = provider.Message{Role: "assistant"}
		}
		final.Role = "assistant"
		final.Version = transcriptVersion
		if provider.MessageText(final) == "" && streamedText.Len() > 0 {
			final.Content = streamedText.String()
		}
		if final.Thought == "" && streamedThought.Len() > 0 {
			final.Thought = streamedThought.String()
		}
		if streamStop == "" {
			streamStop = final.StopReason
		}
		if streamStop == "" {
			streamStop = provider.StopNormal
		}
		final.StopReason = streamStop

		turnTokens := final.Usage
		if turnTokens.TotalTokens == 0 {
			turnTokens = turnUsage
		}
		addUsage(&usage, turnTokens)

		if streamErr != nil {
			final.IsError = true
			final.ErrorMessage = streamErr.Error()
			if provider.MessageText(final) == "" {
				final.Content = "⚠️ Agent error: " + streamErr.Error()
			}
		}

		// Normalize tool call IDs before the message enters the transcript.
		for i := range final.ToolCalls {
			if final.ToolCalls[i].ID == "" {
				final.ToolCalls[i].ID = fmt.Sprintf("call_%d_%d_%d", runSeq, iteration, i)
			}
		}

		// Some providers only expose the full thought/text on StreamDone.
		// Emit it as a delta so every consumer (including the TUI timeline)
		// sees the same content as providers that stream token deltas.
		if final.Thought != "" && !sawThoughtDelta {
			startMessage()
			_ = emit(AgentEvent{Type: EventMessageUpdate, ThinkingDelta: final.Thought})
		}
		if text := provider.MessageText(final); text != "" && !sawTextDelta {
			startMessage()
			_ = emit(AgentEvent{Type: EventMessageUpdate, Delta: text})
		}

		startMessage()
		_ = emit(AgentEvent{Type: EventMessageEnd, Message: &final})
		transcript = append(transcript, final)
		newMessages = append(newMessages, final)

		if streamStop == provider.StopError || streamStop == provider.StopAborted {
			stopReason = streamStop
			runErr = streamErr
			_ = emit(AgentEvent{Type: EventStepEnd, Message: &final})
			break
		}

		if streamStop == provider.StopLength {
			stopReason = provider.StopLength
			if len(final.ToolCalls) == 0 {
				_ = emit(AgentEvent{Type: EventStepEnd, Message: &final})
				break
			}
			// The output was truncated, so tool arguments may be incomplete.
			// Fail every call and let the model re-issue them.
			iterationTrace, toolMessages := failTruncatedCalls(final, iteration, cfg, emit)
			trace = append(trace, iterationTrace...)
			for _, message := range toolMessages {
				transcript = append(transcript, message)
				newMessages = append(newMessages, message)
			}
			_ = emit(AgentEvent{Type: EventTraceUpdate, Trace: trace})
			_ = emit(AgentEvent{Type: EventStepEnd, Message: &final, Trace: trace})
			continue
		}

		if len(final.ToolCalls) == 0 {
			stopReason = provider.StopNormal
			_ = emit(AgentEvent{Type: EventStepEnd, Message: &final, Trace: trace})
			break
		}

		calls := make([]ToolCallInput, 0, len(final.ToolCalls))
		for _, toolCall := range final.ToolCalls {
			calls = append(calls, ToolCallInput{
				ID:        toolCall.ID,
				Name:      toolCall.Function.Name,
				Arguments: json.RawMessage(toolCallArgumentsJSON(toolCall.Function.Arguments)),
			})
		}

		log.Printf("[Agent] step %d: executing %d tool call(s)", iteration, len(calls))
		results, terminate := executeToolCalls(ctx, calls, registry, cfg, emit)

		toolMessages := make([]provider.Message, 0, len(results))
		for i, result := range results {
			toolCall := final.ToolCalls[i]
			observation := ToolResultText(result)
			trace = append(trace, provider.ReactStep{
				Thought:          final.Thought,
				Action:           toolCall.Function.Name,
				ActionInput:      argsToString(toolCall.Function.Arguments),
				Observation:      observation,
				ThoughtSignature: toolCall.Function.ThoughtSignature,
				StepGroup:        iteration,
			})
			message := provider.Message{
				Role:       "tool",
				Name:       toolCall.Function.Name,
				Content:    observation,
				ToolCallID: toolCall.ID,
				IsError:    result.IsError,
				DurationMs: result.DurationMs,
				Version:    transcriptVersion,
			}
			transcript = append(transcript, message)
			newMessages = append(newMessages, message)
			toolMessages = append(toolMessages, message)

			key := toolCall.Function.Name + "\x00" + argsToString(toolCall.Function.Arguments)
			repeatSeen[key]++
			if cfg.RepeatToolCallLimit > 0 && repeatSeen[key] == cfg.RepeatToolCallLimit {
				nextWarn = fmt.Sprintf(
					"⚠️ You have called %s with identical arguments %d times. Do not repeat the same call; use the existing result or choose a different approach.",
					toolCall.Function.Name,
					repeatSeen[key],
				)
			}
		}

		_ = emit(AgentEvent{Type: EventTraceUpdate, Trace: trace})

		// Tool results belong to the step even if the run dies mid-stream.
		checkpoint()

		decision := StepContinue
		if cfg.FinishStep != nil {
			decision = cfg.FinishStep(ctx, StepContext{
				Message:     final,
				ToolResults: toolMessages,
				Transcript:  transcript,
				NewMessages: newMessages,
			})
		}
		_ = emit(AgentEvent{Type: EventStepEnd, Message: &final, ToolResult: nil, Trace: trace})
		checkpoint()

		if ctx.Err() != nil {
			stopReason = provider.StopAborted
			runErr = ctx.Err()
			break
		}
		if terminate || decision == StepEnd {
			stopReason = provider.StopNormal
			break
		}
	}

	// Capture terminal messages (abort, max iterations, provider error).
	checkpoint()

	if final.Role == "" {
		final = provider.Message{Role: "assistant", Version: transcriptVersion}
	}
	final.Trace = trace
	final.Usage = usage
	attachTrace(transcript, trace)
	attachTrace(newMessages, trace)
	attachUsage(transcript, usage)
	attachUsage(newMessages, usage)
	final.Version = transcriptVersion

	_ = emit(AgentEvent{
		Type:       EventAgentEnd,
		Message:    &final,
		Trace:      trace,
		Usage:      usage,
		StopReason: stopReason,
		Err:        runErr,
	})

	return RunResult{
		RunID:       runID,
		StartedAt:   startedAt,
		EndedAt:     time.Now(),
		Transcript:  transcript,
		NewMessages: newMessages,
		Final:       final,
		Usage:       usage,
		StopReason:  stopReason,
		Err:         runErr,
	}, runErr
}

func emitAssistantMessage(emit func(AgentEvent) error, message provider.Message) {
	partial := message
	_ = emit(AgentEvent{Type: EventMessageStart, Message: &partial})
	_ = emit(AgentEvent{Type: EventMessageEnd, Message: &partial})
	_ = emit(AgentEvent{Type: EventStepEnd, Message: &partial})
}

// safeCheckpoint isolates persistence hooks from the agent loop.
func safeCheckpoint(ctx context.Context, hook func(context.Context, []provider.Message), transcript []provider.Message) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Agent] checkpoint failed: %v", r)
		}
	}()
	hook(ctx, transcript)
}

func terminalMessage(text string, reason provider.StopReason, err error) provider.Message {
	message := provider.Message{
		Role:       "assistant",
		Content:    text,
		StopReason: reason,
		Version:    transcriptVersion,
	}
	if err != nil {
		message.IsError = reason == provider.StopError
		message.ErrorMessage = err.Error()
	}
	return message
}

func failTruncatedCalls(
	final provider.Message,
	stepGroup int,
	cfg RunConfig,
	emit func(AgentEvent) error,
) ([]provider.ReactStep, []provider.Message) {
	trace := make([]provider.ReactStep, 0, len(final.ToolCalls))
	messages := make([]provider.Message, 0, len(final.ToolCalls))
	for _, toolCall := range final.ToolCalls {
		reason := fmt.Sprintf("Tool call %q was not executed: the response hit the output token limit, so its arguments may be truncated. Re-issue the call with complete arguments.", toolCall.Function.Name)
		result := errorToolResult(reason)
		_ = emit(AgentEvent{
			Type:          EventToolExecutionStart,
			ToolCallID:    toolCall.ID,
			ToolName:      toolCall.Function.Name,
			ToolArguments: argsToString(toolCall.Function.Arguments),
		})
		_ = emit(AgentEvent{Type: EventToolExecutionEnd, ToolCallID: toolCall.ID, ToolName: toolCall.Function.Name, ToolResult: &result})
		trace = append(trace, provider.ReactStep{
			Thought:     final.Thought,
			Action:      toolCall.Function.Name,
			ActionInput: argsToString(toolCall.Function.Arguments),
			Observation: reason,
			StepGroup:   stepGroup,
		})
		messages = append(messages, provider.Message{
			Role:       "tool",
			Name:       toolCall.Function.Name,
			Content:    reason,
			ToolCallID: toolCall.ID,
			IsError:    true,
			Version:    transcriptVersion,
		})
	}
	return trace, messages
}

func stepAwarenessWarning(iteration int, cfg RunConfig) string {
	if cfg.MaxIterations <= 0 || iteration < cfg.WarningThreshold {
		return ""
	}
	remaining := cfg.MaxIterations - iteration
	if remaining < 0 {
		remaining = 0
	}
	log.Printf("[Agent] Warning: only %d steps remaining", remaining)
	return fmt.Sprintf("\n\n⚠️ URGENT: You have only %d steps remaining! Provide a final answer now. Do not use more tools unless absolutely critical.", remaining)
}

func injectSystemWarning(messages []provider.Message, warning string) []provider.Message {
	if warning == "" {
		return messages
	}
	result := make([]provider.Message, len(messages))
	copy(result, messages)
	if len(result) > 0 && result[0].Role == "system" {
		if text, ok := result[0].Content.(string); ok {
			result[0].Content = text + warning
			return result
		}
	}
	// No usable system message: prepend one.
	system := provider.Message{Role: "system", Content: strings.TrimSpace(warning), Version: transcriptVersion}
	return append([]provider.Message{system}, result...)
}

func attachUsage(messages []provider.Message, usage provider.Usage) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			messages[i].Usage = usage
			return
		}
	}
}

func attachTrace(messages []provider.Message, trace []provider.ReactStep) {
	if len(trace) == 0 {
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			messages[i].Trace = trace
			messages[i].Version = transcriptVersion
			return
		}
	}
}

func addUsage(dst *provider.Usage, src provider.Usage) {
	dst.PromptTokens += src.PromptTokens
	dst.CompletionTokens += src.CompletionTokens
	dst.TotalTokens += src.TotalTokens
	dst.ThoughtsTokens += src.ThoughtsTokens
	dst.CachedInputTokens += src.CachedInputTokens
	if len(src.Details) > 0 {
		dst.Details = append(dst.Details, src.Details...)
	}
}

func extractToolsContext(messages []provider.Message) tools.ToolsContext {
	baseURL := config.MYAAWBaseURL
	if baseURL == "" {
		port := strings.TrimPrefix(config.PORT, ":")
		if port == "" {
			port = "8080"
		}
		baseURL = "http://localhost:" + port
	}

	ctx := tools.ToolsContext{BaseURL: baseURL}
	for _, message := range messages {
		if message.Role != "system" {
			continue
		}
		content, ok := message.Content.(string)
		if !ok {
			continue
		}
		if match := userIDPattern.FindStringSubmatch(content); len(match) > 1 {
			ctx.UserID = match[1]
		}
		if match := channelPattern.FindStringSubmatch(content); len(match) > 1 {
			ctx.Channel = match[1]
		}
		break
	}
	return ctx
}

func argsToString(value interface{}) string {
	if text, ok := value.(string); ok {
		return text
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}
