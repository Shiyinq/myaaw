package cron

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// ShortID is the 8-character id form used in listings and summaries.
func ShortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// JobDetail lays out every stored field of one job, one per line. Timestamps are
// printed exactly as they are stored (RFC3339 with their original offset),
// matching a run's detail block.
func JobDetail(job Job) string {
	var b strings.Builder

	writeDetailLine(&b, "ID", job.ID)
	writeDetailLine(&b, "Name", job.Name)
	writeDetailLine(&b, "Agent", job.AgentID)
	writeDetailLine(&b, "Schedule", scheduleLabel(job.Schedule))
	writeDetailLine(&b, "Payload", job.Payload.Kind)
	writeDetailLine(&b, "Prompt", job.Payload.Content)
	writeDetailLine(&b, "Mode", job.Delivery.Mode)
	writeDetailLine(&b, "Channel", job.Delivery.Channel)
	writeDetailLine(&b, "To", job.Delivery.To)
	writeDetailLine(&b, "Chat type", job.Delivery.ChatType)
	writeDetailLine(&b, "Topic", job.Delivery.ThreadID)
	writeDetailLine(&b, "Session", job.Delivery.SessionID)
	if job.Enabled {
		writeDetailLine(&b, "Enabled", "yes")
	} else {
		writeDetailLine(&b, "Enabled", "no")
	}
	writeDetailLine(&b, "Created", job.CreatedAt.Format(time.RFC3339Nano))
	writeDetailLine(&b, "Updated", job.UpdatedAt.Format(time.RFC3339Nano))
	return b.String()
}

// MatchJobIDs returns the ids of the stored jobs that equal, or start with, an id
// or prefix. The result tells an ambiguous prefix from a missing job.
func MatchJobIDs(jobs []Job, idOrPrefix string) []string {
	id := strings.TrimSpace(idOrPrefix)
	ids := make([]string, 0, 1)
	for _, job := range jobs {
		if job.ID == id {
			return []string{job.ID}
		}
	}
	for _, job := range jobs {
		if id != "" && strings.HasPrefix(job.ID, id) {
			ids = append(ids, job.ID)
		}
	}
	return ids
}

// RunDetail lays out every field a run stores, one per line. The timestamp is
// printed exactly as the log stores it (RFC3339 with its original offset) so it
// can be traced back to a line in cron/runs/<job id>.jsonl. Entries written
// before the log kept a job snapshot only have the time, status, and result, so
// the block shows the fields that exist instead of empty labels.
func RunDetail(run JobExecutionResult) string {
	var b strings.Builder

	writeDetailLine(&b, "Time", run.Timestamp.Format(time.RFC3339Nano))
	writeDetailLine(&b, "Job ID", run.JobID)
	writeDetailLine(&b, "Job", run.Name)
	writeDetailLine(&b, "Status", run.Status)
	writeDetailLine(&b, "Result", run.Result)
	writeDetailLine(&b, "Prompt", run.Message)
	writeDetailLine(&b, "Channel", run.Channel)
	writeDetailLine(&b, "To", run.To)
	writeDetailLine(&b, "Chat type", run.ChatType)
	writeDetailLine(&b, "Topic", run.ThreadID)
	writeDetailLine(&b, "Session", run.SessionID)
	if run.Message == "" && run.Channel == "" {
		fmt.Fprintln(&b, "(recorded before runs kept a job snapshot)")
	}

	return b.String()
}

// writeDetailLine appends "Label   value" and skips an empty value, so a block
// only carries the fields it has. The label column fits the longest name used
// here ("Chat type").
func writeDetailLine(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "%-10s %s\n", label, value)
}

// scheduleLabel reads as "kind · expression · timezone", dropping what the job
// does not set.
func scheduleLabel(schedule Schedule) string {
	parts := make([]string, 0, 3)
	if kind := strings.TrimSpace(schedule.Kind); kind != "" {
		parts = append(parts, kind)
	}
	if expr := strings.TrimSpace(schedule.Expr); expr != "" {
		parts = append(parts, expr)
	}
	if tz := strings.TrimSpace(schedule.Tz); tz != "" {
		parts = append(parts, tz)
	}
	return strings.Join(parts, " · ")
}

// AlignTable renders a left-aligned table. Cells are padded by their visible
// width, so a cell that carries ANSI color lines up with a plain one; padding by
// raw length is what skewed every column that had a style on it.
func AlignTable(header []string, rows [][]string) string {
	widths := make([]int, len(header))
	for i, cell := range header {
		widths[i] = visibleLen(cell)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && visibleLen(cell) > widths[i] {
				widths[i] = visibleLen(cell)
			}
		}
	}

	var b strings.Builder
	write := func(cells []string) {
		var line strings.Builder
		for i, cell := range cells {
			line.WriteString(cell)
			if i == len(widths)-1 {
				break
			}
			padding := widths[i] - visibleLen(cell) + 3
			if padding < 1 {
				padding = 1
			}
			line.WriteString(strings.Repeat(" ", padding))
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteString("\n")
	}

	write(header)
	for _, row := range rows {
		write(row)
	}
	return b.String()
}

// visibleLen counts the characters a terminal draws, skipping ANSI escapes.
func visibleLen(s string) int {
	return utf8.RuneCountInString(stripANSI(s))
}

func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		for i < len(s) && s[i] != 'm' {
			i++
		}
	}
	return b.String()
}
