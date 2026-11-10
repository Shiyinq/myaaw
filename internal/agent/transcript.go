package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
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
	messages = CloneMessages(messages)
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
		total += len(msg.Images) * imageTokenCost
		for _, item := range provider.MessageContentItems(msg) {
			if item.ImageURL != nil {
				total += imageTokenCost
			}
		}
		// Encrypted continuation items also occupy request context, even though
		// they must never be rendered as thought text.
		for _, raw := range msg.ProviderMetadata {
			total += estimateText(string(raw))
		}
		for _, tc := range msg.ToolCalls {
			total += estimateText(tc.Function.Name)
			total += estimateText(argsToString(tc.Function.Arguments))
		}
	}
	return total
}

// imageTokenCost is the flat estimate for one image, aligned with the per-image
// accounting used by computer-use agents.
const imageTokenCost = 1500

// EstimateToolSchemas returns a rough token estimate for the tool definitions
// sent with a request. It uses the same provider-agnostic heuristic as
// EstimateTokens (~4 bytes per token).
func EstimateToolSchemas(schemas []map[string]any) int {
	total := 0
	for _, schema := range schemas {
		data, err := json.Marshal(schema)
		if err != nil {
			continue
		}
		total += estimateText(string(data))
	}
	return total
}

// imagePlaceholder replaces evicted image items while keeping the message
// structure and its text intact.
const imagePlaceholder = "[image removed to save context]"

type imageBatch struct {
	start, end   int
	count, bytes int
}

// EvictTranscriptImages keeps the transcript inside the image budget by
// replacing the images of the oldest tool batches with a text placeholder. The
// newest image-bearing batch is never evicted. A non-positive limit disables
// that dimension.
func EvictTranscriptImages(messages []provider.Message, maxCount, maxBytes int) []provider.Message {
	if len(messages) == 0 || (maxCount <= 0 && maxBytes <= 0) {
		return messages
	}

	batches := imageBatches(messages)
	if len(batches) == 0 {
		return messages
	}

	totalCount, totalBytes := 0, 0
	for _, batch := range batches {
		totalCount += batch.count
		totalBytes += batch.bytes
	}
	overBudget := func() bool {
		return (maxCount > 0 && totalCount > maxCount) || (maxBytes > 0 && totalBytes > maxBytes)
	}
	if !overBudget() {
		return messages
	}

	// Never evict the newest image-bearing batch; it is the current step.
	for i := 0; i < len(batches)-1 && overBudget(); i++ {
		batch := batches[i]
		for j := batch.start; j < batch.end; j++ {
			stripMessageImages(&messages[j])
		}
		totalCount -= batch.count
		totalBytes -= batch.bytes
	}
	return messages
}

// imageBatches groups consecutive tool messages (one tool round) as a single
// eviction unit. Non-tool messages with images are their own batch.
func imageBatches(messages []provider.Message) []imageBatch {
	var batches []imageBatch
	for i := 0; i < len(messages); {
		if messages[i].Role == "tool" {
			j := i
			count, bytes := 0, 0
			for j < len(messages) && messages[j].Role == "tool" {
				c, b := messageImageUsage(&messages[j])
				count += c
				bytes += b
				j++
			}
			if count > 0 {
				batches = append(batches, imageBatch{start: i, end: j, count: count, bytes: bytes})
			}
			i = j
			continue
		}
		if count, bytes := messageImageUsage(&messages[i]); count > 0 {
			batches = append(batches, imageBatch{start: i, end: i + 1, count: count, bytes: bytes})
		}
		i++
	}
	return batches
}

func messageImageUsage(msg *provider.Message) (count, bytes int) {
	for _, item := range provider.MessageContentItems(*msg) {
		if item.Type != "image_url" || item.ImageURL == nil {
			continue
		}
		count++
		bytes += len(item.ImageURL.URL)
	}
	return count, bytes
}

func stripMessageImages(msg *provider.Message) (count, bytes int) {
	items := provider.MessageContentItems(*msg)
	kept := make([]provider.ContentItem, 0, len(items)+1)
	for _, item := range items {
		if item.Type == "image_url" {
			count++
			if item.ImageURL != nil {
				bytes += len(item.ImageURL.URL)
			}
			continue
		}
		kept = append(kept, item)
	}
	if count == 0 {
		return 0, 0
	}
	kept = append(kept, provider.ContentItem{Type: "text", Text: imagePlaceholder})
	msg.Content = kept
	msg.ProviderMetadata = nil
	return count, bytes
}

// brokenImagePlaceholder stands in for an image whose payload cannot be
// repaired, so the rest of the request still goes out.
const brokenImagePlaceholder = "[image data was invalid and has been removed]"

// SanitizeImageDataURLs repairs image items before they are sent. Older MCP tool
// results embedded raw image bytes after ";base64," instead of the encoded text,
// which makes providers reject the entire request with "Invalid base64 data".
// A payload that is already base64 is left untouched, raw image bytes are
// re-encoded, and anything unrecognisable becomes a text placeholder.
func SanitizeImageDataURLs(messages []provider.Message) []provider.Message {
	if len(messages) == 0 {
		return messages
	}

	var out []provider.Message
	for i, msg := range messages {
		items := provider.MessageContentItems(msg)
		if len(items) == 0 {
			continue
		}

		repaired := make([]provider.ContentItem, 0, len(items))
		changed := false
		for _, item := range items {
			if item.Type != "image_url" || item.ImageURL == nil || item.ImageURL.URL == "" {
				repaired = append(repaired, item)
				continue
			}
			url, ok := sanitizeImageDataURL(item.ImageURL.URL)
			if !ok {
				changed = true
				repaired = append(repaired, provider.ContentItem{Type: "text", Text: brokenImagePlaceholder})
				continue
			}
			if url != item.ImageURL.URL {
				changed = true
				item.ImageURL = &provider.ImageInfo{URL: url}
			}
			repaired = append(repaired, item)
		}

		if !changed {
			continue
		}
		if out == nil {
			out = append([]provider.Message(nil), messages...)
		}
		out[i].Content = repaired
		out[i].ProviderMetadata = nil
	}

	if out == nil {
		return messages
	}
	return out
}

// sanitizeImageDataURL returns a safe URL and whether the item may be sent.
// Plain (http) URLs and non-base64 data URLs are passed through.
func sanitizeImageDataURL(raw string) (string, bool) {
	const prefix = "data:"
	if !strings.HasPrefix(raw, prefix) {
		return raw, true
	}
	comma := strings.IndexByte(raw, ',')
	if comma < 0 || !strings.Contains(raw[len(prefix):comma], ";base64") {
		return raw, true
	}
	payload := raw[comma+1:]
	if isBase64Payload(payload) {
		return raw, true
	}
	if looksLikeImage(payload) {
		return raw[:comma+1] + base64.StdEncoding.EncodeToString([]byte(payload)), true
	}
	return "", false
}

// isBase64Payload reports whether s can be decoded as standard base64. It only
// scans the bytes, so a broken multi-megabyte payload costs one pass.
func isBase64Payload(s string) bool {
	if len(s) < 4 {
		return false
	}
	padding := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '+', c == '/':
			if padding > 0 {
				return false
			}
		case c == '=':
			padding++
			if padding > 2 {
				return false
			}
		case c == '\r' || c == '\n':
			// MIME-style line breaks are ignored by decoders.
		default:
			return false
		}
	}
	return true
}

// looksLikeImage reports whether s starts with a known image signature, which
// distinguishes raw bytes from base64 text.
func looksLikeImage(s string) bool {
	switch {
	case strings.HasPrefix(s, "\x89PNG\r\n\x1a\n"):
		return true
	case strings.HasPrefix(s, "\xff\xd8\xff"):
		return true
	case strings.HasPrefix(s, "GIF87a"), strings.HasPrefix(s, "GIF89a"):
		return true
	case len(s) >= 12 && strings.HasPrefix(s, "RIFF") && s[8:12] == "WEBP":
		return true
	}
	return false
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
