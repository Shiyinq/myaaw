package agent

import (
	"encoding/json"
	"fmt"
	"time"

	"myaaw/internal/provider"
)

const transcriptVersion = 2

// MigrateLegacyTranscript upgrades transcript entries written by the old
// trace-based ReAct loop. Legacy assistant messages carry a Trace but no
// adjacent tool messages, so they are expanded exactly once into
// assistant(tool_calls) + tool(result) + assistant(final) sequences.
//
// Entries already marked with Version >= 2 are returned unchanged.
func MigrateLegacyTranscript(messages []provider.Message) []provider.Message {
	needsExpansion := false
	for _, msg := range messages {
		if msg.Version < transcriptVersion && msg.Role == "assistant" && len(msg.Trace) > 0 {
			needsExpansion = true
			break
		}
	}

	out := make([]provider.Message, 0, len(messages))
	if !needsExpansion {
		for _, msg := range messages {
			if msg.Version < transcriptVersion {
				msg.Version = transcriptVersion
			}
			out = append(out, msg)
		}
		return out
	}

	toolCallCounter := 0
	tracePrefix := time.Now().UnixNano()
	for _, msg := range messages {
		if msg.Version >= transcriptVersion || msg.Role != "assistant" || len(msg.Trace) == 0 {
			if msg.Version < transcriptVersion {
				msg.Version = transcriptVersion
			}
			out = append(out, msg)
			continue
		}

		for _, group := range groupTraceSteps(msg.Trace) {
			if len(group) == 0 {
				continue
			}

			ids := make([]string, len(group))
			toolCalls := make([]provider.ToolCall, 0, len(group))
			for i, step := range group {
				id := fmt.Sprintf("trace-%d-%d", tracePrefix, toolCallCounter)
				toolCallCounter++
				ids[i] = id

				var args interface{}
				if step.ActionInput != "" {
					args = step.ActionInput
				}
				signature := ""
				if i == 0 {
					signature = step.ThoughtSignature
				}
				toolCalls = append(toolCalls, provider.ToolCall{
					ID:   id,
					Type: "function",
					Function: provider.FunctionCall{
						Name:             step.Action,
						Arguments:        args,
						ThoughtSignature: signature,
					},
				})
			}

			out = append(out, provider.Message{
				Role:      "assistant",
				Thought:   group[0].Thought,
				ToolCalls: toolCalls,
				Version:   transcriptVersion,
			})

			for i, step := range group {
				out = append(out, provider.Message{
					Role:       "tool",
					Name:       step.Action,
					Content:    step.Observation,
					ToolCallID: ids[i],
					IsError:    looksLikeToolError(step.Observation),
					Version:    transcriptVersion,
				})
			}
		}

		// Keep the original final assistant answer (with its Trace for UI).
		msg.Version = transcriptVersion
		out = append(out, msg)
	}

	return out
}

// groupTraceSteps groups trace steps by StepGroup. Steps with the same
// non-zero StepGroup were produced by parallel tool calls in one turn. Steps
// with StepGroup == 0 (old data) are treated as sequential.
func groupTraceSteps(steps []provider.ReactStep) [][]provider.ReactStep {
	if len(steps) == 0 {
		return nil
	}

	var groups [][]provider.ReactStep
	var current []provider.ReactStep
	currentGroup := -1

	for _, step := range steps {
		if step.StepGroup == 0 {
			if len(current) > 0 {
				groups = append(groups, current)
				current = nil
				currentGroup = -1
			}
			groups = append(groups, []provider.ReactStep{step})
			continue
		}
		if step.StepGroup != currentGroup {
			if len(current) > 0 {
				groups = append(groups, current)
			}
			current = []provider.ReactStep{step}
			currentGroup = step.StepGroup
			continue
		}
		current = append(current, step)
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

// EstimateTokens returns a rough token estimate for a transcript. It is
// deliberately provider-agnostic: roughly four bytes per token plus a small
// per-message overhead.
func EstimateTokens(messages []provider.Message) int {
	total := 0
	for _, msg := range messages {
		total += 4
		total += estimateText(provider.MessageText(msg))
		total += estimateText(msg.Thought)
		total += estimateText(msg.Name)
		total += len(msg.Images) * 1000
		for _, item := range provider.MessageContentItems(msg) {
			if item.ImageURL != nil {
				total += 1000
			}
		}
		for _, tc := range msg.ToolCalls {
			total += estimateText(tc.Function.Name)
			total += estimateText(argsToString(tc.Function.Arguments))
		}
	}
	return total
}

func estimateText(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}

// CompactStats describes what CompactTranscript removed from a transcript.
type CompactStats struct {
	Changed         bool
	TokensBefore    int
	TokensAfter     int
	Budget          int
	DroppedMessages int
	DroppedTokens   int
}

// CompactTranscript trims old turns until the transcript fits the token
// budget. The system prefix and the most recent turn (which contains the
// newest user prompt) are always kept. Turns are never split, so assistant
// tool-call messages and their tool results stay together.
func CompactTranscript(messages []provider.Message, maxTokens int) []provider.Message {
	compacted, _ := CompactTranscriptWithStats(messages, maxTokens)
	return compacted
}

// CompactTranscriptWithStats is CompactTranscript plus the numbers a trace
// needs to show that the transcript was silently shortened.
func CompactTranscriptWithStats(messages []provider.Message, maxTokens int) ([]provider.Message, CompactStats) {
	before := EstimateTokens(messages)
	stats := CompactStats{TokensBefore: before, TokensAfter: before, Budget: maxTokens}
	if maxTokens <= 0 || before <= maxTokens {
		return messages, stats
	}

	compacted := compactMessages(messages, maxTokens)
	if len(compacted) >= len(messages) {
		return messages, stats
	}

	stats.TokensAfter = EstimateTokens(compacted)
	stats.DroppedMessages = len(messages) - len(compacted)
	stats.DroppedTokens = before - stats.TokensAfter
	if stats.DroppedTokens < 0 {
		stats.DroppedTokens = 0
	}
	stats.Changed = stats.DroppedMessages > 0 || stats.DroppedTokens > 0
	return compacted, stats
}

func compactMessages(messages []provider.Message, maxTokens int) []provider.Message {
	prefixEnd := 0
	for prefixEnd < len(messages) && messages[prefixEnd].Role == "system" {
		prefixEnd++
	}
	prefix := messages[:prefixEnd]
	rest := messages[prefixEnd:]
	if len(rest) == 0 {
		return messages
	}

	var groups [][]provider.Message
	var current []provider.Message
	for _, msg := range rest {
		if msg.Role == "user" && len(current) > 0 {
			groups = append(groups, current)
			current = nil
		}
		current = append(current, msg)
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	if len(groups) == 0 {
		return messages
	}

	kept := [][]provider.Message{groups[len(groups)-1]}
	budget := EstimateTokens(prefix) + EstimateTokens(groups[len(groups)-1])
	for i := len(groups) - 2; i >= 0; i-- {
		cost := EstimateTokens(groups[i])
		if budget+cost > maxTokens {
			break
		}
		kept = append([][]provider.Message{groups[i]}, kept...)
		budget += cost
	}

	result := make([]provider.Message, 0, len(prefix)+len(rest))
	result = append(result, prefix...)
	for _, group := range kept {
		result = append(result, group...)
	}
	return result
}

// toolCallArgumentsJSON normalizes tool arguments for transport.
func toolCallArgumentsJSON(v interface{}) string {
	if v == nil {
		return "{}"
	}
	if s, ok := v.(string); ok {
		if s == "" {
			return "{}"
		}
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
