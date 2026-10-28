package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"myaaw/internal/provider"
)

func TestTimelineAppendMerge(t *testing.T) {
	items := appendThinkingItem(nil, "first ")
	items = appendThinkingItem(items, "second")
	if len(items) != 1 || items[0].Kind != "thinking" || items[0].Text != "first second" {
		t.Fatalf("thinking items were not merged: %+v", items)
	}

	items = appendTextItem(items, "hello ")
	items = appendTextItem(items, "world")
	if len(items) != 2 || items[1].Kind != "text" || items[1].Text != "hello world" {
		t.Fatalf("text items were not merged: %+v", items)
	}

	items = startToolItem(items, "c1", "bash", `{"command":"ls"}`)
	items = appendTextItem(items, "after tool")
	if len(items) != 4 || items[2].Kind != "tool" || items[3].Kind != "text" {
		t.Fatalf("text after a tool call should start a new item: %+v", items)
	}
}

func TestTimelineToolLifecycle(t *testing.T) {
	items := startToolItem(nil, "c1", "bash", `{"command":"ls"}`)
	items = startToolItem(items, "c2", "filesystem", `{"tool_name":"list_directory"}`)
	items = finishToolItem(items, "c2", "listing", false)
	items = finishToolItem(items, "c1", "Error: failed", true)

	if len(items) != 2 {
		t.Fatalf("unexpected item count: %d", len(items))
	}
	if items[0].ToolCallID != "c1" || items[0].Running || !items[0].IsError || items[0].ToolResult != "Error: failed" {
		t.Fatalf("c1 was not finalized correctly: %+v", items[0])
	}
	if items[1].ToolCallID != "c2" || items[1].Running || items[1].IsError || items[1].ToolResult != "listing" {
		t.Fatalf("c2 was not finalized correctly: %+v", items[1])
	}
}

func TestSetFinalText(t *testing.T) {
	items := setFinalText(nil, "answer")
	if len(items) != 1 || items[0].Text != "answer" {
		t.Fatalf("final text was not appended: %+v", items)
	}

	items = []timelineItem{{Kind: "text", Text: "draft"}}
	items = setFinalText(items, "final")
	if len(items) != 1 || items[0].Text != "final" {
		t.Fatalf("draft text was not replaced: %+v", items)
	}

	items = []timelineItem{
		{Kind: "text", Text: "intermediate"},
		{Kind: "tool", ToolName: "bash", Running: false},
	}
	items = setFinalText(items, "final answer")
	if len(items) != 3 || items[2].Kind != "text" || items[2].Text != "final answer" {
		t.Fatalf("final text after a tool call should be appended: %+v", items)
	}
	if items[0].Text != "intermediate" {
		t.Fatalf("intermediate text was overwritten: %+v", items[0])
	}
}

func TestRenderTimelineShowsChronologicalDetails(t *testing.T) {
	m := &model{}
	m.viewport.Width = 80
	items := []timelineItem{
		{Kind: "thinking", Text: "inspect the directory"},
		{Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`, ToolResult: "file1\nfile2"},
		{Kind: "text", Text: "Done."},
	}

	collapsed, spans := m.renderTimeline(items, true)
	for _, want := range []string{"Thinking", "\u25b8", "bash", "file1", "Done."} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("collapsed timeline is missing %q:\n%s", want, collapsed)
		}
	}
	if len(spans) != len(items) {
		t.Fatalf("spans = %d, want %d", len(spans), len(items))
	}
	for i := 1; i < len(spans); i++ {
		if spans[i].Start != spans[i-1].End {
			t.Fatalf("spans are not contiguous: %+v", spans)
		}
	}
	lines := strings.Split(collapsed, "\n")
	for i, want := range []string{"Thinking", "tool:", "Done."} {
		if spans[i].Start >= len(lines) || !strings.Contains(lines[spans[i].Start], want) {
			t.Fatalf("span %d starts at the wrong line; want %q, got %q (spans=%+v)", i, want, lines[spans[i].Start], spans)
		}
	}

	items[0].Expanded = true
	items[1].Expanded = true
	expanded, _ := m.renderTimeline(items, true)
	for _, want := range []string{"\u25be", "Args", "Result", "inspect the directory", "ls"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded timeline is missing %q:\n%s", want, expanded)
		}
	}
}

func TestToggleTimelineItem(t *testing.T) {
	m := &model{
		messages: []chatMessage_{{
			role: "bot",
			items: []timelineItem{
				{Kind: "thinking", Text: "think"},
				{Kind: "tool", ToolName: "bash"},
			},
		}},
		streamingItems: []timelineItem{{Kind: "tool", ToolName: "filesystem"}},
	}

	m.toggleTimelineItem(0, 1)
	if !m.messages[0].items[1].Expanded {
		t.Fatal("click did not expand the tool item")
	}
	m.toggleTimelineItem(0, 1)
	if m.messages[0].items[1].Expanded {
		t.Fatal("second click did not collapse the tool item")
	}

	m.toggleTimelineItem(-1, 0)
	if !m.streamingItems[0].Expanded {
		t.Fatal("click did not expand the streaming item")
	}

	m.setAllTimelineExpanded(true)
	if !m.messages[0].items[0].Expanded || !m.messages[0].items[1].Expanded || !m.streamingItems[0].Expanded {
		t.Fatal("expand-all did not update every item")
	}
	if !m.messages[0].groupExpanded || !m.streamingGroupExpanded {
		t.Fatal("expand-all did not update every group")
	}
	if !m.anyTimelineExpanded() {
		t.Fatal("anyTimelineExpanded should be true")
	}
}

func TestToggleTimelineGroup(t *testing.T) {
	m := &model{
		messages: []chatMessage_{{
			role:  "bot",
			items: []timelineItem{{Kind: "thinking", Text: "think"}},
		}},
		streamingItems: []timelineItem{{Kind: "tool", ToolName: "bash"}},
	}

	m.toggleTimelineGroup(0)
	if !m.messages[0].groupExpanded {
		t.Fatal("click did not expand the message group")
	}
	m.toggleTimelineGroup(0)
	if m.messages[0].groupExpanded {
		t.Fatal("second click did not collapse the message group")
	}

	m.toggleTimelineGroup(-1)
	if !m.streamingGroupExpanded {
		t.Fatal("click did not expand the streaming group")
	}
	if !m.anyTimelineExpanded() {
		t.Fatal("anyTimelineExpanded should see the open streaming group")
	}

	m.setAllTimelineExpanded(false)
	if m.messages[0].groupExpanded || m.streamingGroupExpanded || m.messages[0].items[0].Expanded || m.streamingItems[0].Expanded {
		t.Fatal("collapse-all did not close every group and item")
	}
	if m.anyTimelineExpanded() {
		t.Fatal("anyTimelineExpanded should be false after collapse-all")
	}
}

func TestRenderAgentTimelineGroupsIntermediateItems(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(80, 100)
	items := []timelineItem{
		{Kind: "thinking", Text: "inspect the directory"},
		{Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`, ToolResult: "file1"},
		{Kind: "text", Text: "intermediate note"},
		{Kind: "thinking", Text: "second thought"},
		{Kind: "text", Text: "final answer"},
	}

	collapsed, spans, group := m.renderAgentTimeline(items, timelineRenderOptions{})
	if !strings.Contains(collapsed, "Agent timeline") || !strings.Contains(collapsed, "4 steps") {
		t.Fatalf("collapsed group header missing: %q", collapsed)
	}
	for _, hidden := range []string{"inspect the directory", "tool: bash", "intermediate note", "second thought"} {
		if strings.Contains(collapsed, hidden) {
			t.Fatalf("collapsed group should hide %q:\n%s", hidden, collapsed)
		}
	}
	if !strings.Contains(collapsed, "final answer") {
		t.Fatalf("final answer must stay visible:\n%s", collapsed)
	}
	if len(spans) != len(items) {
		t.Fatalf("spans = %d, want %d", len(spans), len(items))
	}
	for i, span := range spans {
		if span.End > span.Start {
			t.Fatalf("hidden item %d should have no click span: %+v", i, span)
		}
	}
	if group.End <= group.Start {
		t.Fatalf("group header should be clickable: %+v", group)
	}

	expanded, spans, group := m.renderAgentTimeline(items, timelineRenderOptions{GroupExpanded: true})
	for _, want := range []string{"▾ Agent timeline", "inspect the directory", "tool: bash", "intermediate note", "second thought", "final answer"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded group is missing %q:\n%s", want, expanded)
		}
	}
	for i := 0; i < len(items)-1; i++ {
		if spans[i].End <= spans[i].Start {
			t.Fatalf("expanded item %d should be clickable: %+v", i, spans[i])
		}
	}
	if spans[len(items)-1].End > spans[len(items)-1].Start {
		t.Fatalf("final answer should not be a timeline target: %+v", spans[len(items)-1])
	}
	if group.End <= group.Start {
		t.Fatalf("expanded group header should be clickable: %+v", group)
	}
}

func TestRenderAgentTimelineShowsDuration(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(80, 100)
	items := []timelineItem{
		{Kind: "tool", ToolName: "bash"},
		{Kind: "text", Text: "done"},
	}

	out, _, _ := m.renderAgentTimeline(items, timelineRenderOptions{Duration: 12400 * time.Millisecond})
	if !strings.Contains(out, "Completed in 12.4s") {
		t.Fatalf("completion duration missing:\n%s", out)
	}

	out, _, _ = m.renderAgentTimeline(items, timelineRenderOptions{Duration: 12400 * time.Millisecond, Interrupted: true})
	if !strings.Contains(out, "Interrupted after 12.4s") {
		t.Fatalf("interruption duration missing:\n%s", out)
	}
}

func TestClickTargetsMatchRenderedItemLines(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(80, 100)
	m.messages = []chatMessage_{
		{role: "user", text: "hello"},
		{
			role:          "bot",
			groupExpanded: true,
			items: []timelineItem{
				{Kind: "thinking", Text: "inspect the directory"},
				{Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`, ToolResult: "file1"},
				{Kind: "text", Text: "Done."},
			},
		},
	}

	m.updateViewportContent(false)
	if len(m.clickTargets) != 3 {
		t.Fatalf("click targets = %d, want 3: %+v", len(m.clickTargets), m.clickTargets)
	}
	lines := strings.Split(m.viewport.View(), "\n")
	wants := []struct {
		label string
		group bool
		item  int
	}{
		{"Agent timeline", true, 0},
		{"Thinking", false, 0},
		{"tool:", false, 1},
	}
	for i, want := range wants {
		target := m.clickTargets[i]
		if target.MessageIndex != 1 || target.Group != want.group || (!want.group && target.ItemIndex != want.item) {
			t.Fatalf("click target %d has wrong indices: %+v", i, target)
		}
		if target.StartLine >= len(lines) || !strings.Contains(lines[target.StartLine], want.label) {
			t.Fatalf("click target %d points at wrong line; want %q, got %q (target=%+v)", i, want.label, lines[target.StartLine], target)
		}
	}
	if !strings.Contains(m.viewport.View(), "Done.") {
		t.Fatal("final answer should stay visible when the group is expanded")
	}

	// The second render uses the cached view; the group header must stay clickable.
	m.updateViewportContent(false)
	if len(m.clickTargets) != 3 || !m.clickTargets[0].Group {
		t.Fatalf("cached click targets = %+v, want 3 with a group target", m.clickTargets)
	}
}

func TestStreamingShowsElapsedCounter(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(80, 100)
	m.state = stateWaiting
	m.streamStartedAt = time.Now().Add(-3 * time.Second)
	m.messages = []chatMessage_{{role: "user", text: "hi"}}

	m.updateViewportContent(false)
	if view := sanitizeForTUI(m.viewport.View()); !strings.Contains(view, "Myaaw is thinking... 3s") {
		t.Fatalf("thinking state is missing the elapsed counter:\n%s", view)
	}

	m.streamingItems = []timelineItem{
		{Kind: "thinking", Text: "working"},
		{Kind: "text", Text: "partial"},
	}
	m.streamingGroupExpanded = true
	m.updateViewportContent(false)
	if view := sanitizeForTUI(m.viewport.View()); !strings.Contains(view, "working 3s") {
		t.Fatalf("streaming title is missing the elapsed counter:\n%s", view)
	}
}

func TestItemMessageShowsDurationOnce(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(80, 100)
	m.messages = []chatMessage_{
		{
			role: "bot",
			items: []timelineItem{
				{Kind: "tool", ToolName: "bash"},
				{Kind: "text", Text: "done"},
			},
			duration: 3200 * time.Millisecond,
		},
	}

	m.updateViewportContent(false)
	if view := sanitizeForTUI(m.viewport.View()); strings.Count(view, "Completed in 3.2s") != 1 {
		t.Fatalf("duration footer should appear exactly once:\n%s", view)
	}
}

func TestPlainMessageShowsDuration(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(80, 100)
	m.messages = []chatMessage_{{
		role:         "bot",
		text:         "hello",
		renderedText: "hello",
		duration:     3200 * time.Millisecond,
	}}

	m.updateViewportContent(false)
	if view := sanitizeForTUI(m.viewport.View()); !strings.Contains(view, "Completed in 3.2s") {
		t.Fatalf("plain message is missing its duration:\n%s", view)
	}
}

func TestClickTargetsMatchStreamingLines(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(80, 100)
	m.state = stateWaiting
	m.streamStartedAt = time.Now()
	m.streamingGroupExpanded = true
	m.messages = []chatMessage_{{role: "user", text: "hello"}}
	m.streamingItems = []timelineItem{
		{Kind: "thinking", Text: "inspect the directory"},
		{Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`, ToolResult: "file1"},
		{Kind: "text", Text: "Done."},
	}

	m.updateViewportContent(false)
	if len(m.clickTargets) != 3 {
		t.Fatalf("click targets = %d, want 3: %+v", len(m.clickTargets), m.clickTargets)
	}
	lines := strings.Split(m.viewport.View(), "\n")
	wants := []struct {
		label string
		group bool
		item  int
	}{
		{"Agent timeline", true, 0},
		{"Thinking", false, 0},
		{"tool:", false, 1},
	}
	for i, want := range wants {
		target := m.clickTargets[i]
		if target.MessageIndex != -1 || target.Group != want.group || (!want.group && target.ItemIndex != want.item) {
			t.Fatalf("streaming click target %d has wrong indices: %+v", i, target)
		}
		if target.StartLine >= len(lines) || !strings.Contains(lines[target.StartLine], want.label) {
			t.Fatalf("streaming click target %d points at wrong line; want %q, got %q (target=%+v)", i, want.label, lines[target.StartLine], target)
		}
	}
}

func TestClickTargetsAccountForWrappedToolLines(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(52, 100)
	longArgs := `{"command":"` + strings.Repeat("a", 120) + `"}`
	longResult := strings.Repeat("result ", 20)
	longName := "toolone" + strings.Repeat("x", 60)
	m.messages = []chatMessage_{
		{role: "user", text: "hello"},
		{
			role:          "bot",
			groupExpanded: true,
			items: []timelineItem{
				{Kind: "tool", ToolName: longName, ToolArgs: longArgs, ToolResult: longResult},
				{Kind: "tool", ToolName: "tooltwo", ToolArgs: longArgs, ToolResult: longResult},
				{Kind: "thinking", Text: "need more detail"},
			},
		},
	}

	m.updateViewportContent(false)
	if len(m.clickTargets) != 4 {
		t.Fatalf("click targets = %d, want 4: %+v", len(m.clickTargets), m.clickTargets)
	}
	lines := strings.Split(m.viewport.View(), "\n")
	for i, want := range []string{"Agent timeline", "▸ tool:", "tooltwo", "Thinking"} {
		target := m.clickTargets[i]
		if target.StartLine >= len(lines) || !strings.Contains(lines[target.StartLine], want) {
			t.Fatalf("wrapped click target %d points at wrong line; want %q, got %q (target=%+v, lines=%v)", i, want, lines[target.StartLine], target, lines)
		}
	}
	if m.clickTargets[1].EndLine-m.clickTargets[1].StartLine < 2 {
		t.Fatalf("wrapped tool item should span more than one display row: %+v", m.clickTargets[1])
	}
}

func TestCollapsedToolHeaderFitsTimelineWidth(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(72, 40)
	longArgs := `{"command":"` + strings.Repeat("x", 300) + `"}`
	items := []timelineItem{
		{Kind: "tool", ToolName: "bash", ToolArgs: longArgs, ToolResult: strings.Repeat("y", 300)},
		{Kind: "thinking", Text: "next"},
	}

	out, spans := m.renderTimeline(items, false)
	header := strings.Split(out, "\n")[0]
	if !strings.Contains(header, "tool: bash") {
		t.Fatalf("unexpected tool header: %q", header)
	}
	if width := utf8.RuneCountInString(sanitizeForTUI(header)); width > m.viewport.Width-8 {
		t.Fatalf("collapsed tool header is %d cells wide, timeline is %d: %q", width, m.viewport.Width-8, header)
	}
	if spans[1].Start != 2 {
		t.Fatalf("second item should start after the header and separator, got %+v", spans)
	}
}

func TestInterruptLocksInputUntilSaveAck(t *testing.T) {
	m := model{
		viewport:        viewport.New(80, 100),
		state:           stateWaiting,
		streamStartedAt: time.Now().Add(-2 * time.Second),
		streamingItems:  []timelineItem{{Kind: "thinking", Text: "working"}},
		cancelFunc:      func() {},
		stopAck:         make(chan error, 1),
		messages:        []chatMessage_{{role: "user", text: "hi"}},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.state != stateStopping {
		t.Fatalf("state = %v, want stateStopping", m.state)
	}
	if cmd == nil {
		t.Fatal("interrupt should wait for the persistence ack")
	}
	if len(m.messages) != 2 || !m.messages[1].interrupted {
		t.Fatalf("interrupted message was not appended: %+v", m.messages)
	}
	if m.interruptedIndex != 1 {
		t.Fatalf("interrupted index = %d, want 1", m.interruptedIndex)
	}

	m.stopAck <- nil
	updated, _ = m.Update(stopAckMsg{err: nil})
	m = updated.(model)
	if m.state != stateInput {
		t.Fatalf("state after ack = %v, want stateInput", m.state)
	}
	if len(m.messages) != 2 {
		t.Fatalf("ack should not add messages: %+v", m.messages)
	}
	if m.interruptedIndex != -1 || m.stopAck != nil {
		t.Fatalf("interrupt state was not reset: index=%d ack=%v", m.interruptedIndex, m.stopAck)
	}
}

func TestInterruptWarnsWhenContextCannotBeSaved(t *testing.T) {
	m := model{
		viewport:        viewport.New(80, 100),
		state:           stateStopping,
		streamStartedAt: time.Now().Add(-2 * time.Second),
		stopAck:         make(chan error, 1),
		messages: []chatMessage_{
			{role: "user", text: "hi"},
			{role: "bot", text: "🛑 *(response interrupted)*", renderedText: "interrupted", interrupted: true},
		},
		interruptedIndex: 1,
	}

	updated, _ := m.Update(stopAckMsg{err: errors.New("database unavailable")})
	m = updated.(model)
	if m.state != stateInput {
		t.Fatalf("state = %v, want stateInput", m.state)
	}
	last := m.messages[len(m.messages)-1]
	if !strings.Contains(last.text, "may not be saved") || !strings.Contains(last.text, "database unavailable") {
		t.Fatalf("missing persistence warning: %+v", last)
	}
}

func TestStoppedRunReplacesInterruptedMessage(t *testing.T) {
	m := model{
		viewport:         viewport.New(80, 100),
		state:            stateStopping,
		streamStartedAt:  time.Now().Add(-2 * time.Second),
		stopAck:          make(chan error, 1),
		interruptedIndex: 1,
		messages: []chatMessage_{
			{role: "user", text: "hi"},
			{role: "bot", items: []timelineItem{{Kind: "text", Text: "🛑 *(interrupted)*"}}, interrupted: true},
		},
	}

	updated, _ := m.Update(nextChunkMsg{done: true, finalText: "real answer"})
	m = updated.(model)
	if m.state != stateInput {
		t.Fatalf("state = %v, want stateInput", m.state)
	}
	if len(m.messages) != 2 {
		t.Fatalf("completed response should replace the interrupted marker, messages=%+v", m.messages)
	}
	done := m.messages[1]
	if done.interrupted || len(done.items) == 0 || done.items[0].Text != "real answer" {
		t.Fatalf("interrupted message was not replaced: %+v", done)
	}
	if done.duration <= 0 {
		t.Fatalf("completed response is missing its duration: %+v", done)
	}
}

func TestMouseEscapeDetection(t *testing.T) {
	cases := map[string]bool{
		"[<65;42;16M":   true,
		"<65;42;16M":    true,
		"65;42;16M":     true,
		"\x1b[<0;10;5M": true,
		"hello":         false,
		"16M":           false,
		"[normal text]": false,
	}
	for input, want := range cases {
		if got := looksLikeMouseEscape(input); got != want {
			t.Errorf("looksLikeMouseEscape(%q) = %v, want %v", input, got, want)
		}
	}

	cleaned := mouseFragmentPattern.ReplaceAllString("[<65;42;16M<65;42;16Mhello", "")
	if cleaned != "hello" {
		t.Fatalf("mouse fragment sanitizer produced %q", cleaned)
	}
}

func TestTimelineFromTrace(t *testing.T) {
	trace := []provider.ReactStep{
		{Thought: "need to list files", Action: "bash", ActionInput: `{"command":"ls"}`, Observation: "file1", StepGroup: 1},
		{Thought: "need to read the file", Action: "bash", ActionInput: `{"command":"cat file1"}`, Observation: "Error: missing", StepGroup: 2},
	}

	items := timelineFromTrace(trace, "final answer")
	if len(items) != 5 {
		t.Fatalf("timeline items = %d, want 5: %+v", len(items), items)
	}
	if items[0].Kind != "thinking" || items[1].Kind != "tool" || items[2].Kind != "thinking" || items[3].Kind != "tool" || items[4].Kind != "text" {
		t.Fatalf("unexpected timeline order: %+v", items)
	}
	if items[3].IsError != true || !strings.Contains(items[3].ToolResult, "Error") {
		t.Fatalf("failed tool result was not marked as an error: %+v", items[3])
	}
	if items[4].Text != "final answer" {
		t.Fatalf("final answer missing: %+v", items[4])
	}
}

func TestConvertProviderMessagesBuildsTimeline(t *testing.T) {
	messages := []provider.Message{
		{Role: "user", Content: "hello"},
		{
			Role:    "assistant",
			Thought: "I should inspect the directory",
			ToolCalls: []provider.ToolCall{{
				ID:   "c1",
				Type: "function",
				Function: provider.FunctionCall{
					Name:      "bash",
					Arguments: `{"command":"ls"}`,
				},
			}},
		},
		{Role: "tool", ToolCallID: "c1", Name: "bash", Content: "file1\nfile2"},
		{Role: "assistant", Thought: "The directory is empty", Content: "Done."},
	}

	converted := convertProviderMessages(messages)
	if len(converted) != 2 {
		t.Fatalf("converted messages = %d, want 2", len(converted))
	}
	if converted[0].role != "user" || converted[0].text != "hello" {
		t.Fatalf("unexpected user message: %+v", converted[0])
	}

	bot := converted[1]
	if bot.role != "bot" || len(bot.items) != 4 {
		t.Fatalf("unexpected bot timeline: %+v", bot)
	}
	if bot.items[0].Kind != "thinking" || !strings.Contains(bot.items[0].Text, "inspect") {
		t.Fatalf("first item is not the initial thinking: %+v", bot.items[0])
	}
	if bot.items[1].Kind != "tool" || bot.items[1].ToolName != "bash" || bot.items[1].Running {
		t.Fatalf("second item is not the finalized tool call: %+v", bot.items[1])
	}
	if !strings.Contains(bot.items[1].ToolResult, "file1") {
		t.Fatalf("tool result missing: %+v", bot.items[1])
	}
	if bot.items[2].Kind != "thinking" || !strings.Contains(bot.items[2].Text, "empty") {
		t.Fatalf("third item is not the final thinking: %+v", bot.items[2])
	}
	if bot.items[3].Kind != "text" || bot.items[3].Text != "Done." {
		t.Fatalf("fourth item is not the final text: %+v", bot.items[3])
	}
}

func TestConvertProviderMessagesMigratesLegacyTrace(t *testing.T) {
	messages := []provider.Message{
		{Role: "user", Content: "hello"},
		{
			Role:    "assistant",
			Content: "final answer",
			Thought: "final thought",
			Trace: []provider.ReactStep{{
				Thought:     "legacy thought",
				Action:      "bash",
				ActionInput: `{"command":"ls"}`,
				Observation: "legacy output",
				StepGroup:   1,
			}},
		},
	}

	converted := convertProviderMessages(messages)
	if len(converted) != 2 {
		t.Fatalf("converted messages = %d, want 2", len(converted))
	}
	bot := converted[1]
	if len(bot.items) != 4 {
		t.Fatalf("legacy timeline items = %d, want 4: %+v", len(bot.items), bot.items)
	}
	if bot.items[0].Kind != "thinking" || bot.items[0].Text != "legacy thought" {
		t.Fatalf("legacy thinking missing: %+v", bot.items[0])
	}
	if bot.items[1].Kind != "tool" || bot.items[1].ToolName != "bash" || bot.items[1].ToolResult != "legacy output" {
		t.Fatalf("legacy tool call missing: %+v", bot.items[1])
	}
	if bot.items[2].Kind != "thinking" || bot.items[2].Text != "final thought" {
		t.Fatalf("final thinking missing: %+v", bot.items[2])
	}
	if bot.items[3].Kind != "text" || bot.items[3].Text != "final answer" {
		t.Fatalf("final answer missing: %+v", bot.items[3])
	}
}

func TestRenderAgentTimelineCountsLoopIterations(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(80, 100)
	items := []timelineItem{
		{Kind: "thinking", Text: "inspect"},
		{Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`, ToolResult: "file1"},
		{Kind: "thinking", Text: "look again"},
		{Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"pwd"}`, ToolResult: "/tmp"},
		{Kind: "thinking", Text: "answer"},
		{Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"date"}`, ToolResult: "now"},
		{Kind: "text", Text: "final answer"},
	}

	// Six activity items inside the group, but four loop iterations.
	collapsed, _, _ := m.renderAgentTimeline(items, timelineRenderOptions{Steps: 4})
	if !strings.Contains(collapsed, "4 steps") {
		t.Fatalf("header should count steps, got %q", collapsed)
	}
	if strings.Contains(collapsed, "6 steps") {
		t.Fatalf("header counted activity items instead of steps: %q", collapsed)
	}

	// History without a step count falls back to the visible item count.
	fallback, _, _ := m.renderAgentTimeline(items, timelineRenderOptions{})
	if !strings.Contains(fallback, "6 steps") {
		t.Fatalf("fallback header = %q", fallback)
	}
}

func TestRenderTimelineSeparatesSteps(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(80, 100)
	items := []timelineItem{
		{Kind: "thinking", Text: "first thought", Step: 1},
		{Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`, ToolResult: "a", Step: 1},
		{Kind: "thinking", Text: "second thought", Step: 2},
		{Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"pwd"}`, ToolResult: "/tmp", Step: 2},
		{Kind: "thinking", Text: "third thought", Step: 3},
		{Kind: "text", Text: "final answer", Step: 3},
	}

	expanded, _, _ := m.renderAgentTimeline(items, timelineRenderOptions{GroupExpanded: true, Steps: 3})
	for step := 1; step <= 3; step++ {
		if !strings.Contains(expanded, fmt.Sprintf("── step %d ──", step)) {
			t.Fatalf("missing label for step %d:\n%s", step, expanded)
		}
	}

	collapsed, _, _ := m.renderAgentTimeline(items, timelineRenderOptions{Steps: 3})
	if strings.Contains(collapsed, "── step") {
		t.Fatalf("collapsed timeline hides the body:\n%s", collapsed)
	}

	// Items without step information render without separators.
	legacy := []timelineItem{
		{Kind: "thinking", Text: "a"},
		{Kind: "tool", ToolName: "bash", ToolArgs: `{}`, ToolResult: "b"},
	}
	out, _ := m.renderTimeline(legacy, false)
	if strings.Contains(out, "── step") {
		t.Fatalf("legacy items must not get separators:\n%s", out)
	}
}

func TestRenderAgentTimelineLabelsTheFinalStep(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(80, 100)
	items := []timelineItem{
		{Kind: "thinking", Text: "first", Step: 1},
		{Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`, ToolResult: "a", Step: 1},
		{Kind: "thinking", Text: "second", Step: 2},
		{Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"pwd"}`, ToolResult: "/tmp", Step: 2},
		{Kind: "thinking", Text: "third", Step: 3},
		{Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"date"}`, ToolResult: "now", Step: 3},
		{Kind: "thinking", Text: "final thought", Step: 4},
		{Kind: "text", Text: "final answer", Step: 4},
	}

	expanded, _, _ := m.renderAgentTimeline(items, timelineRenderOptions{GroupExpanded: true, Steps: 4})
	for step := 1; step <= 4; step++ {
		if !strings.Contains(expanded, fmt.Sprintf("── step %d ──", step)) {
			t.Fatalf("missing label for step %d:\n%s", step, expanded)
		}
	}
	if strings.Count(expanded, "── step 1 ──") != 1 {
		t.Fatalf("step 1 must be labelled exactly once:\n%s", expanded)
	}

	collapsed, _, _ := m.renderAgentTimeline(items, timelineRenderOptions{Steps: 4})
	if strings.Contains(collapsed, "── step 4 ──") {
		t.Fatalf("collapsed view must not draw the final separator:\n%s", collapsed)
	}
}

func TestRenderAgentTimelineLabelsASingleStepOnce(t *testing.T) {
	m := &model{}
	m.viewport = viewport.New(80, 100)
	// Thinking and the answer both belong to step 1.
	items := []timelineItem{
		{Kind: "thinking", Text: "simple factual answer", Step: 1},
		{Kind: "text", Text: "Indonesia merdeka 17 Agustus 1945", Step: 1},
	}

	expanded, _, _ := m.renderAgentTimeline(items, timelineRenderOptions{GroupExpanded: true, Steps: 1})
	if got := strings.Count(expanded, "── step 1 ──"); got != 1 {
		t.Fatalf("step 1 label appears %d times, want 1:\n%s", got, expanded)
	}
	if strings.Contains(expanded, "── step 2 ──") {
		t.Fatalf("there is no step 2:\n%s", expanded)
	}
}
