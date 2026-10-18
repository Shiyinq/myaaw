package discord

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

type fakeDiscordSend struct {
	channelID string
	replyTo   string
	content   string
	reply     bool
}

type fakeDiscordEdit struct {
	channelID string
	messageID string
	content   string
}

type fakeDiscordTransport struct {
	nextID int
	sends  []fakeDiscordSend
	edits  []fakeDiscordEdit
}

func (f *fakeDiscordTransport) nextMessageID() string {
	f.nextID++
	return "m" + strconv.Itoa(f.nextID)
}

func (f *fakeDiscordTransport) Send(channelID, content string) (string, error) {
	f.sends = append(f.sends, fakeDiscordSend{channelID: channelID, content: content})
	return f.nextMessageID(), nil
}

func (f *fakeDiscordTransport) SendReply(channelID, replyTo, content string) (string, error) {
	f.sends = append(f.sends, fakeDiscordSend{channelID: channelID, replyTo: replyTo, content: content, reply: true})
	return f.nextMessageID(), nil
}

func (f *fakeDiscordTransport) Edit(channelID, messageID, content string) error {
	f.edits = append(f.edits, fakeDiscordEdit{channelID: channelID, messageID: messageID, content: content})
	return nil
}

func newTestDiscordTimeline(transport discordTransport, placeholderID string) *discordTimeline {
	tl := newDiscordTimeline(transport, "chan", "msg", placeholderID)
	tl.editThreshold = 0
	tl.editInterval = 0
	tl.now = func() time.Time { return time.Unix(100, 0) }
	return tl
}

func TestDiscordTimelineStreamsEachItemAsBubble(t *testing.T) {
	transport := &fakeDiscordTransport{}
	tl := newTestDiscordTimeline(transport, "")

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
	if !transport.sends[0].reply || transport.sends[0].replyTo != "msg" {
		t.Fatalf("first bubble should reply to the user message: %+v", transport.sends[0])
	}
	if transport.sends[1].reply || transport.sends[2].reply {
		t.Fatalf("later bubbles should not reply: %+v", transport.sends)
	}
	if !strings.Contains(transport.sends[0].content, "Reasoning") ||
		!strings.Contains(transport.sends[0].content, ">>>") {
		t.Fatalf("thinking bubble is malformed: %q", transport.sends[0].content)
	}
	if !strings.Contains(transport.sends[1].content, "bash") ||
		!strings.Contains(transport.sends[1].content, "[running]") ||
		!strings.Contains(transport.sends[1].content, ">>>") {
		t.Fatalf("tool bubble is malformed: %q", transport.sends[1].content)
	}
	if !strings.Contains(transport.sends[2].content, "The directory") {
		t.Fatalf("text bubble missing streamed content: %q", transport.sends[2].content)
	}

	foundResult := false
	foundFinal := false
	for _, edit := range transport.edits {
		if strings.Contains(edit.content, "[ok]") && strings.Contains(edit.content, "file1") {
			foundResult = true
		}
		if strings.Contains(edit.content, "The directory contains file1.") {
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

func TestDiscordTimelineKeepsConsecutiveToolCallsSeparate(t *testing.T) {
	transport := &fakeDiscordTransport{}
	tl := newTestDiscordTimeline(transport, "")

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
		if !strings.Contains(transport.sends[i].content, want) {
			t.Fatalf("bubble %d missing %q: %q", i, want, transport.sends[i].content)
		}
	}
	foundError := false
	for _, edit := range transport.edits {
		if strings.Contains(edit.content, "[error]") && strings.Contains(edit.content, "missing") {
			foundError = true
		}
	}
	if !foundError {
		t.Fatalf("error tool result was not rendered: %+v", transport.edits)
	}
}

func TestDiscordTimelineSeparatesIntermediateText(t *testing.T) {
	transport := &fakeDiscordTransport{}
	tl := newTestDiscordTimeline(transport, "")

	tl.handleChunk(channel.StreamChunk{Kind: "text", Text: "Let me check."})
	tl.handleChunk(channel.StreamChunk{Kind: "tool", ToolCallID: "c1", ToolName: "bash", ToolArguments: `{"command":"ls"}`})
	tl.handleChunk(channel.StreamChunk{Kind: "tool_result", ToolCallID: "c1", ToolResultText: "file1"})
	tl.handleChunk(channel.StreamChunk{Kind: "text", Text: "It is here."})
	tl.handleChunk(channel.StreamChunk{Kind: "final", Text: "It is here."})
	tl.finish()

	textBubbles := 0
	for _, send := range transport.sends {
		if strings.Contains(send.content, "Let me check.") || strings.Contains(send.content, "It is here.") {
			textBubbles++
		}
	}
	if textBubbles != 2 {
		t.Fatalf("intermediate and final text should use different bubbles: %+v", transport.sends)
	}
}

func TestDiscordTimelineClaimsPlaceholder(t *testing.T) {
	transport := &fakeDiscordTransport{}
	tl := newTestDiscordTimeline(transport, "placeholder")

	tl.handleChunk(channel.StreamChunk{Kind: "thought", Thought: "first thought"})
	tl.handleChunk(channel.StreamChunk{Kind: "final", Text: "answer"})
	tl.finish()

	if len(transport.sends) != 1 {
		t.Fatalf("sends = %d, want 1 final bubble: %+v", len(transport.sends), transport.sends)
	}
	if len(transport.edits) == 0 || transport.edits[0].messageID != "placeholder" {
		t.Fatalf("placeholder was not reused for the first bubble: %+v", transport.edits)
	}
	if !strings.Contains(transport.sends[0].content, "answer") {
		t.Fatalf("final answer should be a new bubble after the thinking placeholder: %+v", transport.sends[0])
	}
}

func TestBuildDiscordTimelineFromTrace(t *testing.T) {
	trace := []provider.ReactStep{
		{Thought: "first thought", Action: "bash", ActionInput: `{"command":"ls"}`, Observation: "file1"},
		{Thought: "second thought", Action: "filesystem", ActionInput: `{"path":"file1"}`, Observation: "Error: missing"},
	}

	items := buildDiscordTimeline(trace, "final answer", "second thought")
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

func TestBuildDiscordTimelineFallsBackToThought(t *testing.T) {
	items := buildDiscordTimeline(nil, "answer", "a thought")
	if len(items) != 2 || items[0].Kind != "thinking" || items[1].Kind != "text" {
		t.Fatalf("unexpected fallback timeline: %+v", items)
	}
}

func TestDiscordToolAndThinkingQuotes(t *testing.T) {
	tl := newTestDiscordTimeline(&fakeDiscordTransport{}, "")

	thinking := tl.render(&discordTimelineItem{Kind: "thinking", Text: "reasoning text"})
	if !strings.Contains(thinking, "**💭 Reasoning...**") || !strings.Contains(thinking, ">>> reasoning text") {
		t.Fatalf("thinking quote is malformed: %q", thinking)
	}

	running := tl.render(&discordTimelineItem{
		Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`, Running: true,
	})
	if !strings.Contains(running, "[running]") || !strings.Contains(running, ">>>") || !strings.Contains(running, "Running...") {
		t.Fatalf("running tool quote is malformed: %q", running)
	}

	ok := tl.render(&discordTimelineItem{
		Kind: "tool", ToolName: "bash", ToolArgs: `{"command":"ls"}`, ToolResult: "file1",
	})
	if !strings.Contains(ok, "[ok]") || !strings.Contains(ok, "file1") {
		t.Fatalf("completed tool quote is malformed: %q", ok)
	}

	failed := tl.render(&discordTimelineItem{
		Kind: "tool", ToolName: "bash", ToolResult: "Error: missing", IsError: true,
	})
	if !strings.Contains(failed, "[error]") || !strings.Contains(failed, "missing") {
		t.Fatalf("failed tool quote is malformed: %q", failed)
	}
}

func TestDiscordRenderedBubblesRespectLimit(t *testing.T) {
	tl := newTestDiscordTimeline(&fakeDiscordTransport{}, "")
	huge := strings.Repeat("&<>\"' 😀 line\n", 1000)

	thinking := tl.render(&discordTimelineItem{Kind: "thinking", Text: huge})
	assertDiscordLimit(t, "thinking", thinking)

	tool := tl.render(&discordTimelineItem{
		Kind: "tool", ToolName: "bash", ToolArgs: huge, ToolResult: huge,
	})
	assertDiscordLimit(t, "tool", tool)
	if !strings.Contains(tool, "… (truncated)") {
		t.Fatalf("oversized tool content was not marked as truncated")
	}

	preview := tl.render(&discordTimelineItem{Kind: "text", Text: huge})
	assertDiscordLimit(t, "text preview", preview)
}

func TestDiscordWriteTextKeepsEveryBubbleUnderLimit(t *testing.T) {
	transport := &fakeDiscordTransport{}
	tl := newTestDiscordTimeline(transport, "")
	item := &discordTimelineItem{Kind: "text"}

	text := strings.Repeat("harga *naik* & _turun_ 😀 ", 500)
	tl.writeText(item, text, true)

	if len(transport.sends) < 2 {
		t.Fatalf("long response should be split across bubbles, got %d sends", len(transport.sends))
	}
	for i, send := range transport.sends {
		assertDiscordLimit(t, "send "+strconv.Itoa(i), send.content)
	}
	for i, edit := range transport.edits {
		assertDiscordLimit(t, "edit "+strconv.Itoa(i), edit.content)
	}
	if joined := strings.Join(func() []string {
		parts := make([]string, 0, len(transport.sends))
		for _, send := range transport.sends {
			parts = append(parts, send.content)
		}
		return parts
	}(), ""); joined != text {
		t.Fatalf("long response was truncated or reordered")
	}
}

func TestDiscordSendStreamUsesPlaceholderAndReturnsFinalText(t *testing.T) {
	transport := &fakeDiscordTransport{}
	adapter := &DiscordAdapter{transport: transport}
	msg := &channel.IncomingMessage{RawMeta: DiscordMeta{ChannelID: "chan", MessageID: "msg"}}

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
	if !strings.Contains(transport.sends[0].content, "Typing") || !transport.sends[0].reply {
		t.Fatalf("first message is not the typing placeholder reply: %+v", transport.sends[0])
	}
	if len(transport.edits) == 0 || transport.edits[0].messageID != "m1" {
		t.Fatalf("first timeline item did not claim the placeholder: %+v", transport.edits)
	}
}

func TestDiscordSendBuildsTimelineFromTrace(t *testing.T) {
	transport := &fakeDiscordTransport{}
	adapter := &DiscordAdapter{transport: transport}
	msg := &channel.IncomingMessage{RawMeta: DiscordMeta{ChannelID: "chan", MessageID: "msg"}}
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
	if !transport.sends[0].reply || transport.sends[0].replyTo != "msg" {
		t.Fatalf("first bubble should reply to the user message: %+v", transport.sends[0])
	}
	if !strings.Contains(transport.sends[2].content, "final answer") {
		t.Fatalf("final answer bubble missing: %+v", transport.sends[2])
	}
}

func TestDiscordTimelineWatermarksAnswerWithoutFinalChunk(t *testing.T) {
	previous := config.WatermarkModel
	config.WatermarkModel = true
	defer func() { config.WatermarkModel = previous }()

	transport := &fakeDiscordTransport{}
	tl := newTestDiscordTimeline(transport, "")
	tl.handleChunk(channel.StreamChunk{Kind: "text", Text: "command answer"})
	tl.finish()

	found := false
	for _, send := range transport.sends {
		if strings.Contains(send.content, "command answer") && strings.Contains(send.content, "🤖") {
			found = true
		}
	}
	for _, edit := range transport.edits {
		if strings.Contains(edit.content, "command answer") && strings.Contains(edit.content, "🤖") {
			found = true
		}
	}
	if !found {
		t.Fatalf("streamed answer without final chunk was not watermarked: sends=%+v edits=%+v", transport.sends, transport.edits)
	}
}

func TestSplitDiscordTextPreservesContent(t *testing.T) {
	text := strings.Repeat("baris panjang 😀\n", 500)
	chunks := splitDiscordText(text, discordTextChunk)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	if joined := strings.Join(chunks, ""); joined != text {
		t.Fatalf("chunking changed the content: got %d runes, want %d", len([]rune(joined)), len([]rune(text)))
	}
	for _, chunk := range chunks {
		assertDiscordLimit(t, "chunk", chunk)
	}
}

func assertDiscordLimit(t *testing.T, label, content string) {
	t.Helper()
	if units := discordUnits(content); units > discordMaxLen {
		t.Fatalf("%s is %d UTF-16 units, limit is %d", label, units, discordMaxLen)
	}
}
