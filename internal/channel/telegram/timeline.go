package telegram

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strings"
	"time"
	"unicode/utf16"

	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/utils"
)

const (
	telegramMaxLen     = 4096
	telegramTextChunk  = 3800
	telegramTruncLabel = "\n… (truncated)"

	telegramEditThreshold = 500
	telegramEditInterval  = time.Second
)

// telegramTransport isolates Telegram API calls so the timeline state machine
// can be tested without network access.
type telegramTransport interface {
	Send(chatID, replyTo int, text, parseMode string) (int, error)
	Edit(chatID, messageID int, text, parseMode string) error
}

type telegramAPI struct{}

func (telegramAPI) Send(chatID, replyTo int, text, parseMode string) (int, error) {
	res, err := SendTelegramMessage(chatID, replyTo, text, parseMode)
	if err != nil {
		return 0, err
	}
	if res == nil || !res.Ok {
		description := ""
		if res != nil {
			description = res.Description
		}
		return 0, fmt.Errorf("telegram send failed: %s", description)
	}
	return res.Result.MessageId, nil
}

func (telegramAPI) Edit(chatID, messageID int, text, parseMode string) error {
	res, err := EditTelegramMessage(chatID, 0, messageID, text, parseMode)
	if err != nil {
		return err
	}
	if res == nil || !res.Ok {
		description := ""
		if res != nil {
			description = res.Description
		}
		return fmt.Errorf("telegram edit failed: %s", description)
	}
	return nil
}

// telegramTimelineItem is one bubble in an agent turn.
type telegramTimelineItem struct {
	Kind       string // "thinking" | "tool" | "text"
	Text       string
	ToolCallID string
	ToolName   string
	ToolArgs   string
	ToolResult string
	IsError    bool
	Running    bool
	Final      bool
	MessageID  int

	lastRendered string
	lastEdit     time.Time
}

// telegramTimeline turns granular stream chunks into chronological Telegram
// bubbles. Thinking and tool items are rendered as expandable quotes; text
// items stream into their own bubble and the final answer gets the watermark.
type telegramTimeline struct {
	transport telegramTransport
	chatID    int
	replyTo   int

	placeholderID      int
	placeholderClaimed bool
	sentAny            bool

	items   []*telegramTimelineItem
	current *telegramTimelineItem
	tools   map[string]*telegramTimelineItem

	finalText   string
	finalTrace  []provider.ReactStep
	finalUsage  provider.Usage
	finalSent   bool
	attachments []channel.Attachment
	err         error

	editThreshold int
	editInterval  time.Duration
	now           func() time.Time
}

func newTelegramTimeline(transport telegramTransport, chatID, replyTo, placeholderID int) *telegramTimeline {
	return &telegramTimeline{
		transport:     transport,
		chatID:        chatID,
		replyTo:       replyTo,
		placeholderID: placeholderID,
		sentAny:       placeholderID != 0,
		tools:         map[string]*telegramTimelineItem{},
		editThreshold: telegramEditThreshold,
		editInterval:  telegramEditInterval,
		now:           time.Now,
	}
}

func (tl *telegramTimeline) handleChunk(chunk channel.StreamChunk) {
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
	case "attachment":
		tl.attachments = append(tl.attachments, chunk.Attachments...)
	case "step", "trace", "usage":
		// Step markers and traces are represented by their own items.
	default:
		// Backwards compatible with providers that only set the fields.
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

func (tl *telegramTimeline) appendText(kind, delta string) {
	if delta == "" {
		return
	}
	if tl.current != nil && tl.current.Kind == kind {
		tl.current.Text += delta
		tl.edit(tl.current, false)
		return
	}

	tl.closeCurrent()
	item := &telegramTimelineItem{Kind: kind, Text: delta}
	tl.items = append(tl.items, item)
	item.MessageID = tl.sendItem(item)
	tl.current = item
}

func (tl *telegramTimeline) startTool(callID, name, args string) {
	if name == "" {
		name = "tool"
	}
	tl.closeCurrent()

	item := &telegramTimelineItem{
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

func (tl *telegramTimeline) finishTool(callID, name, result string, isError bool) {
	item := tl.tools[callID]
	if item == nil {
		// Fall back to the most recent running tool, which handles providers
		// that omit tool call IDs.
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
		item = &telegramTimelineItem{
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
	if !item.IsError && telegramToolResultLooksLikeError(result) {
		item.IsError = true
	}
	item.Running = false
	tl.finalizeItem(item, false)
}

func (tl *telegramTimeline) finalize() {
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
	item := &telegramTimelineItem{Kind: "text", Text: tl.finalText, Final: true}
	tl.items = append(tl.items, item)
	tl.writeText(item, tl.finalText, true)
}

func (tl *telegramTimeline) finish() {
	// Command responses and older providers may never send a final chunk. In
	// that case the streamed text is the answer and still needs the watermark.
	if !tl.finalSent && tl.current != nil && tl.current.Kind == "text" {
		tl.finalize()
	} else {
		tl.closeCurrent()
	}

	// Tools without a result should not keep a spinner or look successful.
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

func (tl *telegramTimeline) closeCurrent() {
	if tl.current == nil {
		return
	}
	item := tl.current
	tl.current = nil
	tl.finalizeItem(item, false)
}

func (tl *telegramTimeline) finalizeItem(item *telegramTimelineItem, final bool) {
	if item == nil {
		return
	}
	if item.Kind == "text" {
		tl.writeText(item, item.Text, final)
		return
	}
	content, mode := tl.render(item, true)
	tl.editRaw(item, content, mode, true)
}

func (tl *telegramTimeline) recordErr(err error) {
	if err != nil && tl.err == nil {
		tl.err = err
	}
}

func (tl *telegramTimeline) writeText(item *telegramTimelineItem, text string, final bool) {
	if strings.TrimSpace(text) == "" {
		return
	}

	chunks := renderTelegramMarkdownChunks(text, telegramMaxLen)
	watermark := ""
	if final && config.WatermarkModel {
		watermark = strings.TrimSpace(utils.Watermark("", "", true))
	}

	watermarkSent := false
	for i, content := range chunks {
		if final && watermark != "" && i == len(chunks)-1 {
			if telegramUnits(content)+
				telegramUnits(watermark)+2 <= telegramMaxLen {
				content += "\n\n" + watermark
				watermarkSent = true
			}
		}
		if i == 0 && item.MessageID != 0 {
			tl.editRaw(item, content, "markdown", true)
			continue
		}
		messageID := tl.sendRaw(content, "markdown")
		if i == 0 {
			item.MessageID = messageID
		}
	}
	if watermark != "" && !watermarkSent {
		tl.sendRaw(watermark, "markdown")
	}
	item.Final = final
}

func (tl *telegramTimeline) sendItem(item *telegramTimelineItem) int {
	content, mode := tl.render(item, false)
	if content == "" {
		return 0
	}
	messageID := tl.sendRaw(content, mode)
	item.lastRendered = content
	item.lastEdit = tl.now()
	return messageID
}

func (tl *telegramTimeline) sendRaw(content, mode string) int {
	if strings.TrimSpace(content) == "" {
		return 0
	}
	if !tl.placeholderClaimed && tl.placeholderID != 0 {
		tl.placeholderClaimed = true
		tl.sentAny = true
		if err := tl.transport.Edit(tl.chatID, tl.placeholderID, content, mode); err != nil {
			log.Printf("telegram timeline: failed to claim placeholder: %v", err)
			tl.recordErr(err)
		}
		return tl.placeholderID
	}

	replyTo := 0
	if !tl.sentAny {
		replyTo = tl.replyTo
		tl.sentAny = true
	}
	messageID, err := tl.transport.Send(tl.chatID, replyTo, content, mode)
	if err != nil {
		log.Printf("telegram timeline: failed to send bubble: %v", err)
		tl.recordErr(err)
		return 0
	}
	return messageID
}

func (tl *telegramTimeline) edit(item *telegramTimelineItem, force bool) {
	content, mode := tl.render(item, false)
	tl.editRaw(item, content, mode, force)
}

func (tl *telegramTimeline) editRaw(item *telegramTimelineItem, content, mode string, force bool) {
	if item == nil || item.MessageID == 0 || strings.TrimSpace(content) == "" {
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
	if err := tl.transport.Edit(tl.chatID, item.MessageID, content, mode); err != nil {
		log.Printf("telegram timeline: failed to edit bubble: %v", err)
		tl.recordErr(err)
		return
	}
	item.lastRendered = content
	item.lastEdit = now
}

func (tl *telegramTimeline) render(item *telegramTimelineItem, closing bool) (string, string) {
	switch item.Kind {
	case "thinking":
		const (
			prefix = "💭 <b>Reasoning...</b>\n<blockquote expandable>"
			suffix = "</blockquote>"
		)
		budget := telegramMaxLen - telegramUnits(prefix) - telegramUnits(suffix)
		return prefix + escapeAndFit(item.Text, budget) + suffix, "HTML"

	case "tool":
		status := "[running]"
		if !item.Running {
			if item.IsError {
				status = "[error]"
			} else {
				status = "[ok]"
			}
		}
		name := truncateTelegramRunes(item.ToolName, 80)
		args := prettyJSON(item.ToolArgs)
		result := item.ToolResult

		build := func(args, result string) string {
			var b strings.Builder
			fmt.Fprintf(&b, "🛠️ <b>%s</b> %s", html.EscapeString(name), status)
			if args != "" || result != "" || item.Running {
				b.WriteString("\n<blockquote expandable>")
				if args != "" {
					b.WriteString("<b>Args</b>\n")
					b.WriteString(html.EscapeString(args))
				}
				if result != "" {
					if args != "" {
						b.WriteString("\n\n")
					}
					b.WriteString("<b>Result</b>\n")
					b.WriteString(html.EscapeString(result))
				}
				if result == "" && item.Running {
					if args != "" {
						b.WriteString("\n\n")
					}
					b.WriteString("Running...")
				}
				b.WriteString("</blockquote>")
			}
			return b.String()
		}

		out := build(args, result)
		if telegramUnits(out) <= telegramMaxLen {
			return out, "HTML"
		}

		// Shrink the result first, then the arguments. This keeps the rendered
		// HTML below Telegram's UTF-16 limit without cutting tags in half.
		result = shrinkTelegramToolField(result, func(candidate string) string { return build(args, candidate) })
		if telegramUnits(build(args, result)) > telegramMaxLen {
			result = ""
			args = shrinkTelegramToolField(args, func(candidate string) string { return build(candidate, "") })
		}
		return build(args, result), "HTML"

	case "text":
		if closing {
			chunks := renderTelegramMarkdownChunks(item.Text, telegramMaxLen)
			if len(chunks) == 0 {
				return "", "markdown"
			}
			return chunks[0], "markdown"
		}
		return escapeAndFit(item.Text, telegramMaxLen), "HTML"
	}
	return "", ""
}

func shrinkTelegramToolField(raw string, build func(string) string) string {
	runes := []rune(raw)
	lo, hi := 0, len(runes)
	best := ""
	for lo <= hi {
		mid := (lo + hi) / 2
		candidate := string(runes[:mid])
		if mid < len(runes) {
			candidate += telegramTruncLabel
		}
		if telegramUnits(build(candidate)) <= telegramMaxLen {
			best = candidate
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return best
}

// buildTelegramTimeline reconstructs a timeline for non-stream responses from
// the ReAct trace and the final answer.
func buildTelegramTimeline(trace []provider.ReactStep, finalText, finalThought string) []*telegramTimelineItem {
	var items []*telegramTimelineItem
	lastThought := ""
	for _, step := range trace {
		if step.Thought != "" && step.Thought != lastThought {
			items = append(items, &telegramTimelineItem{Kind: "thinking", Text: step.Thought})
			lastThought = step.Thought
		}
		if step.Action != "" {
			items = append(items, &telegramTimelineItem{
				Kind:       "tool",
				ToolName:   step.Action,
				ToolArgs:   step.ActionInput,
				ToolResult: step.Observation,
				IsError:    telegramToolResultLooksLikeError(step.Observation),
			})
		}
	}
	if len(items) == 0 && finalThought != "" {
		items = append(items, &telegramTimelineItem{Kind: "thinking", Text: finalThought})
	}
	if strings.TrimSpace(finalText) != "" {
		items = append(items, &telegramTimelineItem{Kind: "text", Text: finalText, Final: true})
	}
	return items
}

func telegramToolResultLooksLikeError(text string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(trimmed, "error") ||
		strings.HasPrefix(trimmed, "failed") ||
		strings.Contains(trimmed, "not available") ||
		strings.Contains(trimmed, "blocked")
}

func prettyJSON(raw string) string {
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

// escapeAndFit returns HTML-escaped text that fits within maxUnits once tags
// and entities are counted. Truncation is applied to the raw text so entities
// are never split.
func escapeAndFit(raw string, maxUnits int) string {
	escaped := html.EscapeString(raw)
	if telegramUnits(escaped) <= maxUnits {
		return escaped
	}
	budget := maxUnits - telegramUnits(telegramTruncLabel)
	if budget <= 0 {
		return ""
	}
	runes := []rune(raw)
	lo, hi := 0, len(runes)
	best := ""
	for lo <= hi {
		mid := (lo + hi) / 2
		candidate := html.EscapeString(string(runes[:mid]))
		if telegramUnits(candidate) <= budget {
			best = candidate
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return best + telegramTruncLabel
}

// renderTelegramMarkdownChunks converts markdown to Telegram markdown and
// splits the result until every chunk fits Telegram's limit. Recursive
// splitting handles text that expands after escaping.
func renderTelegramMarkdownChunks(text string, limit int) []string {
	var rendered []string
	var add func(string)
	add = func(chunk string) {
		if chunk == "" {
			return
		}
		out := utils.ParseTelegramMarkdown(chunk)
		if telegramUnits(out) <= limit {
			rendered = append(rendered, out)
			return
		}
		runes := []rune(chunk)
		if len(runes) <= 1 {
			rendered = append(rendered, truncateTelegramUnits(out, limit))
			return
		}
		mid := len(runes) / 2
		add(string(runes[:mid]))
		add(string(runes[mid:]))
	}
	for _, chunk := range splitTelegramText(text, telegramTextChunk) {
		add(chunk)
	}
	return rendered
}

func telegramUnits(text string) int {
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

func truncateTelegramUnits(text string, maxUnits int) string {
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

func truncateTelegramRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes-1]) + "…"
}

// splitTelegramText splits long text into chunks that fit Telegram's 4096
// UTF-16 unit limit, preferring a newline or space near the boundary.
func splitTelegramText(text string, limit int) []string {
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
