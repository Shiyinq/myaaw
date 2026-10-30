package approvals

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEntry is one approval decision record. Arguments are stored in full so
// an operator can reconstruct exactly what the model tried to run.
type AuditEntry struct {
	Time           time.Time `json:"time"`
	RequestID      string    `json:"request_id,omitempty"`
	RunID          string    `json:"run_id,omitempty"`
	Channel        string    `json:"channel,omitempty"`
	UserID         string    `json:"user_id,omitempty"`
	ConversationID string    `json:"conversation_id,omitempty"`
	SubAgent       string    `json:"subagent,omitempty"`
	Tool           string    `json:"tool"`
	Action         string    `json:"action,omitempty"`
	Key            string    `json:"key"`
	Args           any       `json:"args,omitempty"`
	Decision       string    `json:"decision,omitempty"`
	Source         string    `json:"source"`
	Allowed        bool      `json:"allowed"`
	Reason         string    `json:"reason,omitempty"`
	WaitedMS       int64     `json:"waited_ms,omitempty"`
}

// AuditLog appends one JSON object per line to a file. A nil logger is a no-op,
// and I/O failures are logged once per write without affecting the run.
type AuditLog struct {
	mu   sync.Mutex
	path string
}

// NewAuditLog returns a JSONL audit logger for path.
func NewAuditLog(path string) *AuditLog {
	return &AuditLog{path: path}
}

// Record appends entry. It never returns an error: auditing must not block or
// fail a tool call.
func (a *AuditLog) Record(entry AuditEntry) {
	if a == nil || a.path == "" {
		return
	}
	if entry.Time.IsZero() {
		entry.Time = time.Now()
	}
	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("approvals: audit marshal failed: %v", err)
		return
	}
	data = append(data, '\n')

	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		log.Printf("approvals: audit mkdir failed: %v", err)
		return
	}
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("approvals: audit open failed: %v", err)
		return
	}
	if _, err := f.Write(data); err != nil {
		log.Printf("approvals: audit write failed: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Printf("approvals: audit close failed: %v", err)
	}
}

// auditArgs returns the full arguments as a JSON value when they parse, or as a
// raw string otherwise, so no detail is lost in the audit trail.
func auditArgs(args json.RawMessage) any {
	if len(args) == 0 {
		return nil
	}
	if json.Valid(args) {
		return json.RawMessage(append([]byte(nil), args...))
	}
	return string(args)
}
