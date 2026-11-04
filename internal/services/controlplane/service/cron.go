package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/cron"
	"myaaw/internal/onboard"

	"github.com/google/uuid"
)

var ErrCronJobNotFound = errors.New("cron job not found")

type CronJobInput struct {
	Name         string `json:"name"`
	ScheduleKind string `json:"schedule_kind"` // cron, every, at
	Expr         string `json:"expr"`
	Tz           string `json:"tz,omitempty"`
	Message      string `json:"message"`
	Channel      string `json:"channel,omitempty"`
	To           string `json:"to,omitempty"`
	ThreadID     string `json:"thread_id,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	ChatType     string `json:"chat_type,omitempty"`
	Enabled      *bool  `json:"enabled,omitempty"`
}

type CronService struct {
	agents *AgentService
	client *http.Client
}

func NewCronService(agents *AgentService) *CronService {
	return &CronService{
		agents: agents,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (s *CronService) List(agentName string) ([]cron.Job, error) {
	store, _, err := cronStore(agentName)
	if err != nil {
		return nil, err
	}
	return store.Load()
}

func (s *CronService) Add(agentName string, in CronJobInput) (cron.Job, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return cron.Job{}, err
	}

	kind := strings.ToLower(strings.TrimSpace(in.ScheduleKind))
	switch kind {
	case "cron", "every", "at":
	default:
		return cron.Job{}, fmt.Errorf("%w: schedule_kind must be cron, every, or at", ErrInvalidConfig)
	}
	expr := strings.TrimSpace(in.Expr)
	if expr == "" {
		return cron.Job{}, fmt.Errorf("%w: expr is required", ErrInvalidConfig)
	}
	message := strings.TrimSpace(in.Message)
	if message == "" {
		return cron.Job{}, fmt.Errorf("%w: message is required", ErrInvalidConfig)
	}

	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	// A job must name a delivery target; the UI and CLI both require these, and a
	// job without them could only fail when it runs.
	channel := strings.ToLower(strings.TrimSpace(in.Channel))
	switch channel {
	case "telegram", "discord", "api", "cli":
	default:
		return cron.Job{}, fmt.Errorf("%w: channel must be telegram, discord, api, or cli", ErrInvalidConfig)
	}
	to := strings.TrimSpace(in.To)
	if to == "" {
		return cron.Job{}, fmt.Errorf("%w: to is required", ErrInvalidConfig)
	}
	sessionID := strings.TrimSpace(in.SessionID)
	if (channel == "api" || channel == "cli") && sessionID == "" {
		return cron.Job{}, fmt.Errorf("%w: session_id is required for api/cli targets", ErrInvalidConfig)
	}

	threadID := strings.TrimSpace(in.ThreadID)
	if err := onboard.ValidateThreadID(threadID); err != nil {
		return cron.Job{}, fmt.Errorf("%w: thread_id: %v", ErrInvalidConfig, err)
	}
	chatType := strings.ToLower(strings.TrimSpace(in.ChatType))
	if err := onboard.ValidateHeartbeatChatType(chatType); err != nil {
		return cron.Job{}, fmt.Errorf("%w: chat_type: %v", ErrInvalidConfig, err)
	}
	// Keep only the fields that apply to the chosen channel, matching the CLI.
	if channel != "telegram" {
		threadID = ""
	}
	if channel != "discord" {
		chatType = ""
	}

	now := time.Now().UTC()
	job := cron.Job{
		ID:      uuid.NewString(),
		Name:    strings.TrimSpace(in.Name),
		AgentID: agentName,
		Schedule: cron.Schedule{
			Kind: kind,
			Expr: expr,
			Tz:   strings.TrimSpace(in.Tz),
		},
		Payload: cron.Payload{
			Kind:    "agentTurn",
			Content: message,
		},
		Delivery: cron.Delivery{
			Mode:      "announce",
			Channel:   channel,
			To:        to,
			ThreadID:  threadID,
			SessionID: sessionID,
			ChatType:  chatType,
		},
		Enabled:   enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}

	store, _, err := cronStore(agentName)
	if err != nil {
		return cron.Job{}, err
	}
	if err := store.Add(job); err != nil {
		return cron.Job{}, err
	}
	return job, nil
}

func (s *CronService) Remove(agentName, id string) error {
	store, _, err := cronStore(agentName)
	if err != nil {
		return err
	}
	fullID, err := resolveJobID(store, id)
	if err != nil {
		return err
	}
	return store.Remove(fullID)
}

// Run triggers a job through the agent server's /heartbeat endpoint.
func (s *CronService) Run(agentName, id string) error {
	store, cronDir, err := cronStore(agentName)
	if err != nil {
		return err
	}
	fullID, err := resolveJobID(store, id)
	if err != nil {
		return err
	}
	job, err := store.Get(fullID)
	if err != nil {
		return ErrCronJobNotFound
	}

	baseURL, _, err := s.agents.BaseURL(agentName)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(map[string]string{
		"prompt":     job.Payload.Content,
		"to":         job.Delivery.To,
		"thread_id":  job.Delivery.ThreadID,
		"session_id": job.Delivery.SessionID,
		"chat_type":  job.Delivery.ChatType,
		"channel":    job.Delivery.Channel,
		"trigger":    "cron",
	})
	if err != nil {
		return err
	}

	request, err := http.NewRequest(http.MethodPost, baseURL+"/heartbeat", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAgentUnavailable, err)
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(response.Body)
	logger := cron.NewHistoryLogger(cronDir)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = logger.Log(job.ID, "failed", string(body))
		return fmt.Errorf("%w: %s", ErrAgentUnavailable, strings.TrimSpace(string(body)))
	}
	_ = logger.Log(job.ID, "success", string(body))
	return nil
}

func (s *CronService) History(agentName, id string, limit int) ([]cron.ExecutionResult, error) {
	store, cronDir, err := cronStore(agentName)
	if err != nil {
		return nil, err
	}
	fullID, err := resolveJobID(store, id)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	return cron.NewHistoryLogger(cronDir).GetHistory(fullID, limit)
}

func cronStore(agentName string) (*cron.Store, string, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return nil, "", err
	}
	cronDir, err := agentdir.CronDir(agentName)
	if err != nil {
		return nil, "", err
	}
	return cron.NewStore(filepath.Join(cronDir, "jobs.json")), cronDir, nil
}

func resolveJobID(store *cron.Store, id string) (string, error) {
	jobs, err := store.Load()
	if err != nil {
		return "", err
	}
	for _, job := range jobs {
		if job.ID == id {
			return job.ID, nil
		}
	}
	for _, job := range jobs {
		if strings.HasPrefix(job.ID, id) {
			return job.ID, nil
		}
	}
	return "", ErrCronJobNotFound
}
