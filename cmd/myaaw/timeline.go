package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/cellbuf"

	"myaaw/internal/cli/theme"
	"myaaw/internal/provider"
)

var (
	thinkingStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Italic(true)
	toolArrowStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af"))
	toolNameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")).Bold(true)
	toolRunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af"))
	toolOKStyle      = theme.SuccessStyle
	toolErrorStyle   = theme.ErrorStyle
)

// timelineItem is one chronological step in an agent turn:
// thinking, assistant text, or a tool call/result.
type timelineItem struct {
	Kind string // "thinking" | "text" | "tool"
	// Step is the loop iteration this item belongs to, 0 when unknown.
	Step       int
	Text       string
	ToolCallID string
	ToolName   string
	ToolArgs   string
	ToolResult string
	IsError    bool
	Running    bool
	Expanded   bool
}

// tagSteps assigns the current step to items that do not carry one yet. Items
// already tagged (created in an earlier step) keep their step.
func tagSteps(items []timelineItem, step int) {
	if step <= 0 {
		return
	}
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Step != 0 {
			return
		}
		items[i].Step = step
	}
}

func appendThinkingItem(items []timelineItem, text string) []timelineItem {
	if text == "" {
		return items
	}
	if n := len(items); n > 0 && items[n-1].Kind == "thinking" {
		items[n-1].Text += text
		return items
	}
	return append(items, timelineItem{Kind: "thinking", Text: text})
}

func appendTextItem(items []timelineItem, text string) []timelineItem {
	if text == "" {
		return items
	}
	if n := len(items); n > 0 && items[n-1].Kind == "text" {
		items[n-1].Text += text
		return items
	}
	return append(items, timelineItem{Kind: "text", Text: text})
}

func startToolItem(items []timelineItem, id, name, args string) []timelineItem {
	if name == "" {
		return items
	}
	return append(items, timelineItem{
		Kind:       "tool",
		ToolCallID: id,
		ToolName:   name,
		ToolArgs:   args,
		Running:    true,
	})
}

func finishToolItem(items []timelineItem, id, result string, isError bool) []timelineItem {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Kind != "tool" {
			continue
		}
		if id != "" && items[i].ToolCallID != id {
			continue
		}
		if id == "" && !items[i].Running {
			continue
		}
		items[i].ToolResult = result
		items[i].IsError = isError
		items[i].Running = false
		return items
	}
	return items
}

// setFinalText replaces the most recent text item with the authoritative final
// answer, or appends one if the final answer was not streamed as text deltas.
// Text that appeared before the last tool call is intermediate and is kept.
func setFinalText(items []timelineItem, finalText string) []timelineItem {
	if finalText == "" {
		return items
	}
	lastText := -1
	lastTool := -1
	for i := len(items) - 1; i >= 0; i-- {
		if lastText == -1 && items[i].Kind == "text" {
			lastText = i
		}
		if lastTool == -1 && items[i].Kind == "tool" {
			lastTool = i
		}
		if lastText != -1 && lastTool != -1 {
			break
		}
	}
	if lastText != -1 && lastText > lastTool {
		items[lastText].Text = finalText
		return items
	}
	return append(items, timelineItem{Kind: "text", Text: finalText})
}

// timelineFromTrace rebuilds a timeline for non-stream responses, where the
// service only returns the final message plus the ReAct trace.
func timelineFromTrace(trace []provider.ReactStep, finalText string) []timelineItem {
	var items []timelineItem
	lastThought := ""
	for _, step := range trace {
		if step.Thought != "" && step.Thought != lastThought {
			items = appendThinkingItem(items, step.Thought)
			lastThought = step.Thought
		}
		items = startToolItem(items, "", step.Action, step.ActionInput)
		items = finishToolItem(items, "", step.Observation, toolResultLooksLikeError(step.Observation))
	}
	if finalText != "" {
		items = appendTextItem(items, finalText)
	}
	return items
}

func hasToolItems(items []timelineItem) bool {
	for _, item := range items {
		if item.Kind == "tool" {
			return true
		}
	}
	return false
}

func toolResultLooksLikeError(text string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(trimmed, "error") ||
		strings.HasPrefix(trimmed, "failed") ||
		strings.Contains(trimmed, "not available") ||
		strings.Contains(trimmed, "blocked")
}

// timelineLineSpan is the line range of one timeline item inside a rendered
// timeline string. It is used for mouse hit-testing. A zero span (End <= Start)
// means the item is currently hidden, e.g. inside a collapsed group.
type timelineLineSpan struct {
	Start int
	End   int
}

// timelineRenderOptions controls how an agent turn is rendered in the TUI.
type timelineRenderOptions struct {
	Streaming     bool
	GroupExpanded bool
	Duration      time.Duration
	Interrupted   bool
	// Steps is the number of loop iterations. Zero falls back to the number of
	// visible activity items, which is all history can offer for old traces.
	Steps int
}

func (m *model) timelineWidth() int {
	width := m.viewport.Width - 8
	if width < 24 {
		width = 24
	}
	return width
}

// timelineWrapWidth is the width at which the bot box word-wraps its content.
func (m *model) timelineWrapWidth() int {
	wrapWidth := m.viewport.Width - 6
	if wrapWidth < 1 {
		wrapWidth = 1
	}
	return wrapWidth
}

func (m *model) renderTimeline(items []timelineItem, streaming bool) (string, []timelineLineSpan) {
	if len(items) == 0 {
		return "", nil
	}
	width := m.timelineWidth()
	wrapWidth := m.timelineWrapWidth()

	var b strings.Builder
	spans := make([]timelineLineSpan, len(items))
	line := 0
	lastStep := 0
	for i, item := range items {
		if item.Step != 0 && item.Step != lastStep {
			separator := dimStyle.Render(fmt.Sprintf("  ── step %d ──", item.Step))
			b.WriteString(separator)
			b.WriteString("\n")
			line++
			lastStep = item.Step
		}
		spans[i].Start = line
		var rendered string
		switch item.Kind {
		case "thinking":
			rendered = renderThinkingItem(item, item.Expanded, width)
		case "text":
			rendered = renderTextItem(item.Text, m.renderer, streaming)
		case "tool":
			rendered = renderToolItem(item, item.Expanded, width)
		}
		b.WriteString(rendered)
		b.WriteString("\n")
		line += timelineDisplayRows(rendered, wrapWidth)
		// Add breathing room between different kinds of output (thinking,
		// tools, assistant text) but keep consecutive tool calls compact.
		if i < len(items)-1 && items[i].Kind != items[i+1].Kind {
			b.WriteString("\n")
			line++
		}
		spans[i].End = line
	}
	return strings.TrimRight(b.String(), "\n"), spans
}

// renderAgentTimeline renders one agent turn as a collapsible activity group
// followed by the final answer. The last text item is treated as the final
// answer and stays visible while the group is collapsed; thinking, tool calls,
// and intermediate text are kept inside the group.
func (m *model) renderAgentTimeline(items []timelineItem, opts timelineRenderOptions) (string, []timelineLineSpan, timelineLineSpan) {
	itemSpans := make([]timelineLineSpan, len(items))
	if len(items) == 0 {
		return "", itemSpans, timelineLineSpan{}
	}

	finalIndex := -1
	if items[len(items)-1].Kind == "text" {
		finalIndex = len(items) - 1
	}
	groupEnd := len(items)
	if finalIndex >= 0 {
		groupEnd = finalIndex
	}

	var b strings.Builder
	line := 0
	wrapWidth := m.timelineWrapWidth()
	groupSpan := timelineLineSpan{}
	if groupEnd > 0 {
		steps := opts.Steps
		if steps <= 0 {
			steps = groupEnd
		}
		header := renderActivityHeader(steps, opts.GroupExpanded)
		b.WriteString(header)
		b.WriteString("\n")
		line += timelineDisplayRows(header, wrapWidth)
		groupSpan = timelineLineSpan{Start: 0, End: line}

		if opts.GroupExpanded {
			b.WriteString("\n")
			line++
			groupSpan.End = line
			body, spans := m.renderTimeline(items[:groupEnd], opts.Streaming)
			b.WriteString(body)
			b.WriteString("\n")
			for i, span := range spans {
				if span.End > span.Start {
					itemSpans[i] = timelineLineSpan{Start: span.Start + line, End: span.End + line}
				}
			}
			line += timelineDisplayRows(body, wrapWidth)
		}
	}

	if finalIndex >= 0 {
		if line > 0 {
			b.WriteString("\n")
			line++
		}
		// The final answer belongs to the last loop iteration. Label it only when
		// it is a step of its own, so a single-step reply does not repeat the
		// label printed above its reasoning.
		lastBodyStep := 0
		if groupEnd > 0 {
			lastBodyStep = items[groupEnd-1].Step
		}
		finalStep := items[finalIndex].Step
		if finalStep == 0 {
			finalStep = opts.Steps
		}
		if opts.GroupExpanded && lastBodyStep != 0 && finalStep != lastBodyStep {
			separator := dimStyle.Render(fmt.Sprintf("  ── step %d ──", finalStep))
			b.WriteString(separator)
			b.WriteString("\n")
			line++
		}
		rendered := renderTextItem(items[finalIndex].Text, m.renderer, opts.Streaming)
		b.WriteString(rendered)
		b.WriteString("\n")
		line += timelineDisplayRows(rendered, wrapWidth)
	}

	if !opts.Streaming && opts.Duration > 0 {
		if line > 0 {
			b.WriteString("\n")
			line++
		}
		label := fmt.Sprintf("Completed in %s", formatDuration(opts.Duration))
		if opts.Interrupted {
			label = fmt.Sprintf("Interrupted after %s", formatDuration(opts.Duration))
		}
		b.WriteString(dimStyle.Render(label))
		b.WriteString("\n")
		line++
	}

	return strings.TrimRight(b.String(), "\n"), itemSpans, groupSpan
}

func renderActivityHeader(steps int, expanded bool) string {
	arrow := "▸"
	hint := "click to expand"
	if expanded {
		arrow = "▾"
		hint = "click to collapse"
	}
	plural := "s"
	if steps == 1 {
		plural = ""
	}
	label := fmt.Sprintf("Agent timeline · %d step%s", steps, plural)
	return thinkingStyle.Render(arrow+" ") + dimStyle.Bold(true).Render(label) + dimStyle.Render(" ("+hint+")")
}

func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	totalSeconds := int(d.Round(time.Second) / time.Second)
	return fmt.Sprintf("%dm %02ds", totalSeconds/60, totalSeconds%60)
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	totalSeconds := int(d.Round(time.Second) / time.Second)
	return fmt.Sprintf("%dm %02ds", totalSeconds/60, totalSeconds%60)
}

// timelineDisplayRows returns how many terminal rows a rendered block occupies
// after the bot box word-wraps it. Each logical line can become several rows,
// and click spans need those display rows to stay aligned with the viewport.
func timelineDisplayRows(text string, wrapWidth int) int {
	if text == "" {
		return 0
	}
	rows := 0
	for _, line := range strings.Split(text, "\n") {
		wrapped := cellbuf.Wrap(line, wrapWidth, "")
		if wrapped == "" {
			rows++
			continue
		}
		rows += strings.Count(wrapped, "\n") + 1
	}
	return rows
}

// advanceLines returns how many lines a rendered block advances the viewport
// cursor. Rendered blocks normally end with a newline.
func advanceLines(text string) int {
	if text == "" {
		return 0
	}
	lines := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		lines++
	}
	return lines
}

func (m *model) toggleTimelineItem(messageIndex, itemIndex int) {
	if messageIndex < 0 {
		if itemIndex >= 0 && itemIndex < len(m.streamingItems) {
			m.streamingItems[itemIndex].Expanded = !m.streamingItems[itemIndex].Expanded
		}
		return
	}
	if messageIndex >= len(m.messages) {
		return
	}
	msg := &m.messages[messageIndex]
	if itemIndex < 0 || itemIndex >= len(msg.items) {
		return
	}
	msg.items[itemIndex].Expanded = !msg.items[itemIndex].Expanded
	msg.cachedView = ""
	msg.cachedSpans = nil
}

func (m *model) toggleTimelineGroup(messageIndex int) {
	if messageIndex < 0 {
		m.streamingGroupExpanded = !m.streamingGroupExpanded
		return
	}
	if messageIndex >= len(m.messages) {
		return
	}
	msg := &m.messages[messageIndex]
	msg.groupExpanded = !msg.groupExpanded
	msg.cachedView = ""
	msg.cachedSpans = nil
	msg.cachedGroupSpan = timelineLineSpan{}
}

func hasTimelineGroup(items []timelineItem) bool {
	if len(items) == 0 {
		return false
	}
	if items[len(items)-1].Kind == "text" {
		return len(items) > 1
	}
	return true
}

func (m *model) anyTimelineExpanded() bool {
	if len(m.streamingItems) > 0 && m.streamingGroupExpanded {
		return true
	}
	for _, msg := range m.messages {
		if !msg.groupExpanded || !hasTimelineGroup(msg.items) {
			continue
		}
		return true
	}
	return false
}

func (m *model) setAllTimelineExpanded(expanded bool) {
	m.streamingGroupExpanded = expanded
	for i := range m.streamingItems {
		m.streamingItems[i].Expanded = expanded
	}
	for i := range m.messages {
		if len(m.messages[i].items) == 0 {
			continue
		}
		m.messages[i].groupExpanded = expanded
		for j := range m.messages[i].items {
			m.messages[i].items[j].Expanded = expanded
		}
		m.messages[i].cachedView = ""
		m.messages[i].cachedSpans = nil
		m.messages[i].cachedGroupSpan = timelineLineSpan{}
	}
}

func renderThinkingItem(item timelineItem, expanded bool, width int) string {
	text := sanitizeForTUI(item.Text)
	previewWidth := width - len("▸ Thinking (click to expand)  ")
	if previewWidth < 20 {
		previewWidth = 20
	}
	preview := oneLine(text, previewWidth)
	if expanded {
		header := thinkingStyle.Render("▾ Thinking (click to collapse)")
		body := theme.BoxStyle.Padding(0, 1).BorderForeground(lipgloss.Color("#89b4fa")).Width(width).Render(wrapPlainText(text, width-2))
		return header + "\n" + body
	}
	if preview == "" {
		return thinkingStyle.Render("▸ Thinking (click to expand)")
	}
	return thinkingStyle.Render("▸ Thinking (click to expand)  ") + dimStyle.Render(preview)
}

func renderTextItem(text string, renderer *glamour.TermRenderer, streaming bool) string {
	if streaming || renderer == nil {
		return text
	}
	rendered, err := renderer.Render(text)
	if err != nil || strings.TrimSpace(rendered) == "" {
		return text
	}
	return strings.TrimSpace(rendered)
}

func renderToolItem(item timelineItem, expanded bool, width int) string {
	status := "[running]"
	statusStyle := toolRunningStyle
	if !item.Running {
		if item.IsError {
			status = "[error]"
			statusStyle = toolErrorStyle
		} else {
			status = "[ok]"
			statusStyle = toolOKStyle
		}
	}

	argsText := sanitizeForTUI(item.ToolArgs)
	resultText := sanitizeForTUI(item.ToolResult)

	overhead := utf8.RuneCountInString("▸ ") + len("tool: ") + utf8.RuneCountInString(item.ToolName) + 1 + len(status) + len(" (click to expand)")
	summaryBudget := width - overhead - 2
	if summaryBudget < 0 {
		summaryBudget = 0
	}
	var argsPreview, resultPreview string
	switch {
	case argsText != "" && resultText != "":
		const separator = " -> "
		remaining := summaryBudget - len(separator)
		if remaining < 0 {
			remaining = 0
		}
		argsBudget := remaining / 3
		resultBudget := remaining - argsBudget
		argsPreview = oneLineWithin(argsText, argsBudget)
		resultPreview = oneLineWithin(resultText, resultBudget)
	case argsText != "":
		argsPreview = oneLineWithin(argsText, summaryBudget)
	case resultText != "":
		resultPreview = oneLineWithin(resultText, summaryBudget)
	}

	if !expanded {
		header := toolArrowStyle.Render("▸ ") + toolNameStyle.Render("tool: "+item.ToolName) + " " + statusStyle.Render(status) + dimStyle.Render(" (click to expand)")
		summary := argsPreview
		if resultPreview != "" {
			if summary != "" {
				summary += " -> "
			}
			summary += resultPreview
		}
		if summary != "" {
			header += "  " + dimStyle.Render(summary)
		}
		return header
	}

	var b strings.Builder
	b.WriteString(toolArrowStyle.Render("▾ ") + toolNameStyle.Render("tool: "+item.ToolName) + " " + statusStyle.Render(status) + dimStyle.Render(" (click to collapse)") + "\n")
	if item.ToolArgs != "" {
		b.WriteString(dimStyle.Render("Args") + "\n")
		args := wrapPlainText(truncateMultiline(prettyJSON(argsText), 2000), width-2)
		b.WriteString(theme.BoxStyle.Padding(0, 1).BorderForeground(lipgloss.Color("#f9e2af")).Width(width).Render(args) + "\n")
	}
	if item.Running {
		b.WriteString(dimStyle.Render("Result: running...") + "\n")
	} else if item.ToolResult != "" {
		b.WriteString(dimStyle.Render("Result") + "\n")
		result := wrapPlainText(truncateMultiline(resultText, 2000), width-2)
		b.WriteString(theme.BoxStyle.Padding(0, 1).BorderForeground(lipgloss.Color("#f9e2af")).Width(width).Render(result) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func prettyJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
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

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func sanitizeForTUI(text string) string {
	text = ansiEscapePattern.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\t", "  ")
	return text
}

func wrapPlainText(text string, width int) string {
	if width <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, wrapLine(line, width)...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapLine(line string, width int) []string {
	runes := []rune(line)
	if len(runes) <= width {
		return []string{line}
	}

	indent := 0
	for indent < len(runes) && runes[indent] == ' ' {
		indent++
	}
	prefix := string(runes[:indent])
	words := strings.Fields(string(runes[indent:]))
	if len(words) == 0 {
		return []string{line}
	}

	var out []string
	current := prefix
	for _, word := range words {
		candidate := prefix + word
		if strings.TrimSpace(current) != "" {
			candidate = current + " " + word
		}
		if utf8.RuneCountInString(candidate) > width && strings.TrimSpace(current) != "" {
			out = append(out, current)
			current = prefix + word
			continue
		}
		current = candidate
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}

func oneLine(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(sanitizeForTUI(text)), " ")
	return truncateRunes(text, maxRunes)
}

// oneLineWithin collapses text to a single line and truncates it so the result,
// including the ellipsis, is never wider than maxRunes.
func oneLineWithin(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(sanitizeForTUI(text)), " ")
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}

func truncateMultiline(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "\n… (truncated)"
}

func truncateRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "…"
}
