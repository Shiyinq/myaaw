package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type ExecutionResult struct {
	Timestamp time.Time `json:"ts"`
	Status    string    `json:"status"` // success, failed
	Result    string    `json:"result"`
	// Snapshot of the job at run time. One-time jobs are removed after they
	// run, so without this the history could not name the job or say what it
	// delivered. Older lines simply have these fields empty.
	Name      string `json:"name,omitempty"`
	Message   string `json:"message,omitempty"`
	Channel   string `json:"channel,omitempty"`
	To        string `json:"to,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	ChatType  string `json:"chat_type,omitempty"`
}

type HistoryLogger struct {
	basePath string
}

func NewHistoryLogger(basePath string) *HistoryLogger {
	return &HistoryLogger{basePath: basePath}
}

func (h *HistoryLogger) Log(job Job, status string, result string) error {
	dir := filepath.Join(h.basePath, "runs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	entry := ExecutionResult{
		Timestamp: time.Now(),
		Status:    status,
		Result:    result,
		Name:      job.Name,
		Message:   job.Payload.Content,
		Channel:   job.Delivery.Channel,
		To:        job.Delivery.To,
		ThreadID:  job.Delivery.ThreadID,
		SessionID: job.Delivery.SessionID,
		ChatType:  job.Delivery.ChatType,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	filename := filepath.Join(dir, fmt.Sprintf("%s.jsonl", job.ID))
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}
	if _, err := f.Write([]byte("\n")); err != nil {
		return err
	}

	return nil
}

func (h *HistoryLogger) GetHistory(jobID string, limit int) ([]ExecutionResult, error) {
	filename := filepath.Join(h.basePath, "runs", fmt.Sprintf("%s.jsonl", jobID))
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return []ExecutionResult{}, nil
		}
		return nil, err
	}

	var results []ExecutionResult
	lines := splitLines(data)

	// Read from end for latest
	count := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] == "" {
			continue
		}
		var res ExecutionResult
		if err := json.Unmarshal([]byte(lines[i]), &res); err == nil {
			results = append(results, res)
			count++
			if limit > 0 && count >= limit {
				break
			}
		}
	}

	return results, nil
}

func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

type JobExecutionResult struct {
	JobID string `json:"job_id"`
	ExecutionResult
}

// GetGlobalHistory returns the newest runs across every job plus the total
// number of entries, so a client can page through them.
func (h *HistoryLogger) GetGlobalHistory(limit, offset int) ([]JobExecutionResult, int, error) {
	dir := filepath.Join(h.basePath, "runs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []JobExecutionResult{}, 0, nil
		}
		return nil, 0, err
	}

	var allResults []JobExecutionResult

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}

		jobID := entry.Name()[:len(entry.Name())-len(".jsonl")]

		// To be efficient, we might want to read only the last few lines.
		// For simplicity, treating files as small enough for now.
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		lines := splitLines(data)
		for _, line := range lines {
			if line == "" {
				continue
			}
			var res ExecutionResult
			if err := json.Unmarshal([]byte(line), &res); err == nil {
				allResults = append(allResults, JobExecutionResult{
					JobID:           jobID,
					ExecutionResult: res,
				})
			}
		}
	}

	// Sort by timestamp descending
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Timestamp.After(allResults[j].Timestamp)
	})

	total := len(allResults)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []JobExecutionResult{}, total, nil
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return allResults[offset:end], total, nil
}
