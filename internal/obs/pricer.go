package obs

import (
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"myaaw/internal/provider"
)

// Cost is computed from token counts, so swapping models mid-session bills each
// request at the price of the model that actually served it.
var windowPattern = regexp.MustCompile(`^([A-Za-z]{3})-([A-Za-z]{3}) (\d{2}):(\d{2})-(\d{2}):(\d{2}) UTC$`)

// peakWindow is one parsed entry of Price.PeakWindows: the window repeats on
// `day` every week, in minute-of-day units.
type peakWindow struct {
	day     int
	fromMin int
	toMin   int
}

// Pricer resolves a model price and turns token usage into USD.
type Pricer struct {
	prices  map[string]Price
	windows map[string][]peakWindow

	mu     sync.Mutex
	warned map[string]bool
}

// NewPricer builds a pricer from the built-in table plus configuration
// overrides. Overrides are keyed by model name and replace the whole entry.
func NewPricer(overrides map[string]Price) *Pricer {
	prices := DefaultPrices()
	for model, price := range overrides {
		key := normalizeModel(model)
		if key == "" {
			continue
		}
		prices[key] = price
	}

	windows := make(map[string][]peakWindow, len(prices))
	for model, price := range prices {
		if len(price.PeakWindows) == 0 {
			continue
		}
		parsed := make([]peakWindow, 0, len(price.PeakWindows))
		for _, raw := range price.PeakWindows {
			parsed = append(parsed, parseWindow(raw)...)
		}
		windows[model] = parsed
	}

	return &Pricer{prices: prices, windows: windows, warned: map[string]bool{}}
}

// PriceAt returns the effective price for a model at a point in time. The
// second result is false when the model is not priced.
func (p *Pricer) PriceAt(model string, at time.Time) (Price, bool) {
	if p == nil {
		return Price{}, false
	}
	price, key, ok := p.lookup(model)
	if !ok {
		return Price{}, false
	}
	if price.OffPeak != nil && len(p.windows[key]) > 0 {
		if !inPeak(at, p.windows[key]) {
			return *price.OffPeak, true
		}
	}
	return price, true
}

// Cost converts one request's usage into USD. Unknown models cost nothing; they
// are reported once rather than failing the run.
func (p *Pricer) Cost(model string, usage provider.Usage, at time.Time) float64 {
	price, ok := p.PriceAt(model, at)
	if !ok {
		p.warnOnce(model, "no price configured for model %q; cost recorded as zero")
		return 0
	}

	prompt := usage.PromptTokens
	completion := usage.CompletionTokens
	if prompt == 0 && usage.TotalTokens > 0 {
		prompt = usage.TotalTokens - completion
		if prompt < 0 {
			prompt = 0
		}
	}
	cached := usage.CachedInputTokens
	if cached < 0 {
		cached = 0
	}
	if cached > prompt {
		cached = prompt
	}
	miss := prompt - cached

	total := float64(miss)*price.Input +
		float64(cached)*price.CacheHit +
		float64(completion)*price.Output
	return total / 1e6
}

func (p *Pricer) lookup(model string) (Price, string, bool) {
	for _, key := range modelKeys(model) {
		if price, ok := p.prices[key]; ok {
			return price, key, true
		}
	}
	return Price{}, "", false
}

// modelKeys returns the candidate lookup keys for a model id, tolerating
// provider prefixes ("openai/gpt-6-luna") and suffixes ("deepseek-chat:free").
func modelKeys(model string) []string {
	base := normalizeModel(model)
	if base == "" {
		return nil
	}
	keys := []string{base}
	if idx := strings.LastIndex(base, "/"); idx >= 0 && idx+1 < len(base) {
		keys = append(keys, base[idx+1:])
	}
	if idx := strings.Index(base, ":"); idx > 0 {
		keys = append(keys, base[:idx])
	}
	return keys
}

func normalizeModel(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func (p *Pricer) warnOnce(model, format string) {
	key := normalizeModel(model)
	p.mu.Lock()
	seen := p.warned[key]
	if !seen {
		p.warned[key] = true
	}
	p.mu.Unlock()
	if !seen {
		log.Printf("[obs] "+format, model)
	}
}

func inPeak(at time.Time, windows []peakWindow) bool {
	if len(windows) == 0 {
		return true
	}
	utc := at.UTC()
	mow := int(utc.Weekday())*1440 + utc.Hour()*60 + utc.Minute()
	const weekMinutes = 7 * 1440

	for _, window := range windows {
		start := window.day*1440 + window.fromMin
		end := window.day*1440 + window.toMin
		if end <= start {
			// The window crosses midnight into the next day.
			end += 1440
		}
		if mow >= start && mow < end {
			return true
		}
		if mow+weekMinutes >= start && mow+weekMinutes < end {
			return true
		}
	}
	return false
}

// parseWindow expands "Mon-Fri 01:00-04:00 UTC" into one entry per day, so the
// time range applies to every listed day rather than spanning the whole week.
func parseWindow(raw string) []peakWindow {
	match := windowPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		log.Printf("[obs] ignoring unparseable peak window %q; treating it as peak time", raw)
		return nil
	}
	fromDay, okFrom := weekdayIndex(match[1])
	toDay, okTo := weekdayIndex(match[2])
	if !okFrom || !okTo {
		log.Printf("[obs] ignoring peak window %q with unknown weekday; treating it as peak time", raw)
		return nil
	}
	fromHour, _ := strconv.Atoi(match[3])
	fromMinute, _ := strconv.Atoi(match[4])
	toHour, _ := strconv.Atoi(match[5])
	toMinute, _ := strconv.Atoi(match[6])
	if fromHour > 23 || toHour > 23 || fromMinute > 59 || toMinute > 59 {
		log.Printf("[obs] ignoring peak window %q with an invalid time; treating it as peak time", raw)
		return nil
	}

	windows := make([]peakWindow, 0, 7)
	for _, day := range dayRange(fromDay, toDay) {
		windows = append(windows, peakWindow{
			day:     day,
			fromMin: fromHour*60 + fromMinute,
			toMin:   toHour*60 + toMinute,
		})
	}
	return windows
}

func dayRange(from, to int) []int {
	days := make([]int, 0, 7)
	for day := from; ; day = (day + 1) % 7 {
		days = append(days, day)
		if day == to {
			return days
		}
	}
}

func weekdayIndex(name string) (int, bool) {
	switch strings.ToLower(name) {
	case "sun":
		return int(time.Sunday), true
	case "mon":
		return int(time.Monday), true
	case "tue":
		return int(time.Tuesday), true
	case "wed":
		return int(time.Wednesday), true
	case "thu":
		return int(time.Thursday), true
	case "fri":
		return int(time.Friday), true
	case "sat":
		return int(time.Saturday), true
	}
	return 0, false
}
