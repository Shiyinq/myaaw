package discord

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/bwmarrin/discordgo"

	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/utils"
)

const (
	discordMaxLen        = 2000
	discordTextChunk     = 1800
	discordTruncLabel    = "\n… (truncated)"
	discordEditThreshold = 400
	discordEditInterval  = 1500 * time.Millisecond
)

// discordTransport isolates Discord API calls so the timeline state machine
// can be tested without a live gateway session.
type discordTransport interface {
	Send(channelID, content string) (string, error)
	SendReply(channelID, replyTo, content string) (string, error)
	Edit(channelID, messageID, content string) error
}

type discordAPI struct {
	session *discordgo.Session
}

func (a discordAPI) Send(channelID, content string) (string, error) {
	message, err := a.session.ChannelMessageSend(channelID, content)
	if err != nil {
		return "", err
	}
	return message.ID, nil
}

func (a discordAPI) SendReply(channelID, replyTo, content string) (string, error) {
	if replyTo == "" {
		return a.Send(channelID, content)
	}
	reference := &discordgo.MessageReference{MessageID: replyTo, ChannelID: channelID}
	message, err := a.session.ChannelMessageSendReply(channelID, content, reference)
	if err != nil {
		// A deleted referenced message must not drop the whole response.
		return a.Send(channelID, content)
	}
	return message.ID, nil
}

func (a discordAPI) Edit(channelID, messageID, content string) error {
	_, err := a.session.ChannelMessageEdit(channelID, messageID, content)
	return err
}

// discordTimelineItem is one message in an agent turn.
type discordTimelineItem struct {
	Kind       string // "thinking" | "tool" | "text"
	Text       string
	ToolCallID string
	ToolName   string
	ToolArgs   string
	ToolResult string
	IsError    bool
	Running    bool
	Final      bool
	MessageID  string

	lastRendered string
	lastEdit     time.Time
}

// discordTimeline turns stream chunks into chronological Discord messages.
// Thinking and tools use blockquote spoilers; text streams into its own
// message and the final answer is split across messages instead of truncated.
type discordTimeline struct {
	transport discordTransport
	channelID string
	replyTo   string

	placeholderID      string
	placeholderClaimed bool
	sentAny            bool

	items   []*discordTimelineItem
	current *discordTimelineItem
	tools   map[string]*discordTimelineItem

	finalText  string
	finalTrace []provider.ReactStep
	finalUsage provider.Usage
	finalSent  bool
	err        error

	editThreshold int
	editInterval  time.Duration
	now           func() time.Time
}

func newDiscordTimeline(transport discordTransport, channelID, replyTo, placeholderID string) *discordTimeline {
	return &discordTimeline{
		transport:     transport,
		channelID:     channelID,
		replyTo:       replyTo,
		placeholderID: placeholderID,
		sentAny:       placeholderID != "",
		tools:         map[string]*discordTimelineItem{},
		editThreshold: discordEditThreshold,
		editInterval:  discordEditInterval,
		now:           time.Now,
	}
}

func (tl *discordTimeline) handleChunk(chunk channel.StreamChunk) {
	if chunk.Usage.TotalTokens > 0 {
		tl.finalUsage = chunk.Usage
	}
	if len(chunk.Trace) > 0 {
		tl.finalTrace = chunk.Trace
	}

	switch chunk.Kind {
	case "final":
		if chunk.Text != "" {
			tl.finalText = chunk.Text
		}
		if chunk.Usage.TotalTokens > 0 {
			tl.finalUsage = chunk.Usage
		}
		if len(chunk.Trace) > 0 {
			tl.finalTrace = chunk.Trace
		}
		tl.finalize()
	case "thought", "thinking":
		tl.appendText("thinking", chunk.Thought)
	case "text":
		tl.appendText("text", chunk.Text)
	case "tool":
		name := chunk.ToolName
		if name == "" && len(chunk.ToolCalls) > 0 {
			name = chunk.ToolCalls[0].Function.Name
		}
		tl.startTool(chunk.ToolCallID, name, chunk.ToolArguments)
	case "tool_result":
		tl.finishTool(chunk.ToolCallID, chunk.ToolName, chunk.ToolResultText, chunk.IsError)
	case "trace", "usage":
		// Tool lifecycle events already carry the descriptive data.
	default:
		if chunk.Thought != "" {
			tl.appendText("thinking", chunk.Thought)
		}
		if chunk.Text != "" {
			tl.appendText("text", chunk.Text)
		}
		for _, call := range chunk.ToolCalls {
			tl.startTool(call.ID, call.Function.Name, chunk.ToolArguments)
		}
	}
}

func (tl *discordTimeline) appendText(kind, delta string) {
	if delta == "" {
		return
	}
	if tl.current != nil && tl.current.Kind == kind {
		tl.current.Text += delta
		tl.edit(tl.current, false)
		return
	}

	tl.closeCurrent()
	item := &discordTimelineItem{Kind: kind, Text: delta}
	tl.items = append(tl.items, item)
	item.MessageID = tl.sendItem(item)
	tl.current = item
}

func (tl *discordTimeline) startTool(callID, name, args string) {
	if name == "" {
		name = "tool"
	}
	tl.closeCurrent()

	item := &discordTimelineItem{
		Kind:       "tool",
		ToolCallID: callID,
		ToolName:   name,
		ToolArgs:   args,
		Running:    true,
	}
	tl.items = append(tl.items, item)
	if callID != "" {
		tl.tools[callID] = item
	}
	item.MessageID = tl.sendItem(item)
}

func (tl *discordTimeline) finishTool(callID, name, result string, isError bool) {
	item := tl.tools[callID]
	if item == nil {
		for i := len(tl.items) - 1; i >= 0; i-- {
			candidate := tl.items[i]
			if candidate.Kind != "tool" || !candidate.Running {
				continue
			}
			if name != "" && candidate.ToolName != name {
				continue
			}
			item = candidate
			break
		}
	}
	if item == nil {
		tl.closeCurrent()
		item = &discordTimelineItem{
			Kind:       "tool",
			ToolCallID: callID,
			ToolName:   name,
			ToolResult: result,
			IsError:    isError,
		}
		if item.ToolName == "" {
			item.ToolName = "tool"
		}
		tl.items = append(tl.items, item)
		item.MessageID = tl.sendItem(item)
	}

	item.ToolResult = result
	item.IsError = isError
	if !item.IsError && discordToolResultLooksLikeError(result) {
		item.IsError = true
	}
	item.Running = false
	tl.finalizeItem(item, false)
}

func (tl *discordTimeline) finalize() {
	if tl.finalSent {
		return
	}
	tl.finalSent = true

	if tl.current != nil && tl.current.Kind == "text" {
		item := tl.current
		tl.current = nil
		text := tl.finalText
		if text == "" {
			text = item.Text
			tl.finalText = text
		}
		tl.writeText(item, text, true)
		return
	}

	tl.closeCurrent()
	if tl.finalText == "" {
		return
	}
	item := &discordTimelineItem{Kind: "text", Text: tl.finalText, Final: true}
	tl.items = append(tl.items, item)
	tl.writeText(item, tl.finalText, true)
}

func (tl *discordTimeline) finish() {
	// Command responses and older providers may never send a final chunk.
	if !tl.finalSent && tl.current != nil && tl.current.Kind == "text" {
		tl.finalize()
	} else {
		tl.closeCurrent()
	}

	for _, item := range tl.items {
		if item.Kind == "tool" && item.Running {
			item.Running = false
			if item.ToolResult == "" {
				item.IsError = true
			}
			tl.finalizeItem(item, false)
		}
	}
}

func (tl *discordTimeline) closeCurrent() {
	if tl.current == nil {
		return
	}
	item := tl.current
	tl.current = nil
	tl.finalizeItem(item, false)
}

func (tl *discordTimeline) finalizeItem(item *discordTimelineItem, final bool) {
	if item == nil {
		return
	}
	if item.Kind == "text" {
		tl.writeText(item, item.Text, final)
		return
	}
	content := tl.render(item)
	tl.editRaw(item, content, true)
}

func (tl *discordTimeline) writeText(item *discordTimelineItem, text string, final bool) {
	if strings.TrimSpace(text) == "" {
		return
	}

	chunks := splitDiscordText(text, discordTextChunk)
	watermark := ""
	if final && config.WatermarkModel {
		watermark = strings.TrimSpace(utils.Watermark("", "", true))
	}
	watermarkSent := false

	for i, chunk := range chunks {
		content := chunk
		if final && watermark != "" && i == len(chunks)-1 {
			if discordUnits(content)+discordUnits(watermark)+2 <= discordMaxLen {
				content += "\n\n" + watermark
				watermarkSent = true
			}
		}
		if i == 0 && item.MessageID != "" {
			tl.editRaw(item, content, true)
			continue
		}
		messageID := tl.sendRaw(content)
		if i == 0 {
			item.MessageID = messageID
		}
	}
	if watermark != "" && !watermarkSent {
		tl.sendRaw(watermark)
	}
	item.Final = final
}

func (tl *discordTimeline) sendItem(item *discordTimelineItem) string {
	content := tl.render(item)
	if content == "" {
		return ""
	}
	messageID := tl.sendRaw(content)
	item.lastRendered = content
	item.lastEdit = tl.now()
	return messageID
}

func (tl *discordTimeline) sendRaw(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	if !tl.placeholderClaimed && tl.placeholderID != "" {
		tl.placeholderClaimed = true
		tl.sentAny = true
		if err := tl.transport.Edit(tl.channelID, tl.placeholderID, content); err != nil {
			log.Printf("discord timeline: failed to claim placeholder: %v", err)
			tl.recordErr(err)
		}
		return tl.placeholderID
	}

	var (
		messageID string
		err       error
	)
	if !tl.sentAny && tl.replyTo != "" {
		messageID, err = tl.transport.SendReply(tl.channelID, tl.replyTo, content)
	} else {
		messageID, err = tl.transport.Send(tl.channelID, content)
	}
	if !tl.sentAny {
		tl.sentAny = true
	}
	if err != nil {
		log.Printf("discord timeline: failed to send bubble: %v", err)
		tl.recordErr(err)
		return ""
	}
	return messageID
}

func (tl *discordTimeline) edit(item *discordTimelineItem, force bool) {
	content := tl.render(item)
	tl.editRaw(item, content, force)
}

func (tl *discordTimeline) editRaw(item *discordTimelineItem, content string, force bool) {
	if item == nil || item.MessageID == "" || strings.TrimSpace(content) == "" {
		return
	}
	if content == item.lastRendered {
		return
	}
	now := tl.now()
	if !force && tl.editThreshold > 0 &&
		len(content)-len(item.lastRendered) < tl.editThreshold &&
		now.Sub(item.lastEdit) < tl.editInterval {
		return
	}
	if err := tl.transport.Edit(tl.channelID, item.MessageID, content); err != nil {
		log.Printf("discord timeline: failed to edit bubble: %v", err)
		tl.recordErr(err)
		return
	}
	item.lastRendered = content
	item.lastEdit = now
}

func (tl *discordTimeline) recordErr(err error) {
	if err != nil && tl.err == nil {
		tl.err = err
	}
}

func (tl *discordTimeline) render(item *discordTimelineItem) string {
	switch item.Kind {
	case "thinking":
		const header = "**💭 Reasoning...**\n"
		build := func(candidate string) string {
			return header + discordQuote(candidate)
		}
		out := build(item.Text)
		if discordUnits(out) <= discordMaxLen {
			return out
		}
		return build(shrinkDiscordToolField(item.Text, build))

	case "tool":
		status := "[running]"
		if !item.Running {
			if item.IsError {
				status = "[error]"
			} else {
				status = "[ok]"
			}
		}
		name := truncateDiscordRunes(item.ToolName, 80)
		header := fmt.Sprintf("**🛠️ %s %s**\n", name, status)
		argsText := prettyDiscordJSON(item.ToolArgs)

		build := func(args, result string) string {
			var inner strings.Builder
			if args != "" {
				inner.WriteString("Args:\n")
				inner.WriteString(args)
			}
			if result != "" {
				if args != "" {
					inner.WriteString("\n\n")
				}
				inner.WriteString("Result:\n")
				inner.WriteString(result)
			}
			if result == "" && item.Running {
				if args != "" {
					inner.WriteString("\n\n")
				}
				inner.WriteString("Running...")
			}
			if inner.Len() == 0 {
				return header
			}
			return header + discordQuote(inner.String())
		}

		out := build(argsText, item.ToolResult)
		if discordUnits(out) <= discordMaxLen {
			return out
		}

		result := shrinkDiscordToolField(item.ToolResult, func(candidate string) string {
			return build(argsText, candidate)
		})
		if discordUnits(build(argsText, result)) > discordMaxLen {
			result = ""
			args := shrinkDiscordToolField(argsText, func(candidate string) string {
				return build(candidate, "")
			})
			return build(args, "")
		}
		return build(argsText, result)

	case "text":
		return truncateDiscord(item.Text, discordMaxLen)
	}
	return ""
}

func shrinkDiscordToolField(raw string, build func(string) string) string {
	runes := []rune(raw)
	lo, hi := 0, len(runes)
	best := ""
	for lo <= hi {
		mid := (lo + hi) / 2
		candidate := string(runes[:mid])
		if mid < len(runes) {
			candidate += discordTruncLabel
		}
		if discordUnits(build(candidate)) <= discordMaxLen {
			best = candidate
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return best
}

func discordQuote(text string) string {
	if text == "" {
		return ""
	}
	// A multiline blockquote keeps the content visible while still visually
	// grouping it, unlike the old spoiler style (||...||).
	return ">>> " + text
}

// buildDiscordTimeline reconstructs a timeline for non-stream responses from
// the ReAct trace and the final answer.
func buildDiscordTimeline(trace []provider.ReactStep, finalText, finalThought string) []*discordTimelineItem {
	var items []*discordTimelineItem
	lastThought := ""
	for _, step := range trace {
		if step.Thought != "" && step.Thought != lastThought {
			items = append(items, &discordTimelineItem{Kind: "thinking", Text: step.Thought})
			lastThought = step.Thought
		}
		if step.Action != "" {
			items = append(items, &discordTimelineItem{
				Kind:       "tool",
				ToolName:   step.Action,
				ToolArgs:   step.ActionInput,
				ToolResult: step.Observation,
				IsError:    discordToolResultLooksLikeError(step.Observation),
			})
		}
	}
	if len(items) == 0 && finalThought != "" {
		items = append(items, &discordTimelineItem{Kind: "thinking", Text: finalThought})
	}
	if strings.TrimSpace(finalText) != "" {
		items = append(items, &discordTimelineItem{Kind: "text", Text: finalText, Final: true})
	}
	return items
}

func discordToolResultLooksLikeError(text string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(trimmed, "error") ||
		strings.HasPrefix(trimmed, "failed") ||
		strings.Contains(trimmed, "not available") ||
		strings.Contains(trimmed, "blocked")
}

func prettyDiscordJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var value interface{}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	pretty, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return raw
	}
	return string(pretty)
}

func truncateDiscord(text string, maxUnits int) string {
	if maxUnits <= 0 || discordUnits(text) <= maxUnits {
		return text
	}
	budget := maxUnits - discordUnits(discordTruncLabel)
	if budget <= 0 {
		return ""
	}
	return truncateDiscordUnits(text, budget) + discordTruncLabel
}

func truncateDiscordUnits(text string, maxUnits int) string {
	if maxUnits <= 0 {
		return ""
	}
	runes := []rune(text)
	units := 0
	for i, r := range runes {
		runeUnits := utf16.RuneLen(r)
		if runeUnits < 0 {
			runeUnits = 1
		}
		if units+runeUnits > maxUnits {
			return string(runes[:i])
		}
		units += runeUnits
	}
	return text
}

func truncateDiscordRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes-1]) + "…"
}

func discordUnits(text string) int {
	total := 0
	for _, r := range text {
		units := utf16.RuneLen(r)
		if units < 0 {
			units = 1
		}
		total += units
	}
	return total
}

// splitDiscordText splits text into chunks that fit Discord's message limit,
// preferring a newline or space near the boundary.
func splitDiscordText(text string, limit int) []string {
	if limit <= 0 {
		return []string{text}
	}
	runes := []rune(text)
	var chunks []string
	for start := 0; start < len(runes); {
		units := 0
		end := start
		for end < len(runes) {
			runeUnits := utf16.RuneLen(runes[end])
			if runeUnits < 0 {
				runeUnits = 1
			}
			if units+runeUnits > limit && end > start {
				break
			}
			units += runeUnits
			end++
		}
		if end < len(runes) {
			for i := end - 1; i > start && i > end-200; i-- {
				if runes[i] == '\n' || runes[i] == ' ' {
					end = i + 1
					break
				}
			}
		}
		chunks = append(chunks, string(runes[start:end]))
		start = end
	}
	return chunks
}
