package obs

import (
	"database/sql"
	"fmt"
	"sort"
)

// StepStat aggregates one loop iteration across every run in the window.
type StepStat struct {
	StepIndex int     `json:"step_index"`
	Count     int     `json:"count"`
	AvgMs     int64   `json:"avg_ms"`
	P95Ms     int64   `json:"p95_ms"`
	CostUSD   float64 `json:"cost_usd"`
}

// ModelStat aggregates model requests.
type ModelStat struct {
	Model          string  `json:"model"`
	Calls          int     `json:"calls"`
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	CacheHitTokens int     `json:"cache_hit_tokens"`
	CostUSD        float64 `json:"cost_usd"`
	// Priced is false when at least one call spent tokens without recording a
	// cost — usually a missing price entry, sometimes a call recorded before the
	// entry existed.
	Priced        bool `json:"priced"`
	UnpricedCalls int  `json:"unpriced_calls,omitempty"`
}

// TriggerStat aggregates runs by what started them.
type TriggerStat struct {
	Trigger      string  `json:"trigger"`
	Runs         int     `json:"runs"`
	Errors       int     `json:"errors"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	ToolCostUSD  float64 `json:"tool_cost_usd,omitempty"`
}

// ToolStat aggregates tool executions.
type ToolStat struct {
	Name    string  `json:"name"`
	Calls   int     `json:"calls"`
	Errors  int     `json:"errors"`
	AvgMs   int64   `json:"avg_ms"`
	CostUSD float64 `json:"cost_usd"`
}

// Summary is the control plane's headline view of a time window.
type Summary struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`

	Runs      int     `json:"runs"`
	Errors    int     `json:"errors"`
	ErrorRate float64 `json:"error_rate"`

	// DatabaseBytes is the size of the trace database on disk, including its
	// write-ahead log. It makes growth visible without shelling out to du.
	DatabaseBytes int64 `json:"database_bytes"`

	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	CacheHitTokens int     `json:"cache_hit_tokens"`
	CostUSD        float64 `json:"cost_usd"`
	// ToolCostUSD is spend on priced tool calls, separate from model tokens.
	ToolCostUSD float64 `json:"tool_cost_usd"`

	// CostComplete is false when at least one run used a model with no price,
	// so CostUSD is a lower bound rather than the full spend.
	CostComplete         bool     `json:"cost_complete"`
	UnpricedRuns         int      `json:"unpriced_runs"`
	UnpricedInputTokens  int      `json:"unpriced_input_tokens"`
	UnpricedOutputTokens int      `json:"unpriced_output_tokens"`
	UnpricedModels       []string `json:"unpriced_models,omitempty"`

	DurationP50   int64 `json:"duration_ms_p50"`
	DurationP95   int64 `json:"duration_ms_p95"`
	FirstTokenP50 int64 `json:"first_token_ms_p50"`

	PerStep   []StepStat    `json:"per_step"`
	ByModel   []ModelStat   `json:"by_model"`
	ByTrigger []TriggerStat `json:"by_trigger"`
	PerTool   []ToolStat    `json:"per_tool"`

	MostExpensiveRunID string `json:"most_expensive_run_id,omitempty"`
	SlowestRunID       string `json:"slowest_run_id,omitempty"`
}

// Summary aggregates runs that started inside [from, to].
func (s *Store) Summary(from, to int64) (Summary, error) {
	summary := Summary{From: from, To: to, PerStep: []StepStat{}, ByModel: []ModelStat{}, ByTrigger: []TriggerStat{}, PerTool: []ToolStat{}, DatabaseBytes: s.Size()}

	err := s.db.QueryRow(`SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status IN ('error', 'interrupted') THEN 1 ELSE 0 END), 0)
		FROM runs WHERE started_at BETWEEN ? AND ? AND status <> 'running'`, from, to).
		Scan(&summary.Runs, &summary.Errors)
	if err != nil {
		return Summary{}, fmt.Errorf("summarise runs: %w", err)
	}
	if summary.Runs > 0 {
		summary.ErrorRate = float64(summary.Errors) / float64(summary.Runs)
	}

	// Spend includes a run that is still in flight: those tokens are already
	// paid for. Counts and percentiles above exclude it.
	err = s.db.QueryRow(`SELECT
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_hit_tokens), 0),
			COALESCE(SUM(model_cost_usd), 0),
			COALESCE(SUM(tool_cost_usd), 0)
		FROM runs WHERE started_at BETWEEN ? AND ?`, from, to).
		Scan(&summary.InputTokens, &summary.OutputTokens, &summary.CacheHitTokens,
			&summary.CostUSD, &summary.ToolCostUSD)
	if err != nil {
		return Summary{}, fmt.Errorf("summarise usage: %w", err)
	}

	if err := s.fillUnpriced(&summary, from, to); err != nil {
		return Summary{}, err
	}

	durations, err := s.columnInt64(
		`SELECT total_ms FROM runs WHERE started_at BETWEEN ? AND ? AND total_ms > 0 AND status <> 'running'`,
		from, to)
	if err != nil {
		return Summary{}, err
	}
	summary.DurationP50 = percentile(durations, 50)
	summary.DurationP95 = percentile(durations, 95)

	firstTokens, err := s.columnInt64(
		`SELECT first_token_ms FROM runs WHERE started_at BETWEEN ? AND ? AND first_token_ms > 0 AND status <> 'running'`,
		from, to)
	if err != nil {
		return Summary{}, err
	}
	summary.FirstTokenP50 = percentile(firstTokens, 50)

	if err := s.fillStepStats(&summary, from, to); err != nil {
		return Summary{}, err
	}
	if err := s.fillModelStats(&summary, from, to); err != nil {
		return Summary{}, err
	}
	if err := s.fillTriggerStats(&summary, from, to); err != nil {
		return Summary{}, err
	}
	if err := s.fillToolStats(&summary, from, to); err != nil {
		return Summary{}, err
	}

	summary.MostExpensiveRunID = s.runIDBy(
		`SELECT id FROM runs WHERE started_at BETWEEN ? AND ? ORDER BY model_cost_usd DESC, started_at DESC LIMIT 1`,
		from, to)
	summary.SlowestRunID = s.runIDBy(
		`SELECT id FROM runs WHERE started_at BETWEEN ? AND ? ORDER BY total_ms DESC, started_at DESC LIMIT 1`,
		from, to)

	return summary, nil
}

// unpricedFilter matches a run that spent tokens but recorded no cost. The
// heuristic mirrors the CLI cost label: a priced model always produces a
// positive cost once it has tokens, so cost 0 with tokens means no price entry.
const unpricedFilter = `COALESCE(model_cost_usd, 0) = 0 AND (COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0)) > 0`

// fillUnpriced reports how much of the window could not be priced, so a reader
// never mistakes CostUSD for the complete spend.
func (s *Store) fillUnpriced(summary *Summary, from, to int64) error {
	err := s.db.QueryRow(`SELECT COUNT(*),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0)
		FROM runs WHERE started_at BETWEEN ? AND ? AND `+unpricedFilter, from, to).
		Scan(&summary.UnpricedRuns, &summary.UnpricedInputTokens, &summary.UnpricedOutputTokens)
	if err != nil {
		return fmt.Errorf("summarise unpriced runs: %w", err)
	}
	summary.CostComplete = summary.UnpricedRuns == 0

	rows, err := s.db.Query(`SELECT DISTINCT COALESCE(model, '')
		FROM runs WHERE started_at BETWEEN ? AND ? AND `+unpricedFilter, from, to)
	if err != nil {
		return fmt.Errorf("summarise unpriced models: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var model string
		if err := rows.Scan(&model); err != nil {
			return fmt.Errorf("scan unpriced model: %w", err)
		}
		if model != "" {
			summary.UnpricedModels = append(summary.UnpricedModels, model)
		}
	}
	return rows.Err()
}

func (s *Store) fillStepStats(summary *Summary, from, to int64) error {
	rows, err := s.db.Query(`SELECT s.step_index, s.duration_ms
		FROM spans s JOIN runs r ON r.id = s.run_id
		WHERE s.kind = 'step' AND r.started_at BETWEEN ? AND ?
		ORDER BY s.step_index`, from, to)
	if err != nil {
		return fmt.Errorf("summarise steps: %w", err)
	}
	defer rows.Close()

	type bucket struct {
		count   int
		totalMs int64
		cost    float64
		durs    []int64
	}
	buckets := map[int]*bucket{}
	order := []int{}

	for rows.Next() {
		var index int
		var duration int64
		if err := rows.Scan(&index, &duration); err != nil {
			return fmt.Errorf("scan step stat: %w", err)
		}
		b, ok := buckets[index]
		if !ok {
			b = &bucket{}
			buckets[index] = b
			order = append(order, index)
		}
		b.count++
		b.totalMs += duration
		if duration > 0 {
			b.durs = append(b.durs, duration)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate step stats: %w", err)
	}

	// Cost lives on the llm span, not on the step that contains it.
	costRows, err := s.db.Query(`SELECT s.step_index, COALESCE(SUM(s.cost_usd), 0)
		FROM spans s JOIN runs r ON r.id = s.run_id
		WHERE s.kind = 'llm' AND r.started_at BETWEEN ? AND ?
		GROUP BY s.step_index`, from, to)
	if err != nil {
		return fmt.Errorf("summarise step cost: %w", err)
	}
	defer costRows.Close()

	for costRows.Next() {
		var index int
		var cost float64
		if err := costRows.Scan(&index, &cost); err != nil {
			return fmt.Errorf("scan step cost: %w", err)
		}
		if b, ok := buckets[index]; ok {
			b.cost += cost
		}
	}
	if err := costRows.Err(); err != nil {
		return fmt.Errorf("iterate step cost: %w", err)
	}

	sort.Ints(order)
	for _, index := range order {
		b := buckets[index]
		stat := StepStat{StepIndex: index, Count: b.count, CostUSD: b.cost}
		if b.count > 0 {
			stat.AvgMs = b.totalMs / int64(b.count)
		}
		stat.P95Ms = percentile(b.durs, 95)
		summary.PerStep = append(summary.PerStep, stat)
	}
	return nil
}

func (s *Store) fillModelStats(summary *Summary, from, to int64) error {
	rows, err := s.db.Query(`SELECT COALESCE(s.name, ''), COUNT(*),
			COALESCE(SUM(s.input_tokens), 0), COALESCE(SUM(s.output_tokens), 0),
			COALESCE(SUM(s.cache_hit_tokens), 0), COALESCE(SUM(s.cost_usd), 0),
			COALESCE(SUM(CASE WHEN COALESCE(s.cost_usd, 0) = 0 AND (COALESCE(s.input_tokens, 0) + COALESCE(s.output_tokens, 0)) > 0 THEN 1 ELSE 0 END), 0)
		FROM spans s JOIN runs r ON r.id = s.run_id
		WHERE s.kind = 'llm' AND r.started_at BETWEEN ? AND ?
		GROUP BY s.name ORDER BY SUM(s.cost_usd) DESC`, from, to)
	if err != nil {
		return fmt.Errorf("summarise models: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var stat ModelStat
		if err := rows.Scan(&stat.Model, &stat.Calls, &stat.InputTokens,
			&stat.OutputTokens, &stat.CacheHitTokens, &stat.CostUSD,
			&stat.UnpricedCalls); err != nil {
			return fmt.Errorf("scan model stat: %w", err)
		}
		stat.Priced = stat.UnpricedCalls == 0
		summary.ByModel = append(summary.ByModel, stat)
	}
	return rows.Err()
}

func (s *Store) fillTriggerStats(summary *Summary, from, to int64) error {
	rows, err := s.db.Query(`SELECT COALESCE(trigger_type, ''),
			COALESCE(SUM(CASE WHEN status <> 'running' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status IN ('error', 'interrupted') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(model_cost_usd), 0), COALESCE(SUM(tool_cost_usd), 0)
		FROM runs WHERE started_at BETWEEN ? AND ?
		GROUP BY COALESCE(trigger_type, '') ORDER BY COUNT(*) DESC`, from, to)
	if err != nil {
		return fmt.Errorf("summarise triggers: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var stat TriggerStat
		if err := rows.Scan(&stat.Trigger, &stat.Runs, &stat.Errors,
			&stat.InputTokens, &stat.OutputTokens, &stat.CostUSD, &stat.ToolCostUSD); err != nil {
			return fmt.Errorf("scan trigger stat: %w", err)
		}
		if stat.Trigger == "" {
			stat.Trigger = "unknown"
		}
		summary.ByTrigger = append(summary.ByTrigger, stat)
	}
	return rows.Err()
}

func (s *Store) fillToolStats(summary *Summary, from, to int64) error {
	rows, err := s.db.Query(`SELECT COALESCE(s.name, ''), COUNT(*),
			COALESCE(SUM(CASE WHEN s.status = 'error' THEN 1 ELSE 0 END), 0),
			COALESCE(AVG(s.duration_ms), 0), COALESCE(SUM(s.cost_usd), 0)
		FROM spans s JOIN runs r ON r.id = s.run_id
		WHERE s.kind = 'tool' AND r.started_at BETWEEN ? AND ?
		GROUP BY s.name ORDER BY COUNT(*) DESC`, from, to)
	if err != nil {
		return fmt.Errorf("summarise tools: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var stat ToolStat
		var avg float64
		if err := rows.Scan(&stat.Name, &stat.Calls, &stat.Errors, &avg, &stat.CostUSD); err != nil {
			return fmt.Errorf("scan tool stat: %w", err)
		}
		stat.AvgMs = int64(avg)
		summary.PerTool = append(summary.PerTool, stat)
	}
	return rows.Err()
}

func (s *Store) columnInt64(query string, args ...any) ([]int64, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("read durations: %w", err)
	}
	defer rows.Close()

	values := make([]int64, 0, 64)
	for rows.Next() {
		var value int64
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("scan duration: %w", err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) runIDBy(query string, args ...any) string {
	var id string
	if err := s.db.QueryRow(query, args...).Scan(&id); err != nil {
		if err != sql.ErrNoRows {
			return ""
		}
		return ""
	}
	return id
}

// percentile uses the nearest-rank method on an unsorted slice.
func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	rank := int(float64(len(sorted))*p/100+0.999999) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

// Session aggregates the turns that share one conversation id.
type Session struct {
	ConversationID string  `json:"conversation_id"`
	Channel        string  `json:"channel,omitempty"`
	Turns          int     `json:"turns"`
	StartedAt      int64   `json:"started_at"`
	LastActiveAt   int64   `json:"last_active_at"`
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	CostUSD        float64 `json:"cost_usd"`
	ErrorCount     int     `json:"error_count"`
	// CostComplete is false when a turn in the session used an unpriced model.
	CostComplete  bool `json:"cost_complete"`
	UnpricedTurns int  `json:"unpriced_turns"`
}

// ListSessions groups runs by conversation, newest activity first.
func (s *Store) ListSessions(limit int) ([]Session, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	rows, err := s.db.Query(`SELECT conversation_id, COALESCE(MAX(channel), ''),
			COUNT(*), MIN(started_at), MAX(started_at),
			COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(model_cost_usd), 0),
			COALESCE(SUM(CASE WHEN status IN ('error', 'interrupted') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN `+unpricedFilter+` THEN 1 ELSE 0 END), 0)
		FROM runs
		WHERE COALESCE(conversation_id, '') <> ''
		GROUP BY conversation_id
		ORDER BY MAX(started_at) DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]Session, 0, limit)
	for rows.Next() {
		var session Session
		if err := rows.Scan(&session.ConversationID, &session.Channel, &session.Turns,
			&session.StartedAt, &session.LastActiveAt, &session.InputTokens,
			&session.OutputTokens, &session.CostUSD, &session.ErrorCount,
			&session.UnpricedTurns); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		session.CostComplete = session.UnpricedTurns == 0
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}
