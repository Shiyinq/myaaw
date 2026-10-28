package obs

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const schemaVersion = 4

// schemaMigrations upgrades an existing database one version at a time. Version
// 1 is the initial schema, so its statements live in schemaStatements only.
var schemaMigrations = map[int][]string{
	2: {
		`ALTER TABLE runs ADD COLUMN parent_run_id TEXT`,
		`ALTER TABLE runs ADD COLUMN parent_span_id TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_runs_parent ON runs(parent_run_id)`,
	},
	// The loop iteration was called a turn, which collided with the Harness
	// vocabulary where a turn is one user message. Rename it to step.
	3: {
		`ALTER TABLE spans RENAME COLUMN turn_index TO step_index`,
		`UPDATE spans SET kind = 'step' WHERE kind = 'turn'`,
	},
	// Version 4 only marks that the shape is reconciled by alignSchema; it adds
	// no columns of its own.
	4: {},
}

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS runs (
		id               TEXT PRIMARY KEY,
		started_at       INTEGER NOT NULL,
		ended_at         INTEGER,
		channel          TEXT,
		user_id          TEXT,
		conversation_id  TEXT,
		provider         TEXT,
		model            TEXT,
		agent            TEXT,
		parent_run_id    TEXT,
		parent_span_id   TEXT,
		status           TEXT,
		error            TEXT,
		input_tokens     INTEGER DEFAULT 0,
		output_tokens    INTEGER DEFAULT 0,
		cache_hit_tokens INTEGER DEFAULT 0,
		model_cost_usd   REAL    DEFAULT 0,
		total_ms         INTEGER,
		first_token_ms   INTEGER,
		judge_score      REAL,
		judge_model      TEXT,
		judge_cost_usd   REAL
	)`,
	`CREATE TABLE IF NOT EXISTS spans (
		id               TEXT PRIMARY KEY,
		run_id           TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
		parent_id        TEXT,
		kind             TEXT NOT NULL,
		name             TEXT,
		step_index       INTEGER,
		request_index    INTEGER,
		started_at       INTEGER NOT NULL,
		ended_at         INTEGER,
		duration_ms      INTEGER,
		first_token_ms   INTEGER,
		generation_ms    INTEGER,
		input_tokens     INTEGER DEFAULT 0,
		output_tokens    INTEGER DEFAULT 0,
		cache_hit_tokens INTEGER DEFAULT 0,
		cost_usd         REAL    DEFAULT 0,
		status           TEXT,
		error            TEXT,
		input_json       TEXT,
		output_json      TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS checks (
		run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
		name       TEXT NOT NULL,
		status     TEXT,
		detail     TEXT,
		created_at INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_runs_started ON runs(started_at)`,
	`CREATE INDEX IF NOT EXISTS idx_runs_parent ON runs(parent_run_id)`,
	`CREATE INDEX IF NOT EXISTS idx_spans_run ON spans(run_id, started_at)`,
	`CREATE INDEX IF NOT EXISTS idx_spans_kind ON spans(kind)`,
}

// Options configures a store.
type Options struct {
	// RetentionDays drops runs older than this on open. Zero keeps everything.
	RetentionDays int
	// Now overrides the clock in tests.
	Now func() time.Time
}

// Store owns the observability SQLite database.
type Store struct {
	db            *sql.DB
	retentionDays int
	now           func() time.Time
	path          string
}

// Open creates or opens the observability database and applies the schema.
func Open(path string, opts Options) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create observability dir: %w", err)
		}
	}

	dsn := path + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open observability database: %w", err)
	}
	// One connection keeps writes ordered and avoids "database is locked"
	// between the writer goroutine and the control plane's own handle.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping observability database: %w", err)
	}

	now := opts.Now
	if now == nil {
		now = time.Now
	}
	store := &Store{db: db, retentionDays: opts.RetentionDays, now: now, path: path}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := store.MarkStaleRunning(store.now()); err != nil {
		fmt.Fprintf(os.Stderr, "[obs] stale run cleanup failed: %v\n", err)
	}
	if store.retentionDays > 0 {
		if _, err := store.Prune(); err != nil {
			// Retention is housekeeping; a failure must not disable recording.
			fmt.Fprintf(os.Stderr, "[obs] retention prune failed: %v\n", err)
		}
	}
	return store, nil
}

// Path returns the database path.
func (s *Store) Path() string { return s.path }

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read observability schema version: %w", err)
	}

	if version < schemaVersion {
		if err := s.applyMigrations(version); err != nil {
			return err
		}
	}

	// Always verify the shape. A database may have been opened by another build
	// whose vocabulary differs, which leaves user_version ahead of the columns
	// this code expects.
	if err := s.alignSchema(); err != nil {
		return err
	}
	if version < schemaVersion {
		if _, err := s.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
			return fmt.Errorf("set observability schema version: %w", err)
		}
	}
	return nil
}

func (s *Store) applyMigrations(version int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin observability migration: %w", err)
	}

	if version == 0 {
		for _, statement := range schemaStatements {
			if _, err := tx.Exec(statement); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("apply observability schema: %w", err)
			}
		}
	} else {
		for next := version + 1; next <= schemaVersion; next++ {
			for _, statement := range schemaMigrations[next] {
				if _, err := tx.Exec(statement); err != nil {
					// A rename that already happened is not an error worth failing on;
					// alignSchema below reconciles the final shape either way.
					continue
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit observability migration: %w", err)
	}
	return nil
}

// alignSchema makes the tables match what this build reads and writes. It is
// idempotent and runs on every open, because another build may have renamed a
// column and bumped user_version past this one.
func (s *Store) alignSchema() error {
	spanColumns, err := s.tableColumns("spans")
	if err != nil {
		return err
	}
	if !spanColumns["step_index"] {
		for _, legacy := range []string{"turn_index", "iteration_index"} {
			if spanColumns[legacy] {
				if _, err := s.db.Exec(fmt.Sprintf(`ALTER TABLE spans RENAME COLUMN %s TO step_index`, legacy)); err != nil {
					return fmt.Errorf("rename %s to step_index: %w", legacy, err)
				}
				break
			}
		}
	}

	if _, err := s.db.Exec(`UPDATE spans SET kind = 'step' WHERE kind IN ('turn', 'iteration')`); err != nil {
		return fmt.Errorf("normalise span kinds: %w", err)
	}

	runColumns, err := s.tableColumns("runs")
	if err != nil {
		return err
	}
	for _, column := range []string{"parent_run_id", "parent_span_id"} {
		if runColumns[column] {
			continue
		}
		if _, err := s.db.Exec(`ALTER TABLE runs ADD COLUMN ` + column + ` TEXT`); err != nil {
			return fmt.Errorf("add runs.%s: %w", column, err)
		}
	}
	return nil
}

func (s *Store) tableColumns(table string) (map[string]bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, fmt.Errorf("read %s columns: %w", table, err)
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var (
			index      int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&index, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			return nil, fmt.Errorf("scan %s column: %w", table, err)
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

const upsertRunSQL = `INSERT INTO runs (
		id, started_at, ended_at, channel, user_id, conversation_id, provider,
		model, agent, parent_run_id, parent_span_id, status, error,
		input_tokens, output_tokens, cache_hit_tokens, model_cost_usd, total_ms,
		first_token_ms
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(id) DO UPDATE SET
		ended_at = excluded.ended_at,
		channel = excluded.channel,
		user_id = excluded.user_id,
		conversation_id = excluded.conversation_id,
		provider = excluded.provider,
		model = excluded.model,
		agent = excluded.agent,
		parent_run_id = excluded.parent_run_id,
		parent_span_id = excluded.parent_span_id,
		status = excluded.status,
		error = excluded.error,
		input_tokens = excluded.input_tokens,
		output_tokens = excluded.output_tokens,
		cache_hit_tokens = excluded.cache_hit_tokens,
		model_cost_usd = excluded.model_cost_usd,
		total_ms = excluded.total_ms,
		first_token_ms = excluded.first_token_ms`

// UpsertRun inserts or updates one run row.
func (s *Store) UpsertRun(run Run) error {
	_, err := s.db.Exec(upsertRunSQL,
		run.ID, run.StartedAt, run.EndedAt, run.Channel, run.UserID,
		run.ConversationID, run.Provider, run.Model, run.Agent,
		run.ParentRunID, run.ParentSpanID, string(run.Status),
		run.Error, run.InputTokens, run.OutputTokens, run.CacheHitTokens,
		run.ModelCostUSD, run.TotalMs, run.FirstTokenMs,
	)
	if err != nil {
		return fmt.Errorf("upsert observability run: %w", err)
	}
	return nil
}

const upsertSpanSQL = `INSERT INTO spans (
		id, run_id, parent_id, kind, name, step_index, request_index,
		started_at, ended_at, duration_ms, first_token_ms, generation_ms,
		input_tokens, output_tokens, cache_hit_tokens, cost_usd, status, error,
		input_json, output_json
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(id) DO UPDATE SET
		ended_at = excluded.ended_at,
		duration_ms = excluded.duration_ms,
		first_token_ms = excluded.first_token_ms,
		generation_ms = excluded.generation_ms,
		input_tokens = excluded.input_tokens,
		output_tokens = excluded.output_tokens,
		cache_hit_tokens = excluded.cache_hit_tokens,
		cost_usd = excluded.cost_usd,
		status = excluded.status,
		error = excluded.error,
		input_json = excluded.input_json,
		output_json = excluded.output_json`

// UpsertSpans writes a batch of spans in one transaction.
func (s *Store) UpsertSpans(spans []Span) error {
	if len(spans) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin span batch: %w", err)
	}
	statement, err := tx.Prepare(upsertSpanSQL)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare span batch: %w", err)
	}
	defer statement.Close()

	for _, span := range spans {
		if _, err := statement.Exec(
			span.ID, span.RunID, span.ParentID, string(span.Kind), span.Name,
			span.StepIndex, span.RequestIndex, span.StartedAt, span.EndedAt,
			span.DurationMs, span.FirstTokenMs, span.GenerationMs,
			span.InputTokens, span.OutputTokens, span.CacheHitTokens,
			span.CostUSD, string(span.Status), span.Error,
			span.InputJSON, span.OutputJSON,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("upsert span %s: %w", span.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit span batch: %w", err)
	}
	return nil
}

// runColumns reads every nullable column through COALESCE so a row written by
// an older build (or hand-edited) still scans.
const runColumns = `id, started_at, COALESCE(ended_at, 0), COALESCE(channel, ''),
	COALESCE(user_id, ''), COALESCE(conversation_id, ''), COALESCE(provider, ''),
	COALESCE(model, ''), COALESCE(agent, ''), COALESCE(parent_run_id, ''),
	COALESCE(parent_span_id, ''), COALESCE(status, ''), COALESCE(error, ''),
	COALESCE(input_tokens, 0), COALESCE(output_tokens, 0),
	COALESCE(cache_hit_tokens, 0), COALESCE(model_cost_usd, 0),
	COALESCE(total_ms, 0), COALESCE(first_token_ms, 0)`

const spanColumns = `id, COALESCE(run_id, ''), COALESCE(parent_id, ''), COALESCE(kind, ''),
	COALESCE(name, ''), COALESCE(step_index, 0), COALESCE(request_index, 0),
	started_at, COALESCE(ended_at, 0), COALESCE(duration_ms, 0),
	COALESCE(first_token_ms, 0), COALESCE(generation_ms, 0),
	COALESCE(input_tokens, 0), COALESCE(output_tokens, 0),
	COALESCE(cache_hit_tokens, 0), COALESCE(cost_usd, 0), COALESCE(status, ''),
	COALESCE(error, ''), COALESCE(input_json, ''), COALESCE(output_json, '')`

type scanner interface {
	Scan(dest ...any) error
}

func scanRun(row scanner) (Run, error) {
	var run Run
	var status string
	err := row.Scan(
		&run.ID, &run.StartedAt, &run.EndedAt, &run.Channel, &run.UserID,
		&run.ConversationID, &run.Provider, &run.Model, &run.Agent,
		&run.ParentRunID, &run.ParentSpanID, &status,
		&run.Error, &run.InputTokens, &run.OutputTokens, &run.CacheHitTokens,
		&run.ModelCostUSD, &run.TotalMs, &run.FirstTokenMs,
	)
	run.Status = Status(status)
	return run, err
}

func scanSpan(row scanner) (Span, error) {
	var span Span
	var kind, status string
	err := row.Scan(
		&span.ID, &span.RunID, &span.ParentID, &kind, &span.Name,
		&span.StepIndex, &span.RequestIndex, &span.StartedAt, &span.EndedAt,
		&span.DurationMs, &span.FirstTokenMs, &span.GenerationMs,
		&span.InputTokens, &span.OutputTokens, &span.CacheHitTokens,
		&span.CostUSD, &status, &span.Error, &span.InputJSON, &span.OutputJSON,
	)
	span.Kind = SpanKind(kind)
	span.Status = Status(status)
	return span, err
}

// GetRun returns one run and its spans ordered by start time.
func (s *Store) GetRun(id string) (Run, []Span, error) {
	row := s.db.QueryRow(`SELECT `+runColumns+` FROM runs WHERE id = ?`, id)
	run, err := scanRun(row)
	if err == sql.ErrNoRows {
		return Run{}, nil, ErrNotFound
	}
	if err != nil {
		return Run{}, nil, fmt.Errorf("read run: %w", err)
	}

	rows, err := s.db.Query(
		`SELECT `+spanColumns+` FROM spans
		 WHERE run_id = ? OR run_id IN (SELECT id FROM runs WHERE parent_run_id = ?)
		 ORDER BY started_at, id`, id, id)
	if err != nil {
		return Run{}, nil, fmt.Errorf("read spans: %w", err)
	}
	defer rows.Close()

	spans := make([]Span, 0, 16)
	for rows.Next() {
		span, err := scanSpan(rows)
		if err != nil {
			return Run{}, nil, fmt.Errorf("scan span: %w", err)
		}
		spans = append(spans, span)
	}
	if err := rows.Err(); err != nil {
		return Run{}, nil, fmt.Errorf("iterate spans: %w", err)
	}
	return run, spans, nil
}

// ErrNotFound reports a missing run.
var ErrNotFound = fmt.Errorf("run not found")

// RunFilter narrows ListRuns.
type RunFilter struct {
	Limit          int
	Offset         int
	Status         string
	Channel        string
	ConversationID string
	From           int64
	To             int64
}

// ListRuns returns runs newest first plus the total matching count.
func (s *Store) ListRuns(filter RunFilter) ([]Run, int, error) {
	where, args := runWhere(filter)

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM runs`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count runs: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := `SELECT ` + runColumns + ` FROM runs` + where +
		` ORDER BY started_at DESC LIMIT ? OFFSET ?`
	rows, err := s.db.Query(query, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	runs := make([]Run, 0, limit)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate runs: %w", err)
	}
	return runs, total, nil
}

func runWhere(filter RunFilter) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 4)

	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.Channel != "" {
		clauses = append(clauses, "channel = ?")
		args = append(args, filter.Channel)
	}
	if filter.ConversationID != "" {
		clauses = append(clauses, "conversation_id = ?")
		args = append(args, filter.ConversationID)
	}
	if filter.From > 0 {
		clauses = append(clauses, "started_at >= ?")
		args = append(args, filter.From)
	}
	if filter.To > 0 {
		clauses = append(clauses, "started_at <= ?")
		args = append(args, filter.To)
	}
	if len(clauses) == 0 {
		return "", args
	}
	where := " WHERE "
	for i, clause := range clauses {
		if i > 0 {
			where += " AND "
		}
		where += clause
	}
	return where, args
}

// SpansForRuns loads the spans of each run in one pass, including the spans of
// runs nested under them (subagents). The result is keyed by the requested run
// id, so a nested run's spans travel with the turn that started it.
func (s *Store) SpansForRuns(runIDs []string) (map[string][]Span, error) {
	result := make(map[string][]Span, len(runIDs))
	if len(runIDs) == 0 {
		return result, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(runIDs)), ",")
	args := make([]any, len(runIDs))
	index := make(map[string]string, len(runIDs))
	for i, id := range runIDs {
		args[i] = id
		index[id] = id
		result[id] = []Span{}
	}

	// Nested runs belong to the turn that spawned them.
	childArgs := make([]any, len(runIDs))
	copy(childArgs, args)
	childRows, err := s.db.Query(`SELECT id, parent_run_id FROM runs WHERE parent_run_id IN (`+placeholders+`)`, childArgs...)
	if err != nil {
		return nil, fmt.Errorf("read nested runs: %w", err)
	}
	for childRows.Next() {
		var childID, parentID string
		if err := childRows.Scan(&childID, &parentID); err != nil {
			childRows.Close()
			return nil, fmt.Errorf("scan nested run: %w", err)
		}
		index[childID] = parentID
	}
	if err := childRows.Err(); err != nil {
		childRows.Close()
		return nil, fmt.Errorf("iterate nested runs: %w", err)
	}
	childRows.Close()

	spanArgs := make([]any, 0, len(runIDs)*2)
	spanArgs = append(spanArgs, args...)
	spanArgs = append(spanArgs, args...)
	rows, err := s.db.Query(`SELECT `+spanColumns+` FROM spans
		WHERE run_id IN (`+placeholders+`)
		   OR run_id IN (SELECT id FROM runs WHERE parent_run_id IN (`+placeholders+`))
		ORDER BY started_at, id`, spanArgs...)
	if err != nil {
		return nil, fmt.Errorf("read session spans: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		span, err := scanSpan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan session span: %w", err)
		}
		owner, ok := index[span.RunID]
		if !ok {
			continue
		}
		result[owner] = append(result[owner], span)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session spans: %w", err)
	}
	return result, nil
}

// StepCounts returns how many step spans each run has. Unknown ids are absent
// from the map rather than zero.
func (s *Store) StepCounts(runIDs []string) (map[string]int, error) {
	counts := make(map[string]int, len(runIDs))
	if len(runIDs) == 0 {
		return counts, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(runIDs)), ",")
	args := make([]any, len(runIDs))
	for i, id := range runIDs {
		args[i] = id
	}

	rows, err := s.db.Query(`SELECT run_id, COUNT(*) FROM spans
		WHERE kind = 'step' AND run_id IN (`+placeholders+`)
		GROUP BY run_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("count steps: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var runID string
		var count int
		if err := rows.Scan(&runID, &count); err != nil {
			return nil, fmt.Errorf("scan step count: %w", err)
		}
		counts[runID] = count
	}
	return counts, rows.Err()
}

// DeleteBefore drops runs (and their spans) that started before ts and returns
// how many runs were removed.
func (s *Store) DeleteBefore(ts int64) (int64, error) {
	result, err := s.db.Exec(`DELETE FROM runs WHERE started_at < ?`, ts)
	if err != nil {
		return 0, fmt.Errorf("delete runs: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted runs: %w", err)
	}
	return removed, nil
}

// staleRunAfter is how long a run may stay "running" before a restart marks it
// interrupted. It stays above the maximum agent run timeout so a live run is
// never reclassified.
const staleRunAfter = 2 * time.Hour

// MarkStaleRunning closes runs left open by a crash or a killed process, and the
// spans that were still running inside them.
func (s *Store) MarkStaleRunning(now time.Time) (int64, error) {
	cutoff := now.Add(-staleRunAfter).UnixMilli()

	if _, err := s.db.Exec(`UPDATE spans SET status = ? WHERE status = ? AND run_id IN (
			SELECT id FROM runs WHERE status = ? AND started_at < ?)`,
		string(StatusInterrupted), string(StatusRunning), string(StatusRunning), cutoff); err != nil {
		return 0, fmt.Errorf("close stale spans: %w", err)
	}

	result, err := s.db.Exec(`UPDATE runs SET status = ?, error = ? WHERE status = ? AND started_at < ?`,
		string(StatusInterrupted), "run did not finish; the process stopped before it ended",
		string(StatusRunning), cutoff)
	if err != nil {
		return 0, fmt.Errorf("close stale runs: %w", err)
	}
	return result.RowsAffected()
}

// Prune applies the configured retention window.
func (s *Store) Prune() (int64, error) {
	if s.retentionDays <= 0 {
		return 0, nil
	}
	cutoff := s.now().Add(-time.Duration(s.retentionDays) * 24 * time.Hour).UnixMilli()
	return s.DeleteBefore(cutoff)
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
