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
}

type HistoryLogger struct {
	basePath string
}

func NewHistoryLogger(basePath string) *HistoryLogger {
	return &HistoryLogger{basePath: basePath}
}

func (h *HistoryLogger) Log(jobID string, status string, result string) error {
	dir := filepath.Join(h.basePath, "runs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	entry := ExecutionResult{
		Timestamp: time.Now(),
		Status:    status,
		Result:    result,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	filename := filepath.Join(dir, fmt.Sprintf("%s.jsonl", jobID))
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
	JobID     string    `json:"job_id"`
	Timestamp time.Time `json:"ts"`
	Status    string    `json:"status"`
	Result    string    `json:"result"`
}

func (h *HistoryLogger) GetGlobalHistory(limit int) ([]JobExecutionResult, error) {
	dir := filepath.Join(h.basePath, "runs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []JobExecutionResult{}, nil
		}
		return nil, err
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
					JobID:     jobID,
					Timestamp: res.Timestamp,
					Status:    res.Status,
					Result:    res.Result,
				})
			}
		}
	}

	// Sort by timestamp descending
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Timestamp.After(allResults[j].Timestamp)
	})

	if limit > 0 && len(allResults) > limit {
		return allResults[:limit], nil
	}

	return allResults, nil
}
