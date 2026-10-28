package obs

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T, opts Options) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "observability.db"), opts)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func sampleRun(id string, startedAt int64) Run {
	return Run{
		ID:             id,
		StartedAt:      startedAt,
		EndedAt:        startedAt + 1000,
		Channel:        "telegram",
		UserID:         "42",
		ConversationID: "conv-1",
		Provider:       "deepseek",
		Model:          "deepseek-flash",
		Agent:          "default",
		Status:         StatusCompleted,
		InputTokens:    1200,
		OutputTokens:   300,
		CacheHitTokens: 900,
		ModelCostUSD:   0.0004,
		TotalMs:        1000,
		FirstTokenMs:   400,
	}
}

func TestUpsertAndReadRun(t *testing.T) {
	store := newTestStore(t, Options{})
	run := sampleRun("run_1", 1_700_000_000_000)

	if err := store.UpsertRun(run); err != nil {
		t.Fatalf("upsert run: %v", err)
	}

	span := Span{
		ID: "sp_1", RunID: run.ID, Kind: KindLLM, Name: "deepseek-flash",
		StepIndex: 1, RequestIndex: 1, StartedAt: run.StartedAt, EndedAt: run.StartedAt + 900,
		DurationMs: 900, FirstTokenMs: 400, GenerationMs: 500,
		InputTokens: 1200, OutputTokens: 300, CacheHitTokens: 900,
		CostUSD: 0.0004, Status: StatusCompleted,
		InputJSON: `{"messages":[]}`, OutputJSON: `{"content":"hi"}`,
	}
	if err := store.UpsertSpans([]Span{span}); err != nil {
		t.Fatalf("upsert spans: %v", err)
	}

	gotRun, spans, err := store.GetRun(run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if gotRun.Model != run.Model || gotRun.Channel != "telegram" || gotRun.Status != StatusCompleted {
		t.Fatalf("run = %+v", gotRun)
	}
	if gotRun.ModelCostUSD != run.ModelCostUSD {
		t.Fatalf("cost = %v, want %v", gotRun.ModelCostUSD, run.ModelCostUSD)
	}
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Kind != KindLLM || spans[0].InputJSON == "" || spans[0].FirstTokenMs != 400 {
		t.Fatalf("span = %+v", spans[0])
	}
}

func TestUpsertRunUpdatesExistingRow(t *testing.T) {
	store := newTestStore(t, Options{})
	run := sampleRun("run_1", 1_700_000_000_000)
	if err := store.UpsertRun(run); err != nil {
		t.Fatalf("upsert run: %v", err)
	}

	run.Status = StatusError
	run.Error = "provider exploded"
	run.TotalMs = 2000
	if err := store.UpsertRun(run); err != nil {
		t.Fatalf("update run: %v", err)
	}

	got, _, err := store.GetRun(run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != StatusError || got.Error != "provider exploded" || got.TotalMs != 2000 {
		t.Fatalf("run = %+v", got)
	}
}

func TestListRunsFiltersByModelAndTrigger(t *testing.T) {
	store := newTestStore(t, Options{})
	base := int64(1_700_000_000_000)

	user := sampleRun("run_user", base)
	user.Trigger = "user"
	cron := sampleRun("run_cron", base+1000)
	cron.Trigger = "cron"
	cron.Model = "gemini-3.1-pro-preview"
	child := sampleRun("run_child", base+2000)
	child.Trigger = "subagent"
	child.Model = "gemini-3.1-pro-preview"

	for _, run := range []Run{user, cron, child} {
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
	}

	byTrigger, _, err := store.ListRuns(RunFilter{Trigger: "cron"})
	if err != nil {
		t.Fatalf("filter by trigger: %v", err)
	}
	if len(byTrigger) != 1 || byTrigger[0].ID != "run_cron" {
		t.Fatalf("cron runs = %+v", byTrigger)
	}

	byModel, _, err := store.ListRuns(RunFilter{Model: "gemini-3.1-pro-preview"})
	if err != nil {
		t.Fatalf("filter by model: %v", err)
	}
	if len(byModel) != 2 {
		t.Fatalf("model runs = %+v", byModel)
	}

	combined, _, err := store.ListRuns(RunFilter{Model: "gemini-3.1-pro-preview", Trigger: "subagent"})
	if err != nil {
		t.Fatalf("combined filter: %v", err)
	}
	if len(combined) != 1 || combined[0].ID != "run_child" {
		t.Fatalf("combined = %+v", combined)
	}

	read, _, err := store.GetRun("run_cron")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if read.Trigger != "cron" {
		t.Fatalf("trigger = %q, want it to round-trip", read.Trigger)
	}
}

// A database from before the trigger column must gain it on open.
func TestOpenAddsTriggerColumnToOlderDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability.db")

	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	for _, statement := range schemaStatements {
		if _, err := raw.Exec(statement); err != nil {
			t.Fatalf("apply schema: %v", err)
		}
	}
	if _, err := raw.Exec(`ALTER TABLE runs DROP COLUMN trigger_type`); err != nil {
		t.Fatalf("drop trigger_type: %v", err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 4`); err != nil {
		t.Fatalf("set version: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO runs (id, started_at, status, parent_run_id) VALUES ('run_root', 1700000000000, 'completed', '')`); err != nil {
		t.Fatalf("seed root: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO runs (id, started_at, status, parent_run_id) VALUES ('run_child', 1700000000000, 'completed', 'run_root')`); err != nil {
		t.Fatalf("seed child: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	store, err := Open(path, Options{})
	if err != nil {
		t.Fatalf("open upgraded store: %v", err)
	}
	defer store.Close()

	columns, err := store.tableColumns("runs")
	if err != nil {
		t.Fatalf("read columns: %v", err)
	}
	if !columns["trigger_type"] {
		t.Fatalf("trigger_type was not added: %v", columns)
	}

	root, _, err := store.GetRun("run_root")
	if err != nil {
		t.Fatalf("get root: %v", err)
	}
	if root.Trigger != "" {
		t.Fatalf("root trigger = %q, want it left unknown", root.Trigger)
	}
	child, _, err := store.GetRun("run_child")
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	if child.Trigger != "subagent" {
		t.Fatalf("child trigger = %q, want subagent from the parent link", child.Trigger)
	}

	run := sampleRun("run_cron", 1_700_000_000_000)
	run.Trigger = "cron"
	if err := store.UpsertRun(run); err != nil {
		t.Fatalf("upsert run: %v", err)
	}
	read, _, err := store.GetRun(run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if read.Trigger != "cron" {
		t.Fatalf("trigger = %q", read.Trigger)
	}
}

func TestSizeAndVacuum(t *testing.T) {
	store := newTestStore(t, Options{})
	if err := store.UpsertRun(sampleRun("run_1", 1_700_000_000_000)); err != nil {
		t.Fatalf("upsert run: %v", err)
	}
	if size := store.Size(); size <= 0 {
		t.Fatalf("size = %d, want a positive file size", size)
	}
	if err := store.Vacuum(); err != nil {
		t.Fatalf("vacuum: %v", err)
	}
}

func TestListRunsFiltersAndPaginates(t *testing.T) {
	store := newTestStore(t, Options{})
	base := int64(1_700_000_000_000)

	runs := []Run{
		sampleRun("run_old", base),
		sampleRun("run_mid", base+1000),
		sampleRun("run_new", base+2000),
	}
	runs[1].Status = StatusError
	runs[2].Channel = "discord"
	for _, run := range runs {
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
	}

	all, total, err := store.ListRuns(RunFilter{})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("total = %d, len = %d", total, len(all))
	}
	if all[0].ID != "run_new" {
		t.Fatalf("newest first, got %q", all[0].ID)
	}

	page, total, err := store.ListRuns(RunFilter{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("list page: %v", err)
	}
	if total != 3 || len(page) != 1 || page[0].ID != "run_mid" {
		t.Fatalf("page = %+v total = %d", page, total)
	}

	errorsOnly, _, err := store.ListRuns(RunFilter{Status: string(StatusError)})
	if err != nil {
		t.Fatalf("list by status: %v", err)
	}
	if len(errorsOnly) != 1 || errorsOnly[0].ID != "run_mid" {
		t.Fatalf("errors = %+v", errorsOnly)
	}

	discord, _, err := store.ListRuns(RunFilter{Channel: "discord"})
	if err != nil {
		t.Fatalf("list by channel: %v", err)
	}
	if len(discord) != 1 || discord[0].ID != "run_new" {
		t.Fatalf("discord = %+v", discord)
	}
}

func TestDeleteBeforeCascadesSpans(t *testing.T) {
	store := newTestStore(t, Options{})
	base := int64(1_700_000_000_000)

	old := sampleRun("run_old", base)
	fresh := sampleRun("run_new", base+10_000)
	for _, run := range []Run{old, fresh} {
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
		if err := store.UpsertSpans([]Span{{
			ID: "sp_" + run.ID, RunID: run.ID, Kind: KindLLM,
			StartedAt: run.StartedAt, Status: StatusCompleted,
		}}); err != nil {
			t.Fatalf("upsert span: %v", err)
		}
	}

	removed, err := store.DeleteBefore(base + 5000)
	if err != nil {
		t.Fatalf("delete before: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}

	if _, _, err := store.GetRun(old.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old run error = %v, want ErrNotFound", err)
	}

	_, spans, err := store.GetRun(fresh.ID)
	if err != nil {
		t.Fatalf("get fresh run: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("fresh spans = %d, want 1", len(spans))
	}

	rows, err := store.db.Query(`SELECT COUNT(*) FROM spans WHERE run_id = ?`, old.ID)
	if err != nil {
		t.Fatalf("count spans: %v", err)
	}
	defer rows.Close()
	rows.Next()
	var count int
	if err := rows.Scan(&count); err != nil {
		t.Fatalf("scan count: %v", err)
	}
	if count != 0 {
		t.Fatalf("orphaned spans = %d, want 0", count)
	}
}

func TestOpenPrunesExpiredRuns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "observability.db")

	store, err := Open(path, Options{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	expired := sampleRun("run_expired", now.Add(-31*24*time.Hour).UnixMilli())
	kept := sampleRun("run_kept", now.Add(-24*time.Hour).UnixMilli())
	if err := store.UpsertRun(expired); err != nil {
		t.Fatalf("upsert expired: %v", err)
	}
	if err := store.UpsertRun(kept); err != nil {
		t.Fatalf("upsert kept: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened, err := Open(path, Options{RetentionDays: 30, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()

	if _, _, err := reopened.GetRun(expired.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired run error = %v, want ErrNotFound", err)
	}
	if _, _, err := reopened.GetRun(kept.ID); err != nil {
		t.Fatalf("kept run error = %v", err)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability.db")

	first, err := Open(path, Options{})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	second, err := Open(path, Options{})
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer second.Close()

	if err := second.UpsertRun(sampleRun("run_1", time.Now().UnixMilli())); err != nil {
		t.Fatalf("upsert after reopen: %v", err)
	}
}

func TestMarkStaleRunningClosesAbandonedRuns(t *testing.T) {
	store := newTestStore(t, Options{})
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

	abandoned := sampleRun("run_abandoned", now.Add(-3*time.Hour).UnixMilli())
	abandoned.Status = StatusRunning
	abandoned.EndedAt = 0
	abandoned.TotalMs = 0

	live := sampleRun("run_live", now.Add(-1*time.Minute).UnixMilli())
	live.Status = StatusRunning
	live.EndedAt = 0
	live.TotalMs = 0

	for _, run := range []Run{abandoned, live} {
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
		if err := store.UpsertSpans([]Span{{
			ID: "sp_" + run.ID, RunID: run.ID, Kind: KindLLM,
			StartedAt: run.StartedAt, Status: StatusRunning,
		}}); err != nil {
			t.Fatalf("upsert span: %v", err)
		}
	}

	closed, err := store.MarkStaleRunning(now)
	if err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	if closed != 1 {
		t.Fatalf("closed = %d, want 1", closed)
	}

	staleRun, staleSpans, err := store.GetRun(abandoned.ID)
	if err != nil {
		t.Fatalf("get abandoned: %v", err)
	}
	if staleRun.Status != StatusInterrupted || staleRun.Error == "" {
		t.Fatalf("abandoned run = %+v", staleRun)
	}
	if staleSpans[0].Status != StatusInterrupted {
		t.Fatalf("abandoned span status = %q", staleSpans[0].Status)
	}

	liveRun, liveSpans, err := store.GetRun(live.ID)
	if err != nil {
		t.Fatalf("get live: %v", err)
	}
	if liveRun.Status != StatusRunning {
		t.Fatalf("live run status = %q, want running", liveRun.Status)
	}
	if liveSpans[0].Status != StatusRunning {
		t.Fatalf("live span status = %q", liveSpans[0].Status)
	}
}

func TestOpenMigratesVersionOneDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability.db")

	// Build a v1 database: the current schema without the nesting columns.
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	for _, statement := range schemaStatements {
		if strings.Contains(statement, "idx_runs_parent") {
			continue
		}
		if _, err := raw.Exec(statement); err != nil {
			t.Fatalf("apply v1 schema: %v", err)
		}
	}
	if _, err := raw.Exec(`ALTER TABLE runs DROP COLUMN parent_run_id`); err != nil {
		t.Fatalf("drop parent_run_id: %v", err)
	}
	if _, err := raw.Exec(`ALTER TABLE runs DROP COLUMN parent_span_id`); err != nil {
		t.Fatalf("drop parent_span_id: %v", err)
	}
	// Version 1 called a loop iteration a turn.
	if _, err := raw.Exec(`ALTER TABLE spans RENAME COLUMN step_index TO turn_index`); err != nil {
		t.Fatalf("restore turn_index: %v", err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatalf("set version: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO runs (id, started_at, status) VALUES ('run_old', 1700000000000, 'completed')`); err != nil {
		t.Fatalf("seed v1 run: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO spans (id, run_id, kind, name, turn_index, started_at, status)
			VALUES ('sp_old', 'run_old', 'turn', 'Turn 1', 1, 1700000000000, 'completed')`); err != nil {
		t.Fatalf("seed v1 span: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	store, err := Open(path, Options{})
	if err != nil {
		t.Fatalf("migrate open: %v", err)
	}
	defer store.Close()

	var version int
	if err := store.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("schema version = %d, want %d", version, schemaVersion)
	}

	old, _, err := store.GetRun("run_old")
	if err != nil {
		t.Fatalf("existing run was lost: %v", err)
	}
	if old.ID != "run_old" || old.Status != StatusCompleted {
		t.Fatalf("existing run = %+v", old)
	}

	_, migratedSpans, err := store.GetRun("run_old")
	if err != nil {
		t.Fatalf("read migrated spans: %v", err)
	}
	if len(migratedSpans) != 1 {
		t.Fatalf("migrated spans = %d, want 1", len(migratedSpans))
	}
	if migratedSpans[0].Kind != KindStep || migratedSpans[0].StepIndex != 1 {
		t.Fatalf("span was not migrated to step: %+v", migratedSpans[0])
	}

	nested := sampleRun("run_child", 1_700_000_001_000)
	nested.ParentRunID = "run_old"
	nested.ParentSpanID = "sp_parent"
	if err := store.UpsertRun(nested); err != nil {
		t.Fatalf("write after migration: %v", err)
	}
	got, _, err := store.GetRun(nested.ID)
	if err != nil {
		t.Fatalf("read nested run: %v", err)
	}
	if got.ParentRunID != "run_old" || got.ParentSpanID != "sp_parent" {
		t.Fatalf("nested links = %q / %q", got.ParentRunID, got.ParentSpanID)
	}
}

func TestListRunsFiltersByConversation(t *testing.T) {
	store := newTestStore(t, Options{})
	base := int64(1_700_000_000_000)

	first := sampleRun("run_1", base)
	first.ConversationID = "session-a"
	second := sampleRun("run_2", base+1000)
	second.ConversationID = "session-a"
	other := sampleRun("run_3", base+2000)
	other.ConversationID = "session-b"

	for _, run := range []Run{first, second, other} {
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
	}

	runs, total, err := store.ListRuns(RunFilter{ConversationID: "session-a"})
	if err != nil {
		t.Fatalf("list by conversation: %v", err)
	}
	if total != 2 || len(runs) != 2 {
		t.Fatalf("total = %d, runs = %d, want 2", total, len(runs))
	}
	for _, run := range runs {
		if run.ConversationID != "session-a" {
			t.Fatalf("run %s leaked from another session", run.ID)
		}
	}
}

// A database written by a build that used different vocabulary must still open.
func TestOpenRepairsForeignKeyVocabulary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability.db")

	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	for _, statement := range schemaStatements {
		if _, err := raw.Exec(statement); err != nil {
			t.Fatalf("apply schema: %v", err)
		}
	}
	if _, err := raw.Exec(`ALTER TABLE spans RENAME COLUMN step_index TO iteration_index`); err != nil {
		t.Fatalf("rename to iteration_index: %v", err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 4`); err != nil {
		t.Fatalf("set version: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO runs (id, started_at, status) VALUES ('run_x', 1700000000000, 'completed')`); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO spans (id, run_id, kind, iteration_index, started_at, status)
			VALUES ('sp_x', 'run_x', 'iteration', 3, 1700000000000, 'completed')`); err != nil {
		t.Fatalf("seed span: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	store, err := Open(path, Options{})
	if err != nil {
		t.Fatalf("open repaired store: %v", err)
	}
	defer store.Close()

	columns, err := store.tableColumns("spans")
	if err != nil {
		t.Fatalf("read columns: %v", err)
	}
	if !columns["step_index"] {
		t.Fatalf("step_index was not restored: %v", columns)
	}

	_, spans, err := store.GetRun("run_x")
	if err != nil {
		t.Fatalf("read seeded span: %v", err)
	}
	if len(spans) != 1 || spans[0].StepIndex != 3 || spans[0].Kind != KindStep {
		t.Fatalf("span was not normalised: %+v", spans)
	}

	// Writing must work again, which is what the daemon was failing to do.
	if err := store.UpsertSpans([]Span{{
		ID: "sp_new", RunID: "run_x", Kind: KindStep, StepIndex: 1,
		StartedAt: 1700000000000, Status: StatusCompleted,
	}}); err != nil {
		t.Fatalf("write after repair: %v", err)
	}
}

func TestStepCounts(t *testing.T) {
	store := newTestStore(t, Options{})
	base := int64(1_700_000_000_000)

	for _, id := range []string{"run_a", "run_b"} {
		run := sampleRun(id, base)
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
	}
	spans := []Span{
		{ID: "a1", RunID: "run_a", Kind: KindStep, StepIndex: 1, StartedAt: base, Status: StatusCompleted},
		{ID: "a2", RunID: "run_a", Kind: KindStep, StepIndex: 2, StartedAt: base, Status: StatusCompleted},
		{ID: "a3", RunID: "run_a", Kind: KindLLM, StepIndex: 1, StartedAt: base, Status: StatusCompleted},
		{ID: "b1", RunID: "run_b", Kind: KindLLM, StepIndex: 1, StartedAt: base, Status: StatusCompleted},
	}
	if err := store.UpsertSpans(spans); err != nil {
		t.Fatalf("upsert spans: %v", err)
	}

	counts, err := store.StepCounts([]string{"run_a", "run_b", "run_missing"})
	if err != nil {
		t.Fatalf("step counts: %v", err)
	}
	if counts["run_a"] != 2 {
		t.Fatalf("run_a steps = %d, want 2", counts["run_a"])
	}
	if _, ok := counts["run_b"]; ok {
		t.Fatalf("run_b has no step spans, got %d", counts["run_b"])
	}
	if _, ok := counts["run_missing"]; ok {
		t.Fatal("an unknown run must be absent from the map")
	}

	empty, err := store.StepCounts(nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty input = %v, err = %v", empty, err)
	}
}

func TestSpansForRunsIncludesNestedRuns(t *testing.T) {
	store := newTestStore(t, Options{})
	base := int64(1_700_000_000_000)

	parent := sampleRun("run_parent", base)
	parent.ConversationID = "conv-1"
	child := sampleRun("run_child", base+10)
	child.ConversationID = ""
	child.ParentRunID = parent.ID
	child.ParentSpanID = "sp_tool"
	for _, run := range []Run{parent, child} {
		if err := store.UpsertRun(run); err != nil {
			t.Fatalf("upsert run: %v", err)
		}
	}

	spans := []Span{
		{ID: "sp_user", RunID: parent.ID, Kind: KindUser, StartedAt: base, InputJSON: `{"m":1}`},
		{ID: "sp_tool", RunID: parent.ID, Kind: KindTool, Name: "subagent", StartedAt: base + 5, InputJSON: `{"task":"x"}`},
		{ID: "sp_child_run", RunID: child.ID, ParentID: "sp_tool", Kind: KindRun, StartedAt: base + 10},
		{ID: "sp_child_llm", RunID: child.ID, ParentID: "sp_child_run", Kind: KindLLM, StartedAt: base + 11, OutputJSON: `{"content":"y"}`},
	}
	if err := store.UpsertSpans(spans); err != nil {
		t.Fatalf("upsert spans: %v", err)
	}

	byRun, err := store.SpansForRuns([]string{parent.ID})
	if err != nil {
		t.Fatalf("spans for runs: %v", err)
	}
	got := byRun[parent.ID]
	if len(got) != 4 {
		t.Fatalf("spans = %d, want the parent's 2 plus the nested run's 2", len(got))
	}
	var nested bool
	for _, span := range got {
		if span.RunID == child.ID {
			nested = true
		}
	}
	if !nested {
		t.Fatal("nested run spans did not travel with the parent turn")
	}

	empty, err := store.SpansForRuns(nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty input = %v, err = %v", empty, err)
	}
}
