package obs

import (
	"math"
	"testing"
	"time"

	"myaaw/internal/provider"
)

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}

func assertCost(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s: cost = %.10f, want %.10f", label, got, want)
	}
}

func TestCostUsesFlatPrices(t *testing.T) {
	pricer := NewPricer(nil)
	// Tuesday noon UTC is outside every DeepSeek window, but gpt-6.1-sol has no
	// off-peak price, so the flat table applies.
	at := mustTime(t, "2026-10-27T12:00:00Z")

	got := pricer.Cost("gpt-6.1-sol", provider.Usage{
		PromptTokens:     1_000_000,
		CompletionTokens: 100_000,
	}, at)

	assertCost(t, "gpt-6.1-sol", got, 3.00)
}

func TestCostAppliesCacheHitPrice(t *testing.T) {
	pricer := NewPricer(nil)
	at := mustTime(t, "2026-10-27T12:00:00Z") // off-peak

	got := pricer.Cost("deepseek-flash", provider.Usage{
		PromptTokens:      1_000_000,
		CachedInputTokens: 800_000,
		CompletionTokens:  100_000,
	}, at)

	// 200k miss at 0.15 + 800k cached at 0.003 + 100k output at 0.60, per 1M.
	assertCost(t, "deepseek-flash off-peak", got, 0.0924)
}

func TestCostAppliesPeakWindows(t *testing.T) {
	pricer := NewPricer(nil)
	usage := provider.Usage{
		PromptTokens:      1_000_000,
		CachedInputTokens: 800_000,
		CompletionTokens:  100_000,
	}

	cases := []struct {
		name string
		at   string
		want float64
	}{
		{"monday 02:00 utc is peak", "2026-10-26T02:00:00Z", 0.1848},
		{"monday 06:00 utc is peak", "2026-10-26T06:00:00Z", 0.1848},
		{"monday 04:00 utc is off-peak", "2026-10-26T04:00:00Z", 0.0924},
		{"monday 10:00 utc is off-peak", "2026-10-26T10:00:00Z", 0.0924},
		{"monday 00:30 utc is off-peak", "2026-10-26T00:30:00Z", 0.0924},
		{"saturday 02:00 utc is off-peak", "2026-10-31T02:00:00Z", 0.0924},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assertCost(t, testCase.name, pricer.Cost("deepseek-flash", usage, mustTime(t, testCase.at)), testCase.want)
		})
	}
}

func TestCostKeepsPeakWhenOnlyOffPeakIsConfigured(t *testing.T) {
	pricer := NewPricer(map[string]Price{
		"custom-model": {
			Input:   1,
			Output:  2,
			OffPeak: &Price{Input: 0.5, Output: 1},
		},
	})

	got := pricer.Cost("custom-model", provider.Usage{PromptTokens: 1_000_000}, time.Now())
	assertCost(t, "off-peak without windows", got, 1)
}

func TestCostUnknownModelIsFree(t *testing.T) {
	pricer := NewPricer(nil)
	got := pricer.Cost("totally-unknown", provider.Usage{PromptTokens: 1000, CompletionTokens: 1000}, time.Now())
	assertCost(t, "unknown model", got, 0)
}

func TestCostResolvesProviderPrefixedModels(t *testing.T) {
	pricer := NewPricer(nil)
	at := mustTime(t, "2026-10-26T02:00:00Z") // peak

	got := pricer.Cost("deepseek/deepseek-chat", provider.Usage{PromptTokens: 1_000_000}, at)
	assertCost(t, "prefixed model", got, 0.30)
}

func TestCostFallsBackToTotalTokens(t *testing.T) {
	pricer := NewPricer(nil)
	at := mustTime(t, "2026-10-27T12:00:00Z") // off-peak

	got := pricer.Cost("deepseek-flash", provider.Usage{
		TotalTokens:      1_000_000,
		CompletionTokens: 100_000,
	}, at)
	// The prompt share is inferred as 900k tokens at 0.15 plus 100k output at
	// 0.60, per 1M.
	assertCost(t, "total tokens fallback", got, 0.195)
}

func TestConfigOverridesBuiltInPrice(t *testing.T) {
	pricer := NewPricer(map[string]Price{
		"gpt-6.1-sol": {Input: 1, Output: 1},
	})

	got := pricer.Cost("gpt-6.1-sol", provider.Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000}, time.Now())
	assertCost(t, "override", got, 2)
}

func TestInPeakHandlesWindowsCrossingMidnight(t *testing.T) {
	windows := parseWindow("Sun-Mon 22:00-02:00 UTC")
	if len(windows) == 0 {
		t.Fatal("parseWindow rejected a valid window")
	}

	if !inPeak(mustTime(t, "2026-10-25T23:00:00Z"), windows) {
		t.Fatal("sunday 23:00 should be inside the window")
	}
	if !inPeak(mustTime(t, "2026-10-26T01:00:00Z"), windows) {
		t.Fatal("monday 01:00 should be inside the window")
	}
	if inPeak(mustTime(t, "2026-10-26T03:00:00Z"), windows) {
		t.Fatal("monday 03:00 should be outside the window")
	}
}

func TestPercentile(t *testing.T) {
	values := []int64{10, 20, 30, 40, 50}
	if got := percentile(values, 50); got != 30 {
		t.Fatalf("p50 = %d, want 30", got)
	}
	if got := percentile(values, 95); got != 50 {
		t.Fatalf("p95 = %d, want 50", got)
	}
	if got := percentile(nil, 95); got != 0 {
		t.Fatalf("empty p95 = %d, want 0", got)
	}
}

func TestCostResolvesVersionedDeepSeekFlash(t *testing.T) {
	pricer := NewPricer(nil)
	at := mustTime(t, "2026-10-26T02:00:00Z") // Monday 02:00 UTC is peak

	got := pricer.Cost("deepseek-v4-flash", provider.Usage{PromptTokens: 1_000_000}, at)
	assertCost(t, "deepseek-v4-flash", got, 0.30)
}
