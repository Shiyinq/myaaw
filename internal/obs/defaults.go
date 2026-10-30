package obs

// Price is a model price in USD per one million tokens.
//
// Input is the cache-miss price, CacheHit is the discounted price for prompt
// tokens served from the provider cache, and Output is the completion price.
// OffPeak, when set, replaces the whole triplet outside every PeakWindows
// entry; only models that declare both fields get peak pricing.
type Price struct {
	Input       float64  `json:"input"`
	CacheHit    float64  `json:"cache_hit"`
	Output      float64  `json:"output"`
	OffPeak     *Price   `json:"off_peak,omitempty"`
	PeakWindows []string `json:"peak_windows,omitempty"`
}

// DeepSeekPeakWindows is the published DeepSeek peak schedule. Off-peak is
// exactly half price. Chinese public holidays are not modelled, so a holiday
// is billed as peak, which overestimates rather than undercounts.
var DeepSeekPeakWindows = []string{
	"Mon-Fri 01:00-04:00 UTC",
	"Mon-Fri 06:00-10:00 UTC",
}

// DefaultPrices returns the built-in price table. Callers may override any
// entry through configuration; unknown models are recorded with zero cost
// instead of failing a run.
func DefaultPrices() map[string]Price {
	deepSeekPeak := append([]string(nil), DeepSeekPeakWindows...)

	return map[string]Price{
		"deepseek-flash": {
			Input: 0.30, CacheHit: 0.006, Output: 1.20,
			OffPeak:     &Price{Input: 0.15, CacheHit: 0.003, Output: 0.60},
			PeakWindows: deepSeekPeak,
		},
		"deepseek-chat": {
			Input: 0.30, CacheHit: 0.006, Output: 1.20,
			OffPeak:     &Price{Input: 0.15, CacheHit: 0.003, Output: 0.60},
			PeakWindows: append([]string(nil), DeepSeekPeakWindows...),
		},
		// The versioned flash id tracks the published flash rates. Override it in
		// config.json if DeepSeek prices it differently.
		"deepseek-v4-flash": {
			Input: 0.30, CacheHit: 0.006, Output: 1.20,
			OffPeak:     &Price{Input: 0.15, CacheHit: 0.003, Output: 0.60},
			PeakWindows: append([]string(nil), DeepSeekPeakWindows...),
		},
		"deepseek-v4-pro": {
			Input: 1.32, CacheHit: 0.044, Output: 3.96,
			OffPeak:     &Price{Input: 0.66, CacheHit: 0.022, Output: 1.98},
			PeakWindows: append([]string(nil), DeepSeekPeakWindows...),
		},
		"gpt-6.1-sol":      {Input: 2.00, CacheHit: 0.10, Output: 10.00},
		"gpt-6.1-terra":    {Input: 1.00, CacheHit: 0, Output: 5.00},
		"gpt-6-luna":       {Input: 0.10, CacheHit: 0, Output: 0.50},
		"gpt-5.3-codex":    {Input: 1.75, CacheHit: 0, Output: 14.00},
		"chat-latest":      {Input: 5.00, CacheHit: 0, Output: 30.00},
		"gemini-3.8-flash": {Input: 0.75, CacheHit: 0, Output: 3.75},
		"gemini-2.5-flash": {Input: 0.30, CacheHit: 0, Output: 2.50},
	}
}
