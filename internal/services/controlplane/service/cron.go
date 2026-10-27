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
			Mode:    "announce",
			Channel: strings.TrimSpace(in.Channel),
			To:      strings.TrimSpace(in.To),
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
		"prompt":  job.Payload.Content,
		"to":      job.Delivery.To,
		"channel": job.Delivery.Channel,
		"trigger": "cron",
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
