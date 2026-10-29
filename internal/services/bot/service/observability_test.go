package service

import (
	"context"
	"path/filepath"
	"testing"

	"myaaw/internal/agent"
	"myaaw/internal/config"
	"myaaw/internal/obs"
	"myaaw/internal/obs/agenttrace"
	"myaaw/internal/provider"
)

// stubProvider replays one scripted assistant message, enough to drive a run.
type stubProvider struct {
	turns []provider.Message
	calls int
}

func (p *stubProvider) ProviderName() string { return "deepseek" }

func (p *stubProvider) DefaultModel(model string) string {
	if model == "" {
		return "deepseek-flash"
	}
	return model
}

func (p *stubProvider) Models() ([]string, error) { return []string{"deepseek-flash"}, nil }

func (p *stubProvider) Chat(string, []provider.Message, []map[string]interface{}) (provider.Message, error) {
	return provider.Message{}, nil
}

func (p *stubProvider) ChatStream(string, []provider.Message, func(provider.Message) error, []map[string]interface{}) error {
	return nil
}

func (p *stubProvider) ChatStreamEvents(
	ctx context.Context,
	_ string,
	_ []provider.Message,
	_ []map[string]interface{},
	onEvent func(provider.StreamEvent) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(p.turns) == 0 {
		return nil
	}
	message := p.turns[0]
	if len(p.turns) > 1 {
		message = p.turns[p.calls]
		p.calls++
	}
	if text := provider.MessageText(message); text != "" {
		if err := onEvent(provider.StreamEvent{Type: provider.StreamTextDelta, Text: text}); err != nil {
			return err
		}
	}
	return onEvent(provider.StreamEvent{
		Type:       provider.StreamDone,
		Message:    &message,
		Usage:      message.Usage,
		StopReason: provider.StopNormal,
	})
}

func TestRunRecordsTraceEndToEnd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store, err := obs.Open(filepath.Join(t.TempDir(), "observability.db"), obs.Options{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	providerStub := &stubProvider{turns: []provider.Message{{
		Role:       "assistant",
		Content:    "hello from the model",
		StopReason: provider.StopNormal,
		Usage: provider.Usage{
			PromptTokens:      1_000_000,
			CachedInputTokens: 800_000,
			CompletionTokens:  100_000,
			TotalTokens:       1_100_000,
		},
	}}}

	agentUnderTest := agent.NewAgent(providerStub)
	trace := agenttrace.Start(store, obs.NewPricer(nil), obs.Config{StoreContent: true, MaxContentChars: 2048}, obs.RunStart{
		Channel: "telegram", UserID: "42", Provider: "deepseek",
		Model: "deepseek-flash", Agent: "default",
		UserMessage: "hello", SystemPrompt: "be nice",
	})
	defer trace.Close()

	cfg := agent.RunConfig{Model: "deepseek-flash", MaxIterations: 3}
	// The bot service wires the trace id into the engine so both sides agree.
	cfg.RunID = trace.RunID()

	result, err := agentUnderTest.RunEvents(
		context.Background(),
		cfg,
		[]provider.Message{
			{Role: "system", Content: "be nice"},
			{Role: "user", Content: "hello"},
		},
		func(event agent.AgentEvent) error {
			trace.Event(event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	trace.End(result)
	if err := trace.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if runID := trace.RunID(); runID != result.RunID {
		t.Fatalf("recorder run id %q, engine run id %q", runID, result.RunID)
	}

	run, spans, err := store.GetRun(result.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != obs.StatusCompleted {
		t.Fatalf("run status = %q", run.Status)
	}
	if run.InputTokens != 1_000_000 || run.OutputTokens != 100_000 {
		t.Fatalf("run tokens = %d/%d", run.InputTokens, run.OutputTokens)
	}

	kinds := map[obs.SpanKind]int{}
	for _, span := range spans {
		kinds[span.Kind]++
	}
	if kinds[obs.KindRun] != 1 || kinds[obs.KindLLM] != 1 || kinds[obs.KindStep] != 1 {
		t.Fatalf("span kinds = %v", kinds)
	}
	if kinds[obs.KindSystem] != 1 || kinds[obs.KindUser] != 1 {
		t.Fatalf("input spans = %v", kinds)
	}

	for _, span := range spans {
		if span.Kind != obs.KindLLM {
			continue
		}
		if span.InputJSON == "" || span.OutputJSON == "" {
			t.Fatalf("llm content was not captured: %+v", span)
		}
		if span.CostUSD <= 0 {
			t.Fatalf("llm cost = %v, want a priced request", span.CostUSD)
		}
	}
}

func TestRunTrigger(t *testing.T) {
	cases := []struct {
		channel string
		trigger string
		want    string
	}{
		{"telegram", "", "user"},
		{"cli", "", "user"},
		{"a2a", "", "a2a"},
		{"telegram", "cron", "cron"},
		{"subagent", "subagent", "subagent"},
	}
	for _, tc := range cases {
		if got := runTrigger(tc.channel, tc.trigger); got != tc.want {
			t.Errorf("runTrigger(%q, %q) = %q, want %q", tc.channel, tc.trigger, got, tc.want)
		}
	}
}

func TestObsPricesConvertsOverrides(t *testing.T) {
	converted := obsPrices(map[string]config.ModelPriceConfig{
		"deepseek-chat": {
			Input: 0.3, Output: 1.2,
			OffPeak:     &config.ModelPriceConfig{Input: 0.15, Output: 0.6},
			PeakWindows: []string{"Mon-Fri 01:00-04:00 UTC"},
		},
	})
	if len(converted) != 1 {
		t.Fatalf("converted = %+v", converted)
	}
	price := converted["deepseek-chat"]
	if price.Input != 0.3 || price.Output != 1.2 {
		t.Fatalf("price = %+v", price)
	}
	if price.OffPeak == nil || price.OffPeak.Input != 0.15 {
		t.Fatalf("off peak = %+v", price.OffPeak)
	}
	if len(price.PeakWindows) != 1 {
		t.Fatalf("peak windows = %v", price.PeakWindows)
	}
	if obsPrices(nil) != nil {
		t.Fatal("nil prices should stay nil")
	}
}
