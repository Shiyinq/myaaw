package telegram

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

type fakeSend struct {
	chatID  int
	replyTo int
	text    string
	mode    string
}

type fakeEdit struct {
	chatID    int
	messageID int
	text      string
	mode      string
}

type fakeTransport struct {
	nextID int
	sends  []fakeSend
	edits  []fakeEdit
}

func (f *fakeTransport) Send(chatID, replyTo int, text, parseMode string) (int, error) {
	f.nextID++
	f.sends = append(f.sends, fakeSend{chatID: chatID, replyTo: replyTo, text: text, mode: parseMode})
	return f.nextID, nil
}

func (f *fakeTransport) Edit(chatID, messageID int, text, parseMode string) error {
	f.edits = append(f.edits, fakeEdit{chatID: chatID, messageID: messageID, text: text, mode: parseMode})
	return nil
}

func newTestTimeline(transport telegramTransport, placeholderID int) *telegramTimeline {
	tl := newTelegramTimeline(transport, 1, 10, placeholderID)
	tl.editThreshold = 0
	tl.editInterval = 0
	tl.now = func() time.Time { return time.Unix(100, 0) }
	return tl
}

func TestTelegramTimelineStreamsEachItemAsBubble(t *testing.T) {
	transport := &fakeTransport{}
	tl := newTestTimeline(transport, 0)

	tl.handleChunk(channel.StreamChunk{Kind: "thought", Thought: "need to inspect"})
	tl.handleChunk(channel.StreamChunk{
		Kind:          "tool",
		ToolCallID:    "c1",
		ToolName:      "bash",
		ToolArguments: `{"command":"ls"}`,
	})
	tl.handleChunk(channel.StreamChunk{Kind: "tool_result", ToolCallID: "c1", ToolResultText: "file1"})
	tl.handleChunk(channel.StreamChunk{Kind: "text", Text: "The directory "})
	tl.handleChunk(channel.StreamChunk{Kind: "text", Text: "contains file1."})
	tl.handleChunk(channel.StreamChunk{Kind: "final", Text: "The directory contains file1."})
	tl.finish()

	if len(transport.sends) != 3 {
		t.Fatalf("sends = %d, want 3: %+v", len(transport.sends), transport.sends)
	}
	if transport.sends[0].replyTo != 10 {
		t.Fatalf("first bubble should reply to the user message: %+v", transport.sends[0])
	}
	if transport.sends[1].replyTo != 0 || transport.sends[2].replyTo != 0 {
		t.Fatalf("later bubbles should not reply to the user message: %+v", transport.sends)
	}
	if !strings.Contains(transport.sends[0].text, "Reasoning") {
		t.Fatalf("first bubble is not a thinking quote: %q", transport.sends[0].text)
	}
	if !strings.Contains(transport.sends[1].text, "bash") ||
		!strings.Contains(transport.sends[1].text, "[running]") ||
		!strings.Contains(transport.sends[1].text, "<blockquote expandable>") {
		t.Fatalf("tool bubble was not rendered as a quote: %q", transport.sends[1].text)
	}
	if !strings.Contains(transport.sends[2].text, "The directory") {
		t.Fatalf("text bubble missing streamed content: %q", transport.sends[2].text)
	}

	foundResult := false
	foundFinal := false
	for _, edit := range transport.edits {
		if strings.Contains(edit.text, "[ok]") && strings.Contains(edit.text, "file1") {
			foundResult = true
		}
		if strings.Contains(edit.text, "The directory contains file1.") {
			foundFinal = true
		}
	}
	if !foundResult {
		t.Fatalf("tool result edit missing: %+v", transport.edits)
	}
	if !foundFinal {
		t.Fatalf("final text edit missing: %+v", transport.edits)
	}
}

func TestTelegramTimelineKeepsConsecutiveToolCallsSeparate(t *testing.T) {
	transport := &fakeTransport{}
	tl := newTestTimeline(transport, 0)

	tl.handleChunk(channel.StreamChunk{Kind: "thought", Thought: "check both"})
	tl.handleChunk(channel.StreamChunk{Kind: "tool", ToolCallID: "c1", ToolName: "bash", ToolArguments: `{"command":"ls"}`})
	tl.handleChunk(channel.StreamChunk{Kind: "tool_result", ToolCallID: "c1", ToolResultText: "file1"})
	tl.handleChunk(channel.StreamChunk{Kind: "thought", Thought: "now read it"})
	tl.handleChunk(channel.StreamChunk{Kind: "tool", ToolCallID: "c2", ToolName: "filesystem", ToolArguments: `{"path":"file1"}`})
	tl.handleChunk(channel.StreamChunk{Kind: "tool_result", ToolCallID: "c2", ToolResultText: "Error: missing"})
	tl.handleChunk(channel.StreamChunk{Kind: "final", Text: "done"})
	tl.finish()

	if len(transport.sends) != 5 {
		t.Fatalf("sends = %d, want 5 (thinking, tool, thinking, tool, answer): %+v", len(transport.sends), transport.sends)
	}
	wants := []string{"Reasoning", "bash", "Reasoning", "filesystem", "done"}
	for i, want := range wants {
		if !strings.Contains(transport.sends[i].text, want) {
			t.Fatalf("bubble %d missing %q: %q", i, want, transport.sends[i].text)
		}
	}
	foundError := false
	for _, edit := range transport.edits {
		if strings.Contains(edit.text, "[error]") && strings.Contains(edit.text, "missing") {
			foundError = true
		}
	}
	if !foundError {
		t.Fatalf("error tool result was not rendered: %+v", transport.edits)
	}
}

func TestTelegramTimelineSeparatesIntermediateText(t *testing.T) {
	transport := &fakeTransport{}
	tl := newTestTimeline(transport, 0)

	tl.handleChunk(channel.StreamChunk{Kind: "text", Text: "Let me check."})
	tl.handleChunk(channel.StreamChunk{Kind: "tool", ToolCallID: "c1", ToolName: "bash", ToolArguments: `{"command":"ls"}`})
	tl.handleChunk(channel.StreamChunk{Kind: "tool_result", ToolCallID: "c1", ToolResultText: "file1"})
	tl.handleChunk(channel.StreamChunk{Kind: "text", Text: "It is here."})
	tl.handleChunk(channel.StreamChunk{Kind: "final", Text: "It is here."})
	tl.finish()

	textBubbles := 0
	for _, send := range transport.sends {
		if strings.Contains(send.text, "Let me check.") || strings.Contains(send.text, "It is here.") {
			textBubbles++
		}
	}
	if textBubbles != 2 {
		t.Fatalf("intermediate and final text should use different bubbles: %+v", transport.sends)
	}
}

func TestTelegramTimelineClaimsPlaceholder(t *testing.T) {
	transport := &fakeTransport{}
	tl := newTestTimeline(transport, 99)

	tl.handleChunk(channel.StreamChunk{Kind: "thought", Thought: "first thought"})
	tl.handleChunk(channel.StreamChunk{Kind: "final", Text: "answer"})
	tl.finish()

	if len(transport.sends) != 1 {
		t.Fatalf("sends = %d, want 1 final bubble: %+v", len(transport.sends), transport.sends)
	}
	if len(transport.edits) == 0 || transport.edits[0].messageID != 99 {
		t.Fatalf("placeholder was not reused for the first bubble: %+v", transport.edits)
	}
	if !strings.Contains(transport.sends[0].text, "answer") {
		t.Fatalf("final answer should be a new bubble after the thinking placeholder: %+v", transport.sends[0])
	}
}

func TestBuildTelegramTimelineFromTrace(t *testing.T) {
	trace := []provider.ReactStep{
		{Thought: "first thought", Action: "bash", ActionInput: `{"command":"ls"}`, Observation: "file1"},
		{Thought: "second thought", Action: "filesystem", ActionInput: `{"path":"file1"}`, Observation: "Error: missing"},
	}

	items := buildTelegramTimeline(trace, "final answer", "second thought")
	if len(items) != 5 {
		t.Fatalf("items = %d, want 5: %+v", len(items), items)
	}
	if items[0].Kind != "thinking" || items[1].Kind != "tool" || items[2].Kind != "thinking" || items[3].Kind != "tool" || items[4].Kind != "text" {
		t.Fatalf("unexpected item order: %+v", items)
	}
	if items[3].IsError != true {
		t.Fatalf("failed tool result was not marked as an error: %+v", items[3])
	}
	if !items[4].Final || items[4].Text != "final answer" {
		t.Fatalf("final answer item is wrong: %+v", items[4])
	}
}

func TestBuildTelegramTimelineFallsBackToThought(t *testing.T) {
	items := buildTelegramTimeline(nil, "answer", "a thought")
	if len(items) != 2 || items[0].Kind != "thinking" || items[1].Kind != "text" {
		t.Fatalf("unexpected fallback timeline: %+v", items)
	}
}

func TestTelegramTimelineWatermarksAnswerWithoutFinalChunk(t *testing.T) {
	previous := config.WatermarkModel
	config.WatermarkModel = true
	defer func() { config.WatermarkModel = previous }()

	transport := &fakeTransport{}
	tl := newTestTimeline(transport, 0)
	tl.handleChunk(channel.StreamChunk{Kind: "text", Text: "command answer"})
	tl.finish()

	found := false
	for _, send := range transport.sends {
		if strings.Contains(send.text, "command answer") && strings.Contains(send.text, "🤖") {
			found = true
		}
	}
	for _, edit := range transport.edits {
		if strings.Contains(edit.text, "command answer") && strings.Contains(edit.text, "🤖") {
			found = true
		}
	}
	if !found {
		t.Fatalf("streamed answer without a final chunk was not watermarked: sends=%+v edits=%+v", transport.sends, transport.edits)
	}
}

func TestTelegramToolAndThinkingQuotes(t *testing.T) {
	tl := newTestTimeline(&fakeTransport{}, 0)

	thinking, mode := tl.render(&telegramTimelineItem{Kind: "thinking", Text: "reasoning text"}, false)
	if mode != "HTML" || !strings.Contains(thinking, "<blockquote expandable>") || !strings.Contains(thinking, "reasoning text") {
		t.Fatalf("thinking quote is malformed: mode=%q text=%q", mode, thinking)
	}

	running, mode := tl.render(&telegramTimelineItem{
		Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`, Running: true,
	}, false)
	if mode != "HTML" || !strings.Contains(running, "[running]") || !strings.Contains(running, "<blockquote expandable>") {
		t.Fatalf("running tool quote is malformed: mode=%q text=%q", mode, running)
	}

	ok, _ := tl.render(&telegramTimelineItem{
		Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`, ToolResult: "file1",
	}, true)
	if !strings.Contains(ok, "[ok]") || !strings.Contains(ok, "file1") {
		t.Fatalf("completed tool quote is malformed: %q", ok)
	}

	failed, _ := tl.render(&telegramTimelineItem{
		Kind: "tool", ToolName: "bash", ToolResult: "Error: missing", IsError: true,
	}, true)
	if !strings.Contains(failed, "[error]") || !strings.Contains(failed, "missing") {
		t.Fatalf("failed tool quote is malformed: %q", failed)
	}
}

func TestTelegramRenderedBubblesRespectLimit(t *testing.T) {
	tl := newTestTimeline(&fakeTransport{}, 0)

	huge := strings.Repeat("&<>\"' 😀 ", 1000)

	thinking, _ := tl.render(&telegramTimelineItem{Kind: "thinking", Text: huge}, false)
	assertTelegramLimit(t, "thinking", thinking)

	running, _ := tl.render(&telegramTimelineItem{
		Kind: "tool", ToolName: "bash", ToolArgs: huge, Running: true,
	}, false)
	assertTelegramLimit(t, "running tool", running)

	done, _ := tl.render(&telegramTimelineItem{
		Kind: "tool", ToolName: "bash", ToolArgs: huge, ToolResult: huge,
	}, true)
	assertTelegramLimit(t, "completed tool", done)
	if !strings.Contains(done, "… (truncated)") {
		t.Fatalf("oversized tool content was not marked as truncated")
	}

	preview, _ := tl.render(&telegramTimelineItem{Kind: "text", Text: huge}, false)
	assertTelegramLimit(t, "text preview", preview)
}

func TestTelegramWriteTextKeepsEveryBubbleUnderLimit(t *testing.T) {
	transport := &fakeTransport{}
	tl := newTestTimeline(transport, 0)
	item := &telegramTimelineItem{Kind: "text"}

	text := strings.Repeat("harga *naik* & _turun_ [x](https://example.com) 😀 ", 500)
	tl.writeText(item, text, true)

	if len(transport.sends) < 2 {
		t.Fatalf("long markdown should be split across bubbles, got %d sends", len(transport.sends))
	}
	for i, send := range transport.sends {
		assertTelegramLimit(t, fmt.Sprintf("send %d", i), send.text)
	}
	for i, edit := range transport.edits {
		assertTelegramLimit(t, fmt.Sprintf("edit %d", i), edit.text)
	}
}

func assertTelegramLimit(t *testing.T, label, text string) {
	t.Helper()
	if units := telegramUnits(text); units > telegramMaxLen {
		t.Fatalf("%s is %d UTF-16 units, limit is %d", label, units, telegramMaxLen)
	}
}

func TestSplitTelegramTextPreservesContent(t *testing.T) {
	text := strings.Repeat("baris panjang\n", 500)
	chunks := splitTelegramText(text, telegramTextChunk)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	if joined := strings.Join(chunks, ""); joined != text {
		t.Fatalf("chunking changed the content: got %d bytes, want %d", len(joined), len(text))
	}
	for _, chunk := range chunks {
		if runes := len([]rune(chunk)); runes > telegramTextChunk {
			t.Fatalf("chunk has %d runes, limit is %d", runes, telegramTextChunk)
		}
	}
}

func TestTelegramSendStreamUsesPlaceholderAndReturnsFinalText(t *testing.T) {
	transport := &fakeTransport{}
	adapter := &TelegramAdapter{transport: transport}
	msg := &channel.IncomingMessage{RawMeta: TelegramMeta{ChatID: 1, MessageID: 10}}

	out, err := adapter.SendStream(msg, func(onChunk func(channel.StreamChunk)) error {
		onChunk(channel.StreamChunk{Kind: "thought", Thought: "thinking"})
		onChunk(channel.StreamChunk{Kind: "final", Text: "answer"})
		return nil
	})
	if err != nil {
		t.Fatalf("SendStream returned error: %v", err)
	}
	if out == nil || out.Text != "answer" {
		t.Fatalf("unexpected outgoing message: %+v", out)
	}
	if len(transport.sends) != 2 {
		t.Fatalf("sends = %d, want placeholder + final answer: %+v", len(transport.sends), transport.sends)
	}
	if !strings.Contains(transport.sends[0].text, "Typing") {
		t.Fatalf("first message is not the typing placeholder: %+v", transport.sends[0])
	}
	if transport.sends[0].replyTo != 10 {
		t.Fatalf("placeholder should reply to the user message: %+v", transport.sends[0])
	}
	if len(transport.edits) == 0 || transport.edits[0].messageID != 1 {
		t.Fatalf("first timeline item did not claim the placeholder: %+v", transport.edits)
	}
}

func TestTelegramSendBuildsTimelineFromTrace(t *testing.T) {
	transport := &fakeTransport{}
	adapter := &TelegramAdapter{transport: transport}
	msg := &channel.IncomingMessage{RawMeta: TelegramMeta{ChatID: 1, MessageID: 10}}
	out := &channel.OutgoingMessage{
		Text: "final answer",
		Trace: []provider.ReactStep{
			{Thought: "check files", Action: "bash", ActionInput: `{"command":"ls"}`, Observation: "file1"},
		},
	}

	if err := adapter.Send(msg, out); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if len(transport.sends) != 3 {
		t.Fatalf("sends = %d, want 3 (thinking, tool, answer): %+v", len(transport.sends), transport.sends)
	}
	if transport.sends[0].replyTo != 10 {
		t.Fatalf("first bubble should reply to the user message: %+v", transport.sends[0])
	}
	if !strings.Contains(transport.sends[2].text, "final answer") {
		t.Fatalf("final answer bubble missing: %+v", transport.sends[2])
	}
}
