package obs

import (
	"math"
	"testing"
)

func TestSummaryAggregatesRuns(t *testing.T) {
	store := newTestStore(t, Options{})
	base := int64(1_700_000_000_000)

	runA := sampleRun("run_a", base)
	runA.Status = StatusCompleted
	runA.Trigger = "user"
	runA.TotalMs = 1000
	runA.FirstTokenMs = 400
	runA.InputTokens = 1000
	runA.OutputTokens = 100
	runA.CacheHitTokens = 800
	runA.ModelCostUSD = 0.001
	runA.ToolCostUSD = 0.0005

	runB := sampleRun("run_b", base+1000)
	runB.Status = StatusError
	runB.Trigger = "cron"
	runB.Error = "boom"
	runB.TotalMs = 3000
	runB.FirstTokenMs = 900
	runB.InputTokens = 2000
	runB.OutputTokens = 200
	runB.CacheHitTokens = 0
	runB.ModelCostUSD = 0.002

	for _, run := range []Run{runA, runB} {
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
	}

	spans := []Span{
		{ID: "a_turn", RunID: runA.ID, Kind: KindStep, StepIndex: 1, StartedAt: base, DurationMs: 1000, Status: StatusCompleted},
		{ID: "a_llm", RunID: runA.ID, Kind: KindLLM, Name: "deepseek-flash", StepIndex: 1, RequestIndex: 1, StartedAt: base, DurationMs: 1000, InputTokens: 1000, OutputTokens: 100, CacheHitTokens: 800, CostUSD: 0.001, Status: StatusCompleted},
		{ID: "a_tool", RunID: runA.ID, Kind: KindTool, Name: "bash", StepIndex: 1, StartedAt: base, DurationMs: 50, CostUSD: 0.0005, Status: StatusCompleted},
		{ID: "b_turn", RunID: runB.ID, Kind: KindStep, StepIndex: 1, StartedAt: base + 1000, DurationMs: 3000, Status: StatusError},
		{ID: "b_llm", RunID: runB.ID, Kind: KindLLM, Name: "gpt-6.1-sol", StepIndex: 1, RequestIndex: 1, StartedAt: base + 1000, DurationMs: 3000, InputTokens: 2000, OutputTokens: 200, CostUSD: 0.002, Status: StatusError},
		{ID: "b_tool", RunID: runB.ID, Kind: KindTool, Name: "bash", StepIndex: 1, StartedAt: base + 1000, DurationMs: 70, Status: StatusError},
	}
	if err := store.UpsertSpans(spans); err != nil {
		t.Fatalf("upsert spans: %v", err)
	}

	summary, err := store.Summary(base, base+5000)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}

	if summary.Runs != 2 || summary.Errors != 1 {
		t.Fatalf("runs = %d, errors = %d", summary.Runs, summary.Errors)
	}
	if math.Abs(summary.ErrorRate-0.5) > 1e-9 {
		t.Fatalf("error rate = %v", summary.ErrorRate)
	}
	if summary.InputTokens != 3000 || summary.OutputTokens != 300 || summary.CacheHitTokens != 800 {
		t.Fatalf("tokens = %d/%d/%d", summary.InputTokens, summary.OutputTokens, summary.CacheHitTokens)
	}
	if math.Abs(summary.CostUSD-0.003) > 1e-9 {
		t.Fatalf("cost = %v", summary.CostUSD)
	}
	if math.Abs(summary.ToolCostUSD-0.0005) > 1e-9 {
		t.Fatalf("tool cost = %v", summary.ToolCostUSD)
	}
	if summary.DurationP50 != 1000 || summary.DurationP95 != 3000 {
		t.Fatalf("durations = p50 %d p95 %d", summary.DurationP50, summary.DurationP95)
	}
	if summary.DatabaseBytes <= 0 {
		t.Fatalf("database bytes = %d, want the on-disk size", summary.DatabaseBytes)
	}
	if summary.FirstTokenP50 != 400 {
		t.Fatalf("first token p50 = %d", summary.FirstTokenP50)
	}

	if len(summary.PerStep) != 1 {
		t.Fatalf("per turn = %+v", summary.PerStep)
	}
	turn := summary.PerStep[0]
	if turn.StepIndex != 1 || turn.Count != 2 || turn.AvgMs != 2000 || turn.P95Ms != 3000 {
		t.Fatalf("turn stat = %+v", turn)
	}
	if math.Abs(turn.CostUSD-0.003) > 1e-9 {
		t.Fatalf("turn cost = %v", turn.CostUSD)
	}

	if len(summary.ByModel) != 2 {
		t.Fatalf("by model = %+v", summary.ByModel)
	}
	if summary.ByModel[0].Model != "gpt-6.1-sol" {
		t.Fatalf("models must be ordered by cost, got %+v", summary.ByModel)
	}

	if len(summary.ByTrigger) != 2 {
		t.Fatalf("by trigger = %+v", summary.ByTrigger)
	}
	byTrigger := map[string]TriggerStat{}
	for _, stat := range summary.ByTrigger {
		byTrigger[stat.Trigger] = stat
	}
	if stat := byTrigger["user"]; stat.Runs != 1 || stat.Errors != 0 || stat.CostUSD != 0.001 || stat.ToolCostUSD != 0.0005 {
		t.Fatalf("user trigger = %+v", stat)
	}
	if stat := byTrigger["cron"]; stat.Runs != 1 || stat.Errors != 1 || stat.CostUSD != 0.002 {
		t.Fatalf("cron trigger = %+v", stat)
	}

	if len(summary.PerTool) != 1 {
		t.Fatalf("per tool = %+v", summary.PerTool)
	}
	tool := summary.PerTool[0]
	if tool.Name != "bash" || tool.Calls != 2 || tool.Errors != 1 || tool.AvgMs != 60 {
		t.Fatalf("tool stat = %+v", tool)
	}
	if tool.CostUSD != 0.0005 {
		t.Fatalf("tool cost = %v", tool.CostUSD)
	}

	if summary.MostExpensiveRunID != runB.ID {
		t.Fatalf("most expensive = %q", summary.MostExpensiveRunID)
	}
	if summary.SlowestRunID != runB.ID {
		t.Fatalf("slowest = %q", summary.SlowestRunID)
	}
}

func TestSummaryOnEmptyWindow(t *testing.T) {
	store := newTestStore(t, Options{})

	summary, err := store.Summary(0, 1)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Runs != 0 || summary.ErrorRate != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	if len(summary.PerStep) != 0 || len(summary.ByModel) != 0 || len(summary.PerTool) != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.MostExpensiveRunID != "" || summary.SlowestRunID != "" {
		t.Fatalf("badges = %+v", summary)
	}
}

func TestSummaryExcludesRunningRunsFromCountsButNotSpend(t *testing.T) {
	store := newTestStore(t, Options{})
	base := int64(1_700_000_000_000)

	finished := sampleRun("run_done", base)
	finished.TotalMs = 1000
	finished.FirstTokenMs = 400
	finished.ModelCostUSD = 0.001
	finished.InputTokens = 1000
	finished.OutputTokens = 100

	inFlight := sampleRun("run_live", base+1000)
	inFlight.Status = StatusRunning
	inFlight.EndedAt = 0
	inFlight.TotalMs = 500
	inFlight.FirstTokenMs = 0
	inFlight.ModelCostUSD = 0.002
	inFlight.InputTokens = 2000
	inFlight.OutputTokens = 200

	for _, run := range []Run{finished, inFlight} {
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
	}

	summary, err := store.Summary(base, base+5000)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Runs != 1 {
		t.Fatalf("runs = %d, want only the finished run", summary.Runs)
	}
	if summary.DurationP95 != 1000 {
		t.Fatalf("p95 = %d, want the finished run only", summary.DurationP95)
	}
	if math.Abs(summary.CostUSD-0.003) > 1e-9 {
		t.Fatalf("cost = %v, want spend from both runs", summary.CostUSD)
	}
	if summary.InputTokens != 3000 {
		t.Fatalf("input tokens = %d, want 3000", summary.InputTokens)
	}
}

func TestSummaryReportsUnpricedRuns(t *testing.T) {
	store := newTestStore(t, Options{})
	base := int64(1_700_000_000_000)

	priced := sampleRun("run_priced", base)
	priced.Status = StatusCompleted
	priced.Model = "deepseek-flash"
	priced.InputTokens = 1000
	priced.OutputTokens = 100
	priced.ModelCostUSD = 0.001

	unpriced := sampleRun("run_unpriced", base+1000)
	unpriced.Status = StatusCompleted
	unpriced.Model = "models/gemini-3.1-pro-preview"
	unpriced.InputTokens = 5000
	unpriced.OutputTokens = 500
	unpriced.ModelCostUSD = 0

	for _, run := range []Run{priced, unpriced} {
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
	}

	summary, err := store.Summary(base, base+5000)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}

	if summary.CostComplete {
		t.Fatal("cost_complete = true, want false while a model has no price")
	}
	if summary.UnpricedRuns != 1 || summary.UnpricedInputTokens != 5000 || summary.UnpricedOutputTokens != 500 {
		t.Fatalf("unpriced = %d runs, %d/%d tokens", summary.UnpricedRuns,
			summary.UnpricedInputTokens, summary.UnpricedOutputTokens)
	}
	if len(summary.UnpricedModels) != 1 || summary.UnpricedModels[0] != "models/gemini-3.1-pro-preview" {
		t.Fatalf("unpriced models = %+v", summary.UnpricedModels)
	}

	// The total still reports only the priced run, so the flags are the only
	// signal that the number is a lower bound.
	if math.Abs(summary.CostUSD-0.001) > 1e-9 {
		t.Fatalf("cost = %v", summary.CostUSD)
	}
}

func TestSummaryIsCostCompleteWhenEveryModelIsPriced(t *testing.T) {
	store := newTestStore(t, Options{})
	base := int64(1_700_000_000_000)

	run := sampleRun("run_priced", base)
	run.Status = StatusCompleted
	run.Model = "deepseek-flash"
	run.InputTokens = 1000
	run.ModelCostUSD = 0.001
	if err := store.UpsertRun(run); err != nil {
		t.Fatalf("upsert run: %v", err)
	}

	summary, err := store.Summary(base, base+5000)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if !summary.CostComplete || summary.UnpricedRuns != 0 {
		t.Fatalf("cost_complete = %v, unpriced = %d", summary.CostComplete, summary.UnpricedRuns)
	}
}

func TestListSessionsGroupsTurnsByConversation(t *testing.T) {
	store := newTestStore(t, Options{})
	base := int64(1_700_000_000_000)

	first := sampleRun("run_1", base)
	first.ConversationID = "session-a"
	first.ModelCostUSD = 0.001
	second := sampleRun("run_2", base+2000)
	second.ConversationID = "session-a"
	second.ModelCostUSD = 0.002
	second.Status = StatusError

	other := sampleRun("run_3", base+1000)
	other.ConversationID = "session-b"
	other.ModelCostUSD = 0.004

	unscoped := sampleRun("run_4", base+3000)
	unscoped.ConversationID = ""

	for _, run := range []Run{first, second, other, unscoped} {
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
	}

	sessions, err := store.ListSessions(10)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2 (runs without a conversation are skipped)", len(sessions))
	}
	// Newest activity first: session-a was last touched at base+2000.
	if sessions[0].ConversationID != "session-a" {
		t.Fatalf("first session = %q, want session-a", sessions[0].ConversationID)
	}
	if sessions[0].Turns != 2 || sessions[0].ErrorCount != 1 {
		t.Fatalf("session-a = %+v", sessions[0])
	}
	if math.Abs(sessions[0].CostUSD-0.003) > 1e-9 {
		t.Fatalf("session-a cost = %v", sessions[0].CostUSD)
	}
	if sessions[0].StartedAt != base || sessions[0].LastActiveAt != base+2000 {
		t.Fatalf("session-a window = %d..%d", sessions[0].StartedAt, sessions[0].LastActiveAt)
	}
	if !sessions[0].CostComplete || sessions[0].UnpricedTurns != 0 {
		t.Fatalf("session-a pricing = %+v", sessions[0])
	}
}
