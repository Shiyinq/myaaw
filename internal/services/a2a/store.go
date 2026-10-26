package a2a

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTaskNotFound       = errors.New("task not found")
	ErrPushConfigNotFound = errors.New("push notification config not found")
)

// TaskStore persists A2A tasks in the agent's SQLite database.
type TaskStore struct {
	db *sql.DB
}

func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db}
}

func (s *TaskStore) EnsureSchema() error {
	if s == nil || s.db == nil {
		return errors.New("a2a: database is not available")
	}
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS a2a_tasks (
		id TEXT PRIMARY KEY,
		context_id TEXT,
		status TEXT NOT NULL,
		status_message TEXT,
		messages TEXT NOT NULL,
		artifacts TEXT NOT NULL,
		attempts INTEGER NOT NULL DEFAULT 0,
		push_url TEXT,
		caller TEXT,
		created_at DATETIME,
		updated_at DATETIME
	);`)
	if err != nil {
		return err
	}
	// Columns added after the first release. Duplicate-column errors are
	// expected once the database has been migrated.
	for _, stmt := range []string{
		`ALTER TABLE a2a_tasks ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE a2a_tasks ADD COLUMN push_url TEXT`,
		`ALTER TABLE a2a_tasks ADD COLUMN caller TEXT`,
	} {
		_, _ = s.db.Exec(stmt)
	}
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS a2a_callers (
		name TEXT PRIMARY KEY,
		user_id INTEGER UNIQUE NOT NULL,
		created_at DATETIME
	);`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS a2a_push_configs (
		task_id TEXT NOT NULL,
		id TEXT NOT NULL,
		url TEXT NOT NULL,
		token TEXT,
		created_at DATETIME,
		PRIMARY KEY (task_id, id)
	);`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS a2a_message_ids (
		caller TEXT NOT NULL,
		message_id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (caller, message_id)
	);`)
	if err != nil {
		return err
	}
	// Idempotency records only need to survive retry windows; prune old rows
	// so the table does not grow with every message ever sent.
	_, _ = s.db.Exec(`DELETE FROM a2a_message_ids WHERE created_at < datetime('now', '-30 days')`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_a2a_tasks_updated ON a2a_tasks (updated_at DESC, id)`)
	return nil
}

// ReserveMessage associates a message ID with the task it produced so retries
// of the same message are idempotent. It returns the existing task ID when the
// message was already seen; an empty string means the caller now owns the
// reservation and should create the task.
func (s *TaskStore) ReserveMessage(ctx context.Context, caller, messageID, taskID string) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("a2a: database is not available")
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO a2a_message_ids (caller, message_id, task_id, created_at) VALUES (?, ?, ?, ?)`,
		caller, messageID, taskID, time.Now().UTC())
	if err != nil {
		return "", err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 1 {
		return "", nil
	}
	var existing string
	err = s.db.QueryRowContext(ctx,
		`SELECT task_id FROM a2a_message_ids WHERE caller = ? AND message_id = ?`,
		caller, messageID).Scan(&existing)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// The conflicting row vanished (pruned): let the retry proceed.
			return "", nil
		}
		return "", err
	}
	return existing, nil
}

// ReleaseMessage drops an idempotency record after task creation failed, so a
// retry is allowed to run instead of resolving to a task that does not exist.
func (s *TaskStore) ReleaseMessage(ctx context.Context, caller, messageID string) error {
	if s == nil || s.db == nil {
		return errors.New("a2a: database is not available")
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM a2a_message_ids WHERE caller = ? AND message_id = ?`, caller, messageID)
	return err
}

// EnsureCaller returns the synthetic local user ID assigned to a caller name,
// allocating a new one (starting at -1001) on first contact. Each calling
// agent therefore gets its own user row and conversation namespace.
func (s *TaskStore) EnsureCaller(ctx context.Context, name string) (int, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "anonymous"
	}

	var userID int
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM a2a_callers WHERE name = ?`, name).Scan(&userID)
	if err == nil {
		return userID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO a2a_callers (name, user_id, created_at)
		 VALUES (?, COALESCE((SELECT MIN(user_id) FROM a2a_callers), -1000) - 1, ?)`,
		name, time.Now().UTC()); err != nil {
		// Another request may have inserted the same caller concurrently.
		if readErr := s.db.QueryRowContext(ctx, `SELECT user_id FROM a2a_callers WHERE name = ?`, name).Scan(&userID); readErr == nil {
			return userID, nil
		}
		return 0, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT user_id FROM a2a_callers WHERE name = ?`, name).Scan(&userID); err != nil {
		return 0, err
	}
	return userID, nil
}

func (s *TaskStore) Create(ctx context.Context, task Task) error {
	if task.ID == "" {
		return errors.New("a2a: task id is required")
	}
	messages, err := json.Marshal(task.Messages)
	if err != nil {
		return err
	}
	artifacts, err := json.Marshal(task.Artifacts)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO a2a_tasks (id, context_id, status, status_message, messages, artifacts, attempts, push_url, caller, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.ContextID, task.Status.State, encodeMessage(task.Status.Message), string(messages), string(artifacts),
		task.Attempts, nullString(task.PushURL), nullString(task.Caller), now, now,
	)
	if err != nil {
		return fmt.Errorf("a2a: create task: %w", err)
	}
	return nil
}

func (s *TaskStore) Update(ctx context.Context, task Task) error {
	messages, err := json.Marshal(task.Messages)
	if err != nil {
		return err
	}
	artifacts, err := json.Marshal(task.Artifacts)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE a2a_tasks SET context_id = ?, status = ?, status_message = ?, messages = ?, artifacts = ?, attempts = ?, push_url = ?, caller = ?, updated_at = ? WHERE id = ?`,
		task.ContextID, task.Status.State, encodeMessage(task.Status.Message), string(messages), string(artifacts),
		task.Attempts, nullString(task.PushURL), nullString(task.Caller), time.Now().UTC(), task.ID,
	)
	if err != nil {
		return fmt.Errorf("a2a: update task: %w", err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// RecoverStale marks tasks that were still running when the process stopped
// as failed, so clients do not wait forever after a restart.
func (s *TaskStore) RecoverStale(ctx context.Context) (int64, error) {
	msg := Message{Role: "agent", Parts: []Part{{Text: "server restarted before the task completed"}}}
	res, err := s.db.ExecContext(ctx,
		`UPDATE a2a_tasks SET status = ?, status_message = ?, updated_at = ? WHERE status IN (?, ?)`,
		StateFailed, encodeMessage(&msg), time.Now().UTC(), StateSubmitted, StateWorking,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *TaskStore) Get(ctx context.Context, id string) (Task, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, context_id, status, status_message, messages, artifacts, attempts, push_url, caller, created_at, updated_at
		 FROM a2a_tasks WHERE id = ?`,
		id,
	)
	task, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrTaskNotFound
	}
	return task, err
}

// ListPending returns tasks that were accepted or running when the process
// stopped. They are candidates for resumption.
func (s *TaskStore) ListPending(ctx context.Context) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, context_id, status, status_message, messages, artifacts, attempts, push_url, caller, created_at, updated_at
		 FROM a2a_tasks WHERE status IN (?, ?) ORDER BY created_at ASC`,
		StateSubmitted, StateWorking,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner rowScanner) (Task, error) {
	var (
		task          Task
		statusMessage sql.NullString
		pushURL       sql.NullString
		caller        sql.NullString
		messages      string
		artifacts     string
		createdAt     time.Time
		updatedAt     time.Time
	)
	err := scanner.Scan(
		&task.ID, &task.ContextID, &task.Status.State, &statusMessage, &messages, &artifacts,
		&task.Attempts, &pushURL, &caller, &createdAt, &updatedAt,
	)
	if err != nil {
		return Task{}, err
	}
	if statusMessage.Valid && statusMessage.String != "" {
		var msg Message
		if json.Unmarshal([]byte(statusMessage.String), &msg) == nil {
			task.Status.Message = &msg
		}
	}
	if err := json.Unmarshal([]byte(messages), &task.Messages); err != nil {
		return Task{}, err
	}
	if err := json.Unmarshal([]byte(artifacts), &task.Artifacts); err != nil {
		return Task{}, err
	}
	if pushURL.Valid {
		task.PushURL = pushURL.String
	}
	if caller.Valid {
		task.Caller = caller.String
	}
	task.Status.Timestamp = updatedAt.UTC().Format(time.RFC3339Nano)
	return task, nil
}

func (s *TaskStore) SetPushConfig(ctx context.Context, taskID string, cfg PushNotificationConfig) (PushNotificationConfig, error) {
	if taskID == "" {
		return PushNotificationConfig{}, errors.New("task id is required")
	}
	if cfg.ID == "" {
		return PushNotificationConfig{}, errors.New("push notification config id is required")
	}
	if cfg.URL == "" {
		return PushNotificationConfig{}, errors.New("push notification url is required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO a2a_push_configs (task_id, id, url, token, created_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(task_id, id) DO UPDATE SET url = excluded.url, token = excluded.token`,
		taskID, cfg.ID, cfg.URL, nullString(cfg.Token), time.Now().UTC(),
	)
	if err != nil {
		return PushNotificationConfig{}, err
	}
	return cfg, nil
}

func (s *TaskStore) ListPushConfigs(ctx context.Context, taskID string) ([]PushNotificationConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, url, token FROM a2a_push_configs WHERE task_id = ? ORDER BY created_at ASC`,
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	configs := make([]PushNotificationConfig, 0)
	for rows.Next() {
		var (
			config PushNotificationConfig
			token  sql.NullString
		)
		if err := rows.Scan(&config.ID, &config.URL, &token); err != nil {
			return nil, err
		}
		if token.Valid {
			config.Token = token.String
		}
		configs = append(configs, config)
	}
	return configs, rows.Err()
}

func (s *TaskStore) GetPushConfig(ctx context.Context, taskID, id string) (PushNotificationConfig, error) {
	var (
		config PushNotificationConfig
		token  sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, url, token FROM a2a_push_configs WHERE task_id = ? AND id = ?`,
		taskID, id,
	).Scan(&config.ID, &config.URL, &token)
	if errors.Is(err, sql.ErrNoRows) {
		return PushNotificationConfig{}, ErrPushConfigNotFound
	}
	if err != nil {
		return PushNotificationConfig{}, err
	}
	if token.Valid {
		config.Token = token.String
	}
	return config, nil
}

func (s *TaskStore) DeletePushConfig(ctx context.Context, taskID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM a2a_push_configs WHERE task_id = ? AND id = ?`,
		taskID, id,
	)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrPushConfigNotFound
	}
	return nil
}

// ListTasks returns tasks visible to the caller with the given filters,
// ordered by status timestamp descending, using cursor pagination.
func (s *TaskStore) ListTasks(ctx context.Context, req ListTasksRequest, caller string) (ListTasksResponse, error) {
	pageSize := 50
	if req.PageSize != nil {
		if *req.PageSize < 1 || *req.PageSize > 100 {
			return ListTasksResponse{}, fmt.Errorf("pageSize must be between 1 and 100")
		}
		pageSize = *req.PageSize
	}

	where := "WHERE (caller = ? OR caller IS NULL OR caller = '')"
	args := []any{caller}
	if req.ContextID != "" {
		where += " AND context_id = ?"
		args = append(args, req.ContextID)
	}
	if req.Status != "" {
		internal := StateFromWire(req.Status)
		allowed := map[string]bool{
			StateSubmitted: true, StateWorking: true, StateInputRequired: true, StateAuthRequired: true,
			StateCompleted: true, StateFailed: true, StateCanceled: true, StateRejected: true,
		}
		if !allowed[internal] {
			return ListTasksResponse{}, fmt.Errorf("invalid status %q", req.Status)
		}
		where += " AND status = ?"
		args = append(args, internal)
	}
	if req.StatusTimestampAfter != "" {
		timestamp, err := parseTimestamp(req.StatusTimestampAfter)
		if err != nil {
			return ListTasksResponse{}, fmt.Errorf("statusTimestampAfter must be an ISO 8601 timestamp")
		}
		where += " AND updated_at >= ?"
		args = append(args, timestamp)
	}
	if req.PageToken != "" {
		updatedStr, id, err := decodePageToken(req.PageToken)
		if err != nil {
			return ListTasksResponse{}, fmt.Errorf("invalid pageToken")
		}
		updatedAt, err := parseTimestamp(updatedStr)
		if err != nil {
			return ListTasksResponse{}, fmt.Errorf("invalid pageToken")
		}
		where += " AND (updated_at < ? OR (updated_at = ? AND id < ?))"
		args = append(args, updatedAt, updatedAt, id)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM a2a_tasks "+where, args...).Scan(&total); err != nil {
		return ListTasksResponse{}, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, context_id, status, status_message, messages, artifacts, attempts, push_url, caller, created_at, updated_at
		 FROM a2a_tasks `+where+` ORDER BY updated_at DESC, id DESC LIMIT ?`,
		append(args, pageSize+1)...,
	)
	if err != nil {
		return ListTasksResponse{}, err
	}
	defer rows.Close()

	tasks := make([]Task, 0, pageSize)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return ListTasksResponse{}, err
		}
		if !req.IncludeArtifacts {
			task.Artifacts = nil
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return ListTasksResponse{}, err
	}

	response := ListTasksResponse{Tasks: tasks, PageSize: pageSize, TotalSize: total}
	if len(tasks) > pageSize {
		last := tasks[pageSize-1]
		response.Tasks = tasks[:pageSize]
		response.NextPageToken = encodePageToken(last)
	}
	return response, nil
}

func parseTimestamp(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, value)
}

func encodePageToken(task Task) string {
	raw := task.Status.Timestamp + "|" + task.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodePageToken(token string) (string, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("malformed page token")
	}
	return parts[0], parts[1], nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func encodeMessage(msg *Message) any {
	if msg == nil {
		return nil
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil
	}
	return string(data)
}
