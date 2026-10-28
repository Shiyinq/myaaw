package obs

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"myaaw/internal/provider"
)

type recorderFixture struct {
	store *Store
	rec   *Recorder
	base  time.Time
}

func newRecorderFixture(t *testing.T, cfg Config) recorderFixture {
	t.Helper()
	store := newTestStore(t, Options{})
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	cfg.Now = func() time.Time { return base }
	recorder := New(store, NewPricer(nil), cfg)
	t.Cleanup(func() { _ = recorder.Close() })
	return recorderFixture{store: store, rec: recorder, base: base}
}

func startRun(t *testing.T, fixture recorderFixture) string {
	t.Helper()
	runID := fixture.rec.Start(RunStart{
		Channel:        "telegram",
		UserID:         "42",
		ConversationID: "conv-1",
		Provider:       "deepseek",
		Model:          "deepseek-flash",
		Agent:          "default",
		UserMessage:    "halo",
		SystemPrompt:   "kamu kucing",
	})
	if runID == "" {
		t.Fatal("Start returned an empty run id")
	}
	return runID
}

func spansByKind(spans []Span) map[SpanKind][]Span {
	grouped := map[SpanKind][]Span{}
	for _, span := range spans {
		grouped[span.Kind] = append(grouped[span.Kind], span)
	}
	return grouped
}

func TestRecorderBuildsTraceTree(t *testing.T) {
	fixture := newRecorderFixture(t, Config{MaxContentChars: 4096, StoreContent: true})
	base := fixture.base
	runID := startRun(t, fixture)

	usage := provider.Usage{
		PromptTokens:      1_000_000,
		CachedInputTokens: 800_000,
		CompletionTokens:  100_000,
		TotalTokens:       1_100_000,
	}

	fixture.rec.OnEvent(Event{Type: EventStepStart, Time: base, StepIndex: 1})
	fixture.rec.OnEvent(Event{
		Type: EventLLMRequestStart, Time: base.Add(10 * time.Millisecond),
		StepIndex: 1, Model: "deepseek-flash",
		Messages: []provider.Message{
			{Role: "system", Content: "kamu kucing"},
			{Role: "user", Content: "halo"},
		},
	})
	fixture.rec.OnEvent(Event{Type: EventFirstToken, Time: base.Add(410 * time.Millisecond)})
	fixture.rec.OnEvent(Event{
		Type: EventLLMEnd, Time: base.Add(610 * time.Millisecond),
		Usage: usage, StopReason: provider.StopNormal,
		Output: "hai juga", Thought: "berpikir sebentar",
	})
	fixture.rec.OnEvent(Event{
		Type: EventToolStart, Time: base.Add(620 * time.Millisecond), StepIndex: 1,
		ToolCallID: "call_1", ToolName: "bash", ToolArguments: `{"cmd":"ls"}`,
	})
	fixture.rec.OnEvent(Event{
		Type: EventToolEnd, Time: base.Add(650 * time.Millisecond),
		ToolCallID: "call_1", ToolName: "bash", ToolOutput: "a.go", ToolDurationMs: 30,
	})
	fixture.rec.OnEvent(Event{Type: EventStepEnd, Time: base.Add(700 * time.Millisecond)})
	fixture.rec.OnEvent(Event{
		Type: EventRunEnd, Time: base.Add(800 * time.Millisecond),
		Usage: usage, StopReason: provider.StopNormal,
	})

	if err := fixture.rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	run, spans, err := fixture.store.GetRun(runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}

	if run.Status != StatusCompleted {
		t.Fatalf("run status = %q", run.Status)
	}
	if run.TotalMs != 800 {
		t.Fatalf("run total_ms = %d, want 800", run.TotalMs)
	}
	if run.InputTokens != 1_000_000 || run.OutputTokens != 100_000 || run.CacheHitTokens != 800_000 {
		t.Fatalf("run tokens = %d/%d/%d", run.InputTokens, run.OutputTokens, run.CacheHitTokens)
	}
	// Tuesday 12:00 UTC is DeepSeek off-peak.
	if diff := run.ModelCostUSD - 0.0924; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("run cost = %v, want 0.0924", run.ModelCostUSD)
	}
	if run.FirstTokenMs != 400 {
		t.Fatalf("run first_token_ms = %d, want 400", run.FirstTokenMs)
	}

	grouped := spansByKind(spans)
	if len(grouped[KindRun]) != 1 || len(grouped[KindSystem]) != 1 ||
		len(grouped[KindUser]) != 1 ||
		len(grouped[KindStep]) != 1 || len(grouped[KindLLM]) != 1 || len(grouped[KindTool]) != 1 {
		t.Fatalf("span kinds = %v", grouped)
	}

	llm := grouped[KindLLM][0]
	if llm.Name != "deepseek-flash" || llm.StepIndex != 1 || llm.RequestIndex != 1 {
		t.Fatalf("llm span = %+v", llm)
	}
	if llm.FirstTokenMs != 400 || llm.GenerationMs != 200 || llm.DurationMs != 600 {
		t.Fatalf("llm timing = ttft %d generation %d duration %d", llm.FirstTokenMs, llm.GenerationMs, llm.DurationMs)
	}
	if throughput := llm.Throughput(); throughput < 499_999 || throughput > 500_001 {
		t.Fatalf("throughput = %v, want 500000", throughput)
	}
	if llm.Status != StatusCompleted || llm.InputJSON == "" || llm.OutputJSON == "" {
		t.Fatalf("llm content/status = %+v", llm)
	}

	tool := grouped[KindTool][0]
	if tool.Name != "bash" || tool.DurationMs != 30 || tool.Status != StatusCompleted {
		t.Fatalf("tool span = %+v", tool)
	}
	if !strings.Contains(tool.InputJSON, "ls") || !strings.Contains(tool.OutputJSON, "a.go") {
		t.Fatalf("tool content = %q / %q", tool.InputJSON, tool.OutputJSON)
	}

	turn := grouped[KindStep][0]
	if turn.Name != "Step 1" || turn.DurationMs != 700 {
		t.Fatalf("turn span = %+v", turn)
	}
	if turn.ParentID != runID || llm.ParentID != turn.ID || tool.ParentID != turn.ID {
		t.Fatalf("span parents = run %q turn %q llm %q tool %q", runID, turn.ParentID, llm.ParentID, tool.ParentID)
	}
}

func TestRecorderIndexesMultipleTurns(t *testing.T) {
	fixture := newRecorderFixture(t, Config{StoreContent: true})
	base := fixture.base
	runID := startRun(t, fixture)

	for turn := 1; turn <= 2; turn++ {
		offset := time.Duration(turn) * time.Second
		fixture.rec.OnEvent(Event{Type: EventStepStart, Time: base.Add(offset), StepIndex: turn})
		fixture.rec.OnEvent(Event{
			Type: EventLLMRequestStart, Time: base.Add(offset + 10*time.Millisecond),
			StepIndex: turn, Model: "deepseek-flash",
		})
		fixture.rec.OnEvent(Event{
			Type: EventLLMEnd, Time: base.Add(offset + 100*time.Millisecond),
			Usage:      provider.Usage{PromptTokens: 1000, CompletionTokens: 100},
			StopReason: provider.StopNormal,
		})
		fixture.rec.OnEvent(Event{Type: EventStepEnd, Time: base.Add(offset + 200*time.Millisecond)})
	}
	fixture.rec.OnEvent(Event{
		Type: EventRunEnd, Time: base.Add(3 * time.Second),
		Usage:      provider.Usage{PromptTokens: 2000, CompletionTokens: 200},
		StopReason: provider.StopNormal,
	})
	if err := fixture.rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	_, spans, err := fixture.store.GetRun(runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}

	grouped := spansByKind(spans)
	turns := grouped[KindStep]
	llms := grouped[KindLLM]
	if len(turns) != 2 || len(llms) != 2 {
		t.Fatalf("turns = %d, llms = %d", len(turns), len(llms))
	}
	if turns[0].StepIndex != 1 || turns[1].StepIndex != 2 {
		t.Fatalf("turn indexes = %d, %d", turns[0].StepIndex, turns[1].StepIndex)
	}
	if llms[0].RequestIndex != 1 || llms[1].RequestIndex != 2 {
		t.Fatalf("request indexes = %d, %d", llms[0].RequestIndex, llms[1].RequestIndex)
	}
}

func TestRecorderSkipsContentWhenDisabled(t *testing.T) {
	fixture := newRecorderFixture(t, Config{StoreContent: false})
	base := fixture.base
	runID := startRun(t, fixture)

	fixture.rec.OnEvent(Event{Type: EventStepStart, Time: base, StepIndex: 1})
	fixture.rec.OnEvent(Event{
		Type: EventLLMRequestStart, Time: base, StepIndex: 1, Model: "deepseek-flash",
		Messages: []provider.Message{{Role: "user", Content: "rahasia"}},
	})
	fixture.rec.OnEvent(Event{
		Type: EventLLMEnd, Time: base.Add(100 * time.Millisecond),
		Usage:      provider.Usage{PromptTokens: 1000, CompletionTokens: 100},
		StopReason: provider.StopNormal, Output: "jawaban rahasia",
	})
	fixture.rec.OnEvent(Event{
		Type: EventToolStart, Time: base.Add(150 * time.Millisecond), StepIndex: 1,
		ToolCallID: "call_1", ToolName: "bash", ToolArguments: `{"cmd":"cat secret"}`,
	})
	fixture.rec.OnEvent(Event{
		Type: EventToolEnd, Time: base.Add(160 * time.Millisecond),
		ToolCallID: "call_1", ToolName: "bash", ToolOutput: "isi rahasia",
	})
	fixture.rec.OnEvent(Event{Type: EventStepEnd, Time: base.Add(200 * time.Millisecond)})
	fixture.rec.OnEvent(Event{
		Type: EventRunEnd, Time: base.Add(300 * time.Millisecond),
		Usage:      provider.Usage{PromptTokens: 1000, CompletionTokens: 100},
		StopReason: provider.StopNormal,
	})
	if err := fixture.rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	_, spans, err := fixture.store.GetRun(runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	for _, span := range spans {
		if span.InputJSON != "" || span.OutputJSON != "" {
			t.Fatalf("%s span kept content: %q / %q", span.Kind, span.InputJSON, span.OutputJSON)
		}
	}

	grouped := spansByKind(spans)
	if grouped[KindLLM][0].InputTokens != 1000 {
		t.Fatalf("tokens must still be recorded, got %+v", grouped[KindLLM][0])
	}
}

func TestRecorderMarksToolErrors(t *testing.T) {
	fixture := newRecorderFixture(t, Config{StoreContent: true})
	base := fixture.base
	runID := startRun(t, fixture)

	fixture.rec.OnEvent(Event{Type: EventStepStart, Time: base, StepIndex: 1})
	fixture.rec.OnEvent(Event{Type: EventLLMRequestStart, Time: base, StepIndex: 1, Model: "deepseek-flash"})
	fixture.rec.OnEvent(Event{
		Type: EventLLMEnd, Time: base.Add(50 * time.Millisecond),
		Usage: provider.Usage{PromptTokens: 10, CompletionTokens: 10}, StopReason: provider.StopNormal,
	})
	fixture.rec.OnEvent(Event{
		Type: EventToolStart, Time: base.Add(60 * time.Millisecond), StepIndex: 1,
		ToolCallID: "call_1", ToolName: "bash",
	})
	fixture.rec.OnEvent(Event{
		Type: EventToolEnd, Time: base.Add(90 * time.Millisecond),
		ToolCallID: "call_1", ToolName: "bash", IsError: true, ErrorText: "exit status 1",
		ToolOutput: "boom",
	})
	fixture.rec.OnEvent(Event{Type: EventStepEnd, Time: base.Add(100 * time.Millisecond)})
	fixture.rec.OnEvent(Event{
		Type: EventRunEnd, Time: base.Add(120 * time.Millisecond),
		Usage:      provider.Usage{PromptTokens: 10, CompletionTokens: 10},
		StopReason: provider.StopNormal,
	})
	if err := fixture.rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	_, spans, err := fixture.store.GetRun(runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	grouped := spansByKind(spans)
	if grouped[KindTool][0].Status != StatusError {
		t.Fatalf("tool status = %q, want error", grouped[KindTool][0].Status)
	}
	if grouped[KindStep][0].Status != StatusError {
		t.Fatalf("turn status = %q, want error", grouped[KindStep][0].Status)
	}
}

func TestRecorderTruncatesContent(t *testing.T) {
	fixture := newRecorderFixture(t, Config{MaxContentChars: 200, StoreContent: true})
	base := fixture.base
	runID := startRun(t, fixture)

	big := strings.Repeat("x", 5000)
	fixture.rec.OnEvent(Event{Type: EventStepStart, Time: base, StepIndex: 1})
	fixture.rec.OnEvent(Event{
		Type: EventLLMRequestStart, Time: base, StepIndex: 1, Model: "deepseek-flash",
		Messages: []provider.Message{
			{Role: "system", Content: big},
			{Role: "user", Content: big},
		},
	})
	fixture.rec.OnEvent(Event{
		Type: EventLLMEnd, Time: base.Add(10 * time.Millisecond),
		Usage: provider.Usage{PromptTokens: 10, CompletionTokens: 10}, StopReason: provider.StopNormal,
		Output: big,
	})
	fixture.rec.OnEvent(Event{Type: EventStepEnd, Time: base.Add(20 * time.Millisecond)})
	fixture.rec.OnEvent(Event{
		Type: EventRunEnd, Time: base.Add(30 * time.Millisecond),
		Usage:      provider.Usage{PromptTokens: 10, CompletionTokens: 10},
		StopReason: provider.StopNormal,
	})
	if err := fixture.rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	_, spans, err := fixture.store.GetRun(runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	grouped := spansByKind(spans)
	llm := grouped[KindLLM][0]

	if len(llm.InputJSON) > 200 {
		t.Fatalf("input_json length = %d, want <= 200", len(llm.InputJSON))
	}
	if !strings.Contains(llm.InputJSON, "truncated") {
		t.Fatalf("input_json is not marked truncated: %q", llm.InputJSON)
	}
	var stored decodedEnvelope
	if err := json.Unmarshal([]byte(llm.InputJSON), &stored); err != nil {
		t.Fatalf("input_json is not a message envelope: %v", err)
	}
	if len(stored.Messages) == 0 || stored.Messages[0].Role != "system" {
		t.Fatalf("trimming dropped the system message: %q", llm.InputJSON)
	}
	if !json.Valid([]byte(llm.InputJSON)) {
		t.Fatalf("input_json is not valid json: %q", llm.InputJSON)
	}
	if !json.Valid([]byte(llm.OutputJSON)) {
		t.Fatalf("output_json is not valid json: %q", llm.OutputJSON)
	}
}

func TestEncodeMessagesKeepsTheSystemPromptWhole(t *testing.T) {
	system := strings.Repeat("s", 12000)
	encoded := encodeMessages([]provider.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: "halo"},
		{Role: "assistant", Content: strings.Repeat("a", 2000)},
		{Role: "tool", Content: strings.Repeat("t", 40000)},
	}, 16384, true)

	if !json.Valid([]byte(encoded)) {
		t.Fatalf("encoded transcript is not valid json: %q", encoded)
	}
	var stored decodedEnvelope
	if err := json.Unmarshal([]byte(encoded), &stored); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(stored.Messages) == 0 || stored.Messages[0].Role != "system" {
		t.Fatalf("system message was dropped: %s", encoded)
	}
	if stored.Messages[0].Content != system {
		t.Fatalf("system prompt was shortened (len %d, want %d)",
			len(stored.Messages[0].Content), len(system))
	}
	if !strings.Contains(encoded, "truncated") {
		t.Fatal("the oversized tool result should have been truncated instead")
	}
}

func TestEncodeMessagesShortensTheSystemPromptLast(t *testing.T) {
	system := strings.Repeat("s", 5000)
	encoded := encodeMessages([]provider.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: strings.Repeat("u", 3000)},
	}, 500, true)

	if !json.Valid([]byte(encoded)) {
		t.Fatalf("encoded transcript is not valid json: %q", encoded)
	}
	if len(encoded) > 500 {
		t.Fatalf("encoded length = %d, want <= 500", len(encoded))
	}
	var stored decodedEnvelope
	if err := json.Unmarshal([]byte(encoded), &stored); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(stored.Messages) == 0 || stored.Messages[0].Role != "system" {
		t.Fatalf("system message was dropped: %s", encoded)
	}
	if !strings.Contains(stored.Messages[0].Content, "truncated") {
		t.Fatalf("system prompt should be the last thing shortened: %q", stored.Messages[0].Content)
	}
}

func TestRecorderSplitsRetriedRequest(t *testing.T) {
	fixture := newRecorderFixture(t, Config{StoreContent: true})
	base := fixture.base
	runID := startRun(t, fixture)

	messages := []provider.Message{
		{Role: "system", Content: "kamu kucing"},
		{Role: "user", Content: "halo"},
	}
	fixture.rec.OnEvent(Event{Type: EventStepStart, Time: base, StepIndex: 1})
	fixture.rec.OnEvent(Event{
		Type: EventLLMRequestStart, Time: base.Add(10 * time.Millisecond),
		StepIndex: 1, Model: "deepseek-flash", Messages: messages,
	})
	fixture.rec.OnEvent(Event{
		Type: EventLLMRetry, Time: base.Add(510 * time.Millisecond),
		StepIndex: 1, Attempt: 2, ErrorText: "stream request failed (500)",
	})
	fixture.rec.OnEvent(Event{Type: EventFirstToken, Time: base.Add(600 * time.Millisecond), StepIndex: 1})
	fixture.rec.OnEvent(Event{
		Type: EventLLMEnd, Time: base.Add(time.Second), StepIndex: 1,
		Usage: provider.Usage{PromptTokens: 10, CompletionTokens: 5}, StopReason: provider.StopNormal,
		Output: "ok",
	})
	fixture.rec.OnEvent(Event{Type: EventStepEnd, Time: base.Add(1100 * time.Millisecond)})
	fixture.rec.OnEvent(Event{
		Type: EventRunEnd, Time: base.Add(1200 * time.Millisecond),
		Usage: provider.Usage{PromptTokens: 10, CompletionTokens: 5}, StopReason: provider.StopNormal,
	})
	if err := fixture.rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	_, spans, err := fixture.store.GetRun(runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	grouped := spansByKind(spans)
	if len(grouped[KindLLM]) != 2 {
		t.Fatalf("llm spans = %d, want one per attempt", len(grouped[KindLLM]))
	}

	first, second := grouped[KindLLM][0], grouped[KindLLM][1]
	if first.RequestIndex != 1 || second.RequestIndex != 2 {
		t.Fatalf("request indexes = %d/%d", first.RequestIndex, second.RequestIndex)
	}
	if first.Status != StatusRetried || first.Error == "" {
		t.Fatalf("failed attempt = %+v", first)
	}
	if second.Status != StatusCompleted {
		t.Fatalf("retried attempt status = %q", second.Status)
	}
	if second.FirstTokenMs == 0 {
		t.Fatalf("retried attempt lost its TTFT: %+v", second)
	}
	if second.InputJSON != first.InputJSON {
		t.Fatal("the retried attempt should reuse the stored transcript")
	}
	if step := grouped[KindStep][0]; step.Status != StatusCompleted {
		t.Fatalf("step status = %q, a retry must not fail the step", step.Status)
	}
}

func TestRecorderRecordsCompactionWithoutContent(t *testing.T) {
	fixture := newRecorderFixture(t, Config{StoreContent: false})
	base := fixture.base
	runID := startRun(t, fixture)

	fixture.rec.OnEvent(Event{Type: EventStepStart, Time: base, StepIndex: 3})
	fixture.rec.OnEvent(Event{
		Type: EventCompaction, Time: base.Add(time.Millisecond), StepIndex: 3,
		TokensBefore: 60000, TokensAfter: 38000, Budget: 64000,
		DroppedMessages: 14, DroppedTokens: 22000,
	})
	fixture.rec.OnEvent(Event{Type: EventStepEnd, Time: base.Add(2 * time.Millisecond)})
	fixture.rec.OnEvent(Event{
		Type: EventRunEnd, Time: base.Add(3 * time.Millisecond), StopReason: provider.StopNormal,
	})
	if err := fixture.rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	_, spans, err := fixture.store.GetRun(runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	grouped := spansByKind(spans)
	if len(grouped[KindCompaction]) != 1 {
		t.Fatalf("compaction spans = %d, want 1", len(grouped[KindCompaction]))
	}
	compaction := grouped[KindCompaction][0]
	if compaction.ParentID != grouped[KindStep][0].ID {
		t.Fatal("compaction span is not nested under the step")
	}
	if !strings.Contains(compaction.InputJSON, `"tokens_before":60000`) {
		t.Fatalf("compaction metadata = %q", compaction.InputJSON)
	}
	if !strings.Contains(compaction.InputJSON, `"messages_dropped":14`) {
		t.Fatalf("compaction metadata = %q", compaction.InputJSON)
	}
}

func TestRecorderCancelsOpenSpansOnAbort(t *testing.T) {
	fixture := newRecorderFixture(t, Config{StoreContent: true})
	base := fixture.base
	runID := startRun(t, fixture)

	fixture.rec.OnEvent(Event{Type: EventStepStart, Time: base, StepIndex: 1})
	fixture.rec.OnEvent(Event{Type: EventLLMRequestStart, Time: base, StepIndex: 1, Model: "deepseek-flash"})
	fixture.rec.OnEvent(Event{
		Type: EventRunEnd, Time: base.Add(50 * time.Millisecond),
		StopReason: provider.StopAborted, Status: StatusCancelled, Err: "context canceled",
	})
	if err := fixture.rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	run, spans, err := fixture.store.GetRun(runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusCancelled || run.Error != "context canceled" {
		t.Fatalf("run = %+v", run)
	}
	grouped := spansByKind(spans)
	if grouped[KindLLM][0].Status != StatusCancelled {
		t.Fatalf("llm status = %q, want cancelled", grouped[KindLLM][0].Status)
	}
	if grouped[KindStep][0].EndedAt == 0 {
		t.Fatal("turn span was left open")
	}
}

func TestRecorderIgnoresEventsWithoutRun(t *testing.T) {
	fixture := newRecorderFixture(t, Config{StoreContent: true})
	// Must not panic before Start.
	fixture.rec.OnEvent(Event{Type: EventFirstToken, Time: fixture.base})
	fixture.rec.OnEvent(Event{Type: EventLLMEnd, Time: fixture.base})
	fixture.rec.OnEvent(Event{Type: EventStepEnd, Time: fixture.base})
}

func TestRecorderDropsEventsAfterRunEnd(t *testing.T) {
	fixture := newRecorderFixture(t, Config{StoreContent: true})
	base := fixture.base
	runID := startRun(t, fixture)

	fixture.rec.OnEvent(Event{
		Type: EventRunEnd, Time: base.Add(10 * time.Millisecond),
		Usage: provider.Usage{PromptTokens: 5, CompletionTokens: 5}, StopReason: provider.StopNormal,
	})
	fixture.rec.OnEvent(Event{Type: EventStepStart, Time: base.Add(20 * time.Millisecond), StepIndex: 1})
	if err := fixture.rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	_, spans, err := fixture.store.GetRun(runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if len(spansByKind(spans)[KindStep]) != 0 {
		t.Fatal("turn span recorded after the run ended")
	}
}

func TestRecorderReportsLiveTotalsWhileRunning(t *testing.T) {
	fixture := newRecorderFixture(t, Config{StoreContent: true})
	base := fixture.base
	runID := startRun(t, fixture)

	fixture.rec.OnEvent(Event{Type: EventStepStart, Time: base, StepIndex: 1})
	fixture.rec.OnEvent(Event{
		Type: EventLLMRequestStart, Time: base.Add(10 * time.Millisecond),
		StepIndex: 1, Model: "deepseek-flash",
	})
	fixture.rec.OnEvent(Event{
		Type: EventLLMEnd, Time: base.Add(610 * time.Millisecond),
		Usage: provider.Usage{
			PromptTokens:      1_000_000,
			CachedInputTokens: 800_000,
			CompletionTokens:  100_000,
			TotalTokens:       1_100_000,
		},
		StopReason: provider.StopNormal,
	})

	if err := fixture.rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	run, spans, err := fixture.store.GetRun(runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusRunning {
		t.Fatalf("status = %q, want running", run.Status)
	}
	if run.InputTokens != 1_000_000 || run.OutputTokens != 100_000 {
		t.Fatalf("live tokens = %d/%d", run.InputTokens, run.OutputTokens)
	}
	if diff := run.ModelCostUSD - 0.0924; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("live cost = %v, want 0.0924", run.ModelCostUSD)
	}
	if run.TotalMs < 600 {
		t.Fatalf("live duration = %d ms, want at least 600", run.TotalMs)
	}
	if run.FirstTokenMs != 0 {
		t.Fatalf("no first-token event was sent, got %d", run.FirstTokenMs)
	}

	grouped := spansByKind(spans)
	if grouped[KindStep][0].Status != StatusRunning {
		t.Fatalf("turn should still be running, got %q", grouped[KindStep][0].Status)
	}
}

type decodedEnvelope struct {
	Dropped  int `json:"dropped_messages"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func TestEncodeMessagesKeepsTheSystemMessage(t *testing.T) {
	big := strings.Repeat("x", 4000)
	messages := []provider.Message{
		{Role: "system", Content: big},
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "second question"},
		{Role: "assistant", Content: "second answer"},
		{Role: "user", Content: "latest question"},
	}

	encoded := encodeMessages(messages, 512, true)
	if !json.Valid([]byte(encoded)) {
		t.Fatalf("not valid json: %q", encoded)
	}
	if len(encoded) > 512 {
		t.Fatalf("length = %d, want <= 512", len(encoded))
	}

	var envelope decodedEnvelope
	if err := json.Unmarshal([]byte(encoded), &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(envelope.Messages) == 0 {
		t.Fatal("no messages kept")
	}
	if envelope.Messages[0].Role != "system" {
		t.Fatalf("first kept message = %q, want system", envelope.Messages[0].Role)
	}
	if envelope.Dropped == 0 {
		t.Fatal("dropped_messages should report the trimmed history")
	}
	last := envelope.Messages[len(envelope.Messages)-1]
	if last.Role != "user" || last.Content != "latest question" {
		t.Fatalf("last kept message = %+v, want the newest question", last)
	}
}

func TestEncodeMessagesTrimsFromTheFrontWithoutASystemMessage(t *testing.T) {
	messages := []provider.Message{
		{Role: "user", Content: "old question"},
		{Role: "assistant", Content: "old answer"},
		{Role: "user", Content: "new question"},
	}
	encoded := encodeMessages(messages, 120, true)

	var envelope decodedEnvelope
	if err := json.Unmarshal([]byte(encoded), &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(envelope.Messages) == 0 {
		t.Fatal("no messages kept")
	}
	if envelope.Messages[0].Role == "system" {
		t.Fatal("there was no system message to protect")
	}
	last := envelope.Messages[len(envelope.Messages)-1]
	if last.Content != "new question" {
		t.Fatalf("last kept message = %+v", last)
	}
	if envelope.Dropped != 1 {
		t.Fatalf("dropped = %d, want 1", envelope.Dropped)
	}
}

func TestEncodeMessagesRespectsStoreContent(t *testing.T) {
	messages := []provider.Message{{Role: "system", Content: "prompt"}}
	if got := encodeMessages(messages, 512, false); got != "" {
		t.Fatalf("store_content=false returned %q", got)
	}
}
