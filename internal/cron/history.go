package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
