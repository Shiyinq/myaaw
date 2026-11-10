package cron

import (
	"strings"
	"testing"
	"time"
)

// A removed one-time job keeps its run file but not its jobs.json entry, so the
// log itself is what resolves an id.
func TestMatchRunIDs(t *testing.T) {
	logger := NewHistoryLogger(t.TempDir())

	first := Job{ID: "11111111-2222-4333-8444-555555555555", Name: "Water reminder"}
	second := Job{ID: "11111112-2222-4333-8444-555555555555", Name: "Daily Report"}
	for _, job := range []Job{first, second} {
		if err := logger.Log(job, "success", "Job executed successfully"); err != nil {
			t.Fatalf("log %s: %v", job.Name, err)
		}
	}

	cases := []struct {
		name   string
		prefix string
		want   []string
	}{
		{name: "exact", prefix: first.ID, want: []string{first.ID}},
		{name: "shared prefix", prefix: "11111111", want: []string{first.ID}},
		{name: "ambiguous", prefix: "1111111", want: []string{first.ID, second.ID}},
		{name: "missing", prefix: "deadbeef", want: nil},
		{name: "empty", prefix: "", want: nil},
	}

	for _, tc := range cases {
		got, err := logger.MatchRunIDs(tc.prefix)
		if err != nil {
			t.Fatalf("%s: MatchRunIDs: %v", tc.name, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
			}
		}
	}
}

// A colored cell must not shift the columns after it. The escape bytes are not
// drawn, so padding by them is what used to misalign the history table.
func TestAlignTablePadsByVisibleWidth(t *testing.T) {
	const green = "\x1b[32msuccess\x1b[0m"
	const red = "\x1b[31mfailed\x1b[0m"

	header := []string{"Time", "Status", "Result"}
	rows := [][]string{
		{"2026-11-05 13:18:52", green, "Job executed successfully"},
		{"2026-11-05 12:03:10", green, "OK"},
		{"2026-11-05 11:00:00", red, "Server returned error: 502 Bad Gateway"},
	}

	table := AlignTable(header, rows)
	lines := strings.Split(strings.TrimRight(table, "\n"), "\n")
	if len(lines) != len(rows)+1 {
		t.Fatalf("lines = %d, want %d\n%s", len(lines), len(rows)+1, table)
	}

	// Every row's last cell must begin at the same visible column.
	want := -1
	for i, row := range rows {
		at := strings.LastIndex(stripANSI(lines[i+1]), row[len(row)-1])
		if want == -1 {
			want = at
		}
		if at != want {
			t.Fatalf("row %d starts at %d, want %d\n%s", i, at, want, table)
		}
	}
	if at := strings.LastIndex(stripANSI(lines[0]), "Result"); at != want {
		t.Fatalf("header starts at %d, want %d\n%s", at, want, table)
	}
	if visibleLen(green) != len("success") {
		t.Fatalf("visibleLen counts escape bytes: %d", visibleLen(green))
	}
}

// A run carries a snapshot of the job, and the detail has to show every field of
// it, not a summary: channel, recipient, chat type, topic, and the full session id.
func TestRunDetailShowsEveryField(t *testing.T) {
	run := JobExecutionResult{
		JobID: "11111111-2222-4333-8444-555555555555",
		ExecutionResult: ExecutionResult{
			Timestamp: time.Date(2026, 11, 5, 13, 18, 52, 0, time.UTC),
			Status:    "success",
			Result:    "Job executed successfully",
			Name:      "Water reminder",
			Message:   "Time to drink water",
			Channel:   "telegram",
			To:        "-1001234567890",
			ThreadID:  "77",
			SessionID: "0f4e8b3a-1111-4222-8333-944455556666",
			ChatType:  "guild",
		},
	}

	got := RunDetail(run)
	for _, want := range [][2]string{
		{"Time", "2026-11-05T13:18:52Z"},
		{"Job ID", "11111111-2222-4333-8444-555555555555"},
		{"Job", "Water reminder"},
		{"Status", "success"},
		{"Result", "Job executed successfully"},
		{"Prompt", "Time to drink water"},
		{"Channel", "telegram"},
		{"To", "-1001234567890"},
		{"Chat type", "guild"},
		{"Topic", "77"},
		{"Session", "0f4e8b3a-1111-4222-8333-944455556666"},
	} {
		if !hasDetailLine(got, want[0], want[1]) {
			t.Errorf("detail is missing %s %q:\n%s", want[0], want[1], got)
		}
	}
	if strings.Contains(got, "recorded before") {
		t.Errorf("snapshot entry was reported as old:\n%s", got)
	}
}

// The detail prints the timestamp exactly as the log stores it, including the
// original offset and sub-second precision, so a line can be traced back to the
// JSONL file.
func TestRunDetailKeepsRawTimestamp(t *testing.T) {
	jakarta := time.FixedZone("WIB", 7*60*60)
	run := JobExecutionResult{
		JobID: "11111111-2222-4333-8444-555555555555",
		ExecutionResult: ExecutionResult{
			Timestamp: time.Date(2026, 11, 5, 13, 18, 52, 513049000, jakarta),
			Status:    "success",
			Result:    "Job executed successfully",
		},
	}

	if got := RunDetail(run); !strings.Contains(got, "2026-11-05T13:18:52.513049+07:00") {
		t.Errorf("detail does not keep the stored timestamp:\n%s", got)
	}
}

// An entry written before the log kept a snapshot only has the id to go on, and
// the block says so instead of printing empty labels.
func TestRunDetailWithoutSnapshot(t *testing.T) {
	run := JobExecutionResult{
		JobID: "11111111-2222-4333-8444-555555555555",
		ExecutionResult: ExecutionResult{
			Timestamp: time.Date(2026, 11, 5, 13, 18, 52, 0, time.UTC),
			Status:    "success",
			Result:    "Job executed successfully",
		},
	}

	got := RunDetail(run)
	if !hasDetailLine(got, "Job ID", "11111111-2222-4333-8444-555555555555") {
		t.Errorf("detail does not carry the job id:\n%s", got)
	}
	if !strings.Contains(got, "recorded before") {
		t.Errorf("detail does not explain the missing snapshot:\n%s", got)
	}
	for _, unwanted := range []string{"Prompt", "Channel", "To", "Topic", "Session"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("detail printed an empty %s row:\n%s", unwanted, got)
		}
	}
}

// A name filled in by the caller (the per-job view knows the job) must not hide
// the fact that the entry itself kept no snapshot.
func TestRunDetailKeepsSnapshotNoteWithFallbackName(t *testing.T) {
	run := JobExecutionResult{
		JobID: "11111111-2222-4333-8444-555555555555",
		ExecutionResult: ExecutionResult{
			Timestamp: time.Date(2026, 11, 5, 13, 18, 52, 0, time.UTC),
			Status:    "success",
			Result:    "Job executed successfully",
			Name:      "Water reminder",
		},
	}

	got := RunDetail(run)
	if !hasDetailLine(got, "Job", "Water reminder") {
		t.Errorf("detail lost the job name:\n%s", got)
	}
	if !strings.Contains(got, "recorded before") {
		t.Errorf("detail hid the missing snapshot:\n%s", got)
	}
}

func TestMatchJobIDs(t *testing.T) {
	jobs := []Job{
		{ID: "11111111-2222-4333-8444-555555555555", Name: "Water reminder"},
		{ID: "11111112-2222-4333-8444-555555555555", Name: "Daily Report"},
	}

	if got := MatchJobIDs(jobs, jobs[0].ID); len(got) != 1 || got[0] != jobs[0].ID {
		t.Errorf("exact = %v", got)
	}
	if got := MatchJobIDs(jobs, "11111111"); len(got) != 1 || got[0] != jobs[0].ID {
		t.Errorf("prefix = %v", got)
	}
	if got := MatchJobIDs(jobs, "1111111"); len(got) != 2 {
		t.Errorf("ambiguous = %v", got)
	}
	if got := MatchJobIDs(jobs, "deadbeef"); len(got) != 0 {
		t.Errorf("missing = %v", got)
	}
	if got := MatchJobIDs(jobs, ""); len(got) != 0 {
		t.Errorf("empty = %v", got)
	}
}

// A job block is the only place a job's prompt and target are visible from
// `cron list`, so it carries every stored field with the raw timestamps.
func TestJobDetail(t *testing.T) {
	jakarta := time.FixedZone("WIB", 7*60*60)
	job := Job{
		ID:      "33333333-4444-4555-8666-777777777777",
		Name:    "Water reminder",
		AgentID: "default",
		Schedule: Schedule{
			Kind: "at",
			Expr: "2026-11-05T15:13:10+07:00",
		},
		Payload: Payload{Kind: "agentTurn", Content: "Time to drink water"},
		Delivery: Delivery{
			Mode:      "announce",
			Channel:   "telegram",
			To:        "-1001234567890",
			ThreadID:  "77",
			SessionID: "0f4e8b3a-1111-4222-8333-944455556666",
			ChatType:  "guild",
		},
		Enabled:   true,
		CreatedAt: time.Date(2026, 11, 5, 14, 13, 0, 0, jakarta),
		UpdatedAt: time.Date(2026, 11, 5, 14, 13, 0, 0, jakarta),
	}

	got := JobDetail(job)
	for _, want := range [][2]string{
		{"ID", "33333333-4444-4555-8666-777777777777"},
		{"Name", "Water reminder"},
		{"Agent", "default"},
		{"Schedule", "at · 2026-11-05T15:13:10+07:00"},
		{"Payload", "agentTurn"},
		{"Prompt", "Time to drink water"},
		{"Mode", "announce"},
		{"Channel", "telegram"},
		{"To", "-1001234567890"},
		{"Chat type", "guild"},
		{"Topic", "77"},
		{"Session", "0f4e8b3a-1111-4222-8333-944455556666"},
		{"Enabled", "yes"},
		{"Created", "2026-11-05T14:13:00+07:00"},
		{"Updated", "2026-11-05T14:13:00+07:00"},
	} {
		if !hasDetailLine(got, want[0], want[1]) {
			t.Errorf("job detail is missing %s %q:\n%s", want[0], want[1], got)
		}
	}
}

// hasDetailLine reports whether the block carries "label value" on one line.
func hasDetailLine(detail, label, value string) bool {
	for _, line := range strings.Split(detail, "\n") {
		if strings.HasPrefix(line, label) && strings.HasSuffix(line, value) {
			return true
		}
	}
	return false
}
