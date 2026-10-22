package a2a

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agent/skills"
	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	"myaaw/internal/config"

	"github.com/google/uuid"
)

const (
	maxFilesPerMessage = 10
	maxFileBytes       = 5 << 20
	// Files at or below inlineFileBytes are returned as base64 `raw` parts;
	// larger files are returned as `url` parts and streamed on demand.
	inlineFileBytes = 1 << 20
	// maxArtifactBytes bounds the size of an outgoing file artifact.
	maxArtifactBytes  = 1 << 30
	defaultRunTimeout = 15 * time.Minute
	maxTaskAttempts   = 3
	maxStreamEvents   = 2000
	streamRetention   = 10 * time.Minute

	notifyRetryAttempts  = 3
	notifyRetryBaseDelay = 500 * time.Millisecond

	envTaskDir        = "MYAAW_A2A_TASK_DIR"
	inputRequiredFile = "input-required.json"
	authRequiredFile  = "auth-required.json"
)

var taskIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

var (
	ErrTaskNotResumable          = errors.New("task is not in a resumable state")
	ErrTaskNotCancelable         = errors.New("task is not cancelable")
	ErrContextMismatch           = errors.New("contextId does not match the task")
	ErrExtendedCardNotConfigured = errors.New("extended agent card is not configured")
)

// Runner is the subset of BotService needed to execute an A2A task.
type Runner interface {
	BotContext(ctx context.Context, msg *channel.IncomingMessage) (*channel.OutgoingMessage, error)
	BotStreamContext(ctx context.Context, msg *channel.IncomingMessage, onChunk func(channel.StreamChunk)) (*channel.OutgoingMessage, error)
}

// Service executes A2A tasks with the local agent loop.
type Service struct {
	store  *TaskStore
	runner Runner
	runMu  sync.Mutex

	callers      []callerCredential
	signingKey   ed25519.PrivateKey
	trustedKeys  map[string]ed25519.PublicKey
	trustedNames map[string]string

	runsMu sync.Mutex
	runs   map[string]context.CancelFunc

	streamsMu sync.Mutex
	streams   map[string]*taskStream

	rateLimit float64
	rateBurst float64
	rateMu    sync.Mutex
	buckets   map[string]*tokenBucket
}

// tokenBucket is a per-caller token bucket used for rate limiting.
type tokenBucket struct {
	tokens float64
	last   time.Time
}

func NewService(store *TaskStore, runner Runner) *Service {
	rateLimit := config.A2ARateLimit
	if rateLimit < 0 {
		rateLimit = 0
	}
	rateBurst := float64(config.A2ARateBurst)
	if rateBurst <= 0 && rateLimit > 0 {
		rateBurst = math.Max(1, math.Ceil(rateLimit))
	}
	service := &Service{
		store:     store,
		runner:    runner,
		runs:      make(map[string]context.CancelFunc),
		streams:   make(map[string]*taskStream),
		rateLimit: rateLimit,
		rateBurst: rateBurst,
		buckets:   make(map[string]*tokenBucket),
	}
	service.loadCallers()
	service.loadSigning()
	return service
}

// AllowCaller consumes one token from the caller's bucket. Rate limiting is
// disabled when A2A_RATE_LIMIT is zero or negative.
func (s *Service) AllowCaller(name string) bool {
	if s.rateLimit <= 0 || s.rateBurst <= 0 {
		return true
	}
	now := time.Now()
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	bucket := s.buckets[name]
	if bucket == nil {
		bucket = &tokenBucket{tokens: s.rateBurst, last: now}
		s.buckets[name] = bucket
	}
	elapsed := now.Sub(bucket.last).Seconds()
	bucket.tokens = math.Min(s.rateBurst, bucket.tokens+elapsed*s.rateLimit)
	bucket.last = now
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func (s *Service) EnsureSchema() error {
	return s.store.EnsureSchema()
}

// Card returns the public discovery document for the active agent.
func (s *Service) Card() AgentCard {
	name := agentdir.Active()
	var skillList []skills.Skill
	if dir, err := agentdir.ActiveSkillsDir(); err == nil {
		skillList, _ = skills.ListSkills(dir)
	}
	card := BuildCard(name, baseURL(), skillList, len(s.callers) > 0)
	if ExtendedCardSupported() {
		card.Capabilities.ExtendedAgentCard = true
	}
	if len(s.signingKey) > 0 {
		card = s.signCard(card)
	}
	return card
}

// ExtendedCardSupported reports whether an extended card document is
// configured through A2A_EXTENDED_CARD.
func ExtendedCardSupported() bool {
	return strings.TrimSpace(config.A2AExtendedCardPath) != ""
}

// ExtendedCard returns the public Agent Card with the A2A_EXTENDED_CARD JSON
// document merged over it. The merge is recursive for objects and replaces
// arrays and scalars, so an operator can add skills or override capabilities.
func (s *Service) ExtendedCard() ([]byte, error) {
	path := strings.TrimSpace(config.A2AExtendedCardPath)
	if path == "" {
		return nil, ErrExtendedCardNotConfigured
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrExtendedCardNotConfigured
		}
		return nil, fmt.Errorf("cannot read extended agent card: %w", err)
	}
	var override map[string]any
	if err := json.Unmarshal(data, &override); err != nil {
		return nil, fmt.Errorf("invalid extended agent card: %w", err)
	}
	public, err := json.Marshal(s.Card())
	if err != nil {
		return nil, err
	}
	var merged map[string]any
	if err := json.Unmarshal(public, &merged); err != nil {
		return nil, err
	}
	mergeJSONObjects(merged, override)
	return json.MarshalIndent(merged, "", "  ")
}

func mergeJSONObjects(base, override map[string]any) {
	for key, value := range override {
		if key == "skills" {
			base[key] = mergeSkills(base[key], value)
			continue
		}
		if overrideObject, ok := value.(map[string]any); ok {
			if baseObject, ok := base[key].(map[string]any); ok {
				mergeJSONObjects(baseObject, overrideObject)
				continue
			}
		}
		base[key] = value
	}
}

// mergeSkills appends the extended skills to the public ones, deduplicating by
// id so an operator only has to list the additional skills.
func mergeSkills(base, extra any) any {
	merged := make([]any, 0)
	ids := make(map[string]bool)
	for _, list := range []any{base, extra} {
		items, _ := list.([]any)
		for _, item := range items {
			if object, ok := item.(map[string]any); ok {
				if id, _ := object["id"].(string); id != "" {
					if ids[id] {
						continue
					}
					ids[id] = true
				}
			}
			merged = append(merged, item)
		}
	}
	return merged
}

// BuildCard builds an Agent Card from explicit inputs.
func BuildCard(name, baseURL string, skillList []skills.Skill, requireAuth bool) AgentCard {
	interfaces := []AgentInterface{
		{
			URL:             strings.TrimRight(baseURL, "/") + "/a2a",
			ProtocolBinding: "JSONRPC",
			ProtocolVersion: SupportedProtocolVersion,
		},
		{
			URL:             strings.TrimRight(baseURL, "/"),
			ProtocolBinding: "HTTP+JSON",
			ProtocolVersion: SupportedProtocolVersion,
		},
	}
	if grpcURL := grpcInterfaceURL(); grpcURL != "" {
		interfaces = append(interfaces, AgentInterface{
			URL:             grpcURL,
			ProtocolBinding: "GRPC",
			ProtocolVersion: SupportedProtocolVersion,
		})
	}

	card := AgentCard{
		Name:                name,
		Description:         "Myaaw agent " + name,
		SupportedInterfaces: interfaces,
		Version:             SupportedProtocolVersion,
		Capabilities:        Capabilities{Streaming: true, PushNotifications: true},
		DefaultInputModes:   []string{"text/plain", "application/json"},
		DefaultOutputModes:  []string{"text/plain"},
		Skills:              []AgentSkill{},
	}
	if url := strings.TrimSpace(config.A2AProviderURL); url != "" || strings.TrimSpace(config.A2AProviderOrganization) != "" {
		card.Provider = &AgentProvider{
			URL:          url,
			Organization: strings.TrimSpace(config.A2AProviderOrganization),
		}
	}
	if url := strings.TrimSpace(config.A2ADocumentationURL); url != "" {
		card.DocumentationURL = url
	}
	if url := strings.TrimSpace(config.A2AIconURL); url != "" {
		card.IconURL = url
	}
	for _, skill := range skillList {
		if !skill.Enabled {
			continue
		}
		card.Skills = append(card.Skills, AgentSkill{
			ID:          skill.Name,
			Name:        displayName(skill),
			Description: skill.Description,
			Tags:        []string{skill.Name},
		})
	}
	if requireAuth {
		card.SecuritySchemes = map[string]SecurityScheme{"bearer": BearerSecurityScheme()}
		card.SecurityRequirements = []SecurityRequirement{BearerSecurityRequirement()}
	}
	return card
}

func displayName(skill skills.Skill) string {
	if name := strings.TrimSpace(skill.DisplayName); name != "" {
		return name
	}
	return skill.Name
}

func baseURL() string {
	base := strings.TrimRight(strings.TrimSpace(config.MYAAWBaseURL), "/")
	if base != "" {
		return base
	}
	port := strings.TrimPrefix(config.PORT, ":")
	if port == "" {
		port = "8080"
	}
	return "http://localhost:" + port
}

// grpcInterfaceURL returns the hostname:port address advertised in the Agent
// Card for the gRPC binding, or an empty string when gRPC is disabled.
func grpcInterfaceURL() string {
	port := strings.TrimSpace(config.A2AGRPCAddr)
	if port == "" {
		port = strings.TrimSpace(config.A2AGRPCPort)
	}
	port = strings.TrimLeft(strings.TrimSpace(port), ":")
	if port == "" {
		return ""
	}
	host := "localhost"
	if base := strings.TrimSpace(config.MYAAWBaseURL); base != "" {
		if parsed, err := url.Parse(base); err == nil && parsed.Hostname() != "" {
			host = parsed.Hostname()
		}
	}
	return host + ":" + port
}

// SendMessage creates a task for the request and either waits for it to reach
// a terminal or interrupted state, or returns immediately when the caller
// requested returnImmediately.
func (s *Service) SendMessage(ctx context.Context, req SendMessageRequest) (Task, error) {
	if req.Configuration != nil && req.Configuration.ReturnImmediately {
		return s.SendMessageAsync(ctx, req)
	}

	task, msg, err := s.prepareTask(ctx, req, StateWorking)
	if err != nil {
		return Task{}, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.registerRun(task.ID, cancel)
	defer s.unregisterRun(task.ID)

	s.runMu.Lock()
	out, question, authRequest, err := s.runBot(task.ID, func() (*channel.OutgoingMessage, error) {
		return s.runner.BotContext(runCtx, msg)
	})
	s.runMu.Unlock()
	if err != nil {
		return s.failTask(ctx, task, err), nil
	}
	if question != "" {
		return s.inputRequired(ctx, task, question), nil
	}
	if authRequest != "" {
		return s.authRequired(ctx, task, authRequest), nil
	}
	return s.completeTask(ctx, task, out)
}

// SendMessageAsync creates the task and schedules it in the background.
func (s *Service) SendMessageAsync(ctx context.Context, req SendMessageRequest) (Task, error) {
	task, msg, err := s.prepareTask(ctx, req, StateSubmitted)
	if err != nil {
		return Task{}, err
	}
	go s.runTask(task, msg)
	return task, nil
}

// SendMessageStream runs a task and emits StreamResponse objects as text
// chunks arrive. The first response is the task itself.
func (s *Service) SendMessageStream(ctx context.Context, req SendMessageRequest, send func(StreamResponse)) (Task, error) {
	task, msg, err := s.prepareTask(ctx, req, StateWorking)
	if err != nil {
		return Task{}, err
	}
	s.startStream(task.ID)
	defer s.finishStream(task.ID)

	emit := func(response StreamResponse) {
		s.publishResponse(task.ID, response)
		if send != nil {
			send(response)
		}
	}
	emit(StreamResponse{Task: taskPtr(task)})

	status := func(final bool) {
		emit(StreamResponse{StatusUpdate: &TaskStatusUpdateEvent{
			TaskID:    task.ID,
			ContextID: task.ContextID,
			Status:    task.Status,
			Metadata:  map[string]bool{"final": final},
		}})
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.registerRun(task.ID, cancel)
	defer s.unregisterRun(task.ID)

	out, question, authRequest, err := func() (*channel.OutgoingMessage, string, string, error) {
		s.runMu.Lock()
		defer s.runMu.Unlock()
		return s.runBot(task.ID, func() (*channel.OutgoingMessage, error) {
			return s.runner.BotStreamContext(runCtx, msg, func(chunk channel.StreamChunk) {
				if chunk.Kind != "text" || chunk.Text == "" {
					return
				}
				emit(StreamResponse{ArtifactUpdate: &TaskArtifactUpdateEvent{
					TaskID:    task.ID,
					ContextID: task.ContextID,
					Artifact: Artifact{
						ArtifactID: uuid.NewString(),
						Name:       "response",
						Parts:      []Part{{Text: chunk.Text}},
					},
					Append: true,
				}})
			})
		})
	}()
	if err != nil {
		task = s.failTask(ctx, task, err)
		status(true)
		return task, nil
	}
	if question != "" {
		task = s.inputRequired(ctx, task, question)
		status(true)
		return task, nil
	}
	if authRequest != "" {
		task = s.authRequired(ctx, task, authRequest)
		status(true)
		return task, nil
	}

	task, err = s.completeTask(ctx, task, out)
	if err != nil {
		return Task{}, err
	}
	for i := range task.Artifacts {
		artifact := task.Artifacts[i]
		emit(StreamResponse{ArtifactUpdate: &TaskArtifactUpdateEvent{
			TaskID:    task.ID,
			ContextID: task.ContextID,
			Artifact:  artifact,
			LastChunk: true,
		}})
	}
	status(true)
	return task, nil
}

func taskPtr(task Task) *Task {
	return &task
}

func (s *Service) GetTask(ctx context.Context, id string) (Task, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	return s.store.Get(ctx, id)
}

// ListTasks lists tasks visible to the caller with the given filters.
func (s *Service) ListTasks(ctx context.Context, req ListTasksRequest, caller string) (ListTasksResponse, error) {
	return s.store.ListTasks(ctx, req, caller)
}

// CreatePushConfig validates and stores a push notification config for a task.
func (s *Service) CreatePushConfig(ctx context.Context, taskID string, cfg PushNotificationConfig) (PushNotificationConfig, error) {
	pushURL, err := validateWebhookURL(cfg.URL)
	if err != nil {
		return PushNotificationConfig{}, err
	}
	if strings.TrimSpace(pushURL) == "" {
		return PushNotificationConfig{}, errors.New("url is required")
	}
	cfg.URL = pushURL
	cfg.TaskID = taskID
	if cfg.ID == "" {
		cfg.ID = uuid.NewString()
	}
	return s.store.SetPushConfig(ctx, taskID, cfg)
}

// GetPushConfig returns one push notification config.
func (s *Service) GetPushConfig(ctx context.Context, taskID, id string) (PushNotificationConfig, error) {
	return s.store.GetPushConfig(ctx, taskID, id)
}

// ListPushConfigs returns the push notification configs for a task.
func (s *Service) ListPushConfigs(ctx context.Context, taskID string) ([]PushNotificationConfig, error) {
	return s.store.ListPushConfigs(ctx, taskID)
}

// DeletePushConfig removes a push notification config.
func (s *Service) DeletePushConfig(ctx context.Context, taskID, id string) error {
	return s.store.DeletePushConfig(ctx, taskID, id)
}

// SubscribeStream exposes the in-memory stream buffer so bindings can replay
// and forward events. active is false when the stream is finished or unknown.
func (s *Service) SubscribeStream(id string) (snapshot []StreamResponse, events chan StreamResponse, active bool, exists bool) {
	return s.subscribeStream(id)
}

// UnsubscribeStream detaches a subscriber created by SubscribeStream.
func (s *Service) UnsubscribeStream(id string, events chan StreamResponse) {
	s.unsubscribeStream(id, events)
}

// ResumePending re-runs tasks that were accepted or running when the process
// stopped. Tasks that already exhausted their attempts are failed instead.
func (s *Service) ResumePending(ctx context.Context) (int, error) {
	tasks, err := s.store.ListPending(ctx)
	if err != nil {
		return 0, err
	}
	resumed := 0
	for _, task := range tasks {
		if task.Attempts >= maxTaskAttempts {
			s.failTask(ctx, task, fmt.Errorf("task did not finish after %d attempts", maxTaskAttempts))
			continue
		}
		message, ok := lastUserMessage(task)
		if !ok {
			s.failTask(ctx, task, errors.New("task has no user message to resume"))
			continue
		}
		callerName := strings.TrimSpace(task.Caller)
		userID, err := s.store.EnsureCaller(ctx, callerName)
		if err != nil {
			log.Printf("a2a: resume caller for task %s: %v", task.ID, err)
			s.failTask(ctx, task, err)
			continue
		}
		incoming, err := buildIncomingMessage(task.ID, task.ContextID, userID, callerName, message)
		if err != nil {
			s.failTask(ctx, task, err)
			continue
		}
		task.Attempts++
		task.Status = TaskStatus{State: StateSubmitted, Timestamp: nowTimestamp()}
		if err := s.store.Update(ctx, task); err != nil {
			log.Printf("a2a: resume task %s: %v", task.ID, err)
			continue
		}
		go s.runTask(task, incoming)
		resumed++
	}
	return resumed, nil
}

func lastUserMessage(task Task) (Message, bool) {
	for i := len(task.Messages) - 1; i >= 0; i-- {
		if task.Messages[i].Role == "user" {
			return task.Messages[i], true
		}
	}
	return Message{}, false
}

// OwnsTask reports whether the caller may read or cancel a task. Legacy tasks
// without a recorded caller remain accessible.
func (s *Service) OwnsTask(task Task, callerName string) bool {
	if task.Caller == "" || strings.TrimSpace(callerName) == "" {
		return true
	}
	return strings.EqualFold(task.Caller, callerName)
}

// CancelTask cancels a running or interrupted task. Terminal tasks return
// ErrTaskNotCancelable.
func (s *Service) CancelTask(ctx context.Context, id string) (Task, error) {
	task, err := s.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if TerminalStates[task.Status.State] {
		return Task{}, ErrTaskNotCancelable
	}
	task.Status = TaskStatus{State: StateCanceled, Timestamp: nowTimestamp()}
	if err := s.store.Update(ctx, task); err != nil {
		return Task{}, err
	}
	s.cancelRun(id)
	s.notifyTaskAsync(task.ID)
	return task, nil
}

// taskStream buffers StreamResponse objects for SendMessageStream so a
// reconnecting caller can replay them with SubscribeToTask.
type taskStream struct {
	events []StreamResponse
	subs   map[chan StreamResponse]struct{}
	done   bool
}

func (s *Service) startStream(id string) {
	s.streamsMu.Lock()
	s.streams[id] = &taskStream{subs: make(map[chan StreamResponse]struct{})}
	s.streamsMu.Unlock()
}

func (s *Service) publishResponse(id string, response StreamResponse) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	stream := s.streams[id]
	if stream == nil {
		return
	}
	if len(stream.events) < maxStreamEvents {
		stream.events = append(stream.events, response)
	} else {
		copy(stream.events, stream.events[1:])
		stream.events[len(stream.events)-1] = response
	}
	for ch := range stream.subs {
		select {
		case ch <- response:
		default:
		}
	}
}

func (s *Service) subscribeStream(id string) ([]StreamResponse, chan StreamResponse, bool, bool) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	stream := s.streams[id]
	if stream == nil {
		return nil, nil, false, false
	}
	snapshot := append([]StreamResponse(nil), stream.events...)
	if stream.done {
		return snapshot, nil, false, true
	}
	events := make(chan StreamResponse, 256)
	stream.subs[events] = struct{}{}
	return snapshot, events, true, true
}

func (s *Service) unsubscribeStream(id string, events chan StreamResponse) {
	if events == nil {
		return
	}
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	stream := s.streams[id]
	if stream == nil {
		return
	}
	if _, ok := stream.subs[events]; !ok {
		return
	}
	delete(stream.subs, events)
	close(events)
}

func (s *Service) finishStream(id string) {
	s.streamsMu.Lock()
	stream := s.streams[id]
	if stream == nil {
		s.streamsMu.Unlock()
		return
	}
	stream.done = true
	for events := range stream.subs {
		delete(stream.subs, events)
		close(events)
	}
	s.streamsMu.Unlock()

	time.AfterFunc(streamRetention, func() {
		s.streamsMu.Lock()
		if s.streams[id] == stream {
			delete(s.streams, id)
		}
		s.streamsMu.Unlock()
	})
}

func (s *Service) registerRun(id string, cancel context.CancelFunc) {
	s.runsMu.Lock()
	s.runs[id] = cancel
	s.runsMu.Unlock()
}

func (s *Service) unregisterRun(id string) {
	s.runsMu.Lock()
	delete(s.runs, id)
	s.runsMu.Unlock()
}

func (s *Service) cancelRun(id string) bool {
	s.runsMu.Lock()
	cancel, ok := s.runs[id]
	s.runsMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// canceledTask reports whether the caller canceled the task while it was
// running. A canceled task is never overwritten by a late run result.
func (s *Service) canceledTask(ctx context.Context, task Task) (Task, bool) {
	current, err := s.store.Get(ctx, task.ID)
	if err != nil {
		return Task{}, false
	}
	if current.Status.State == StateCanceled {
		return current, true
	}
	return Task{}, false
}

func (s *Service) prepareTask(ctx context.Context, req SendMessageRequest, state string) (Task, *channel.IncomingMessage, error) {
	msg := req.Message
	if !hasContent(msg) {
		return Task{}, nil, fmt.Errorf("message must contain text, raw, url, or data parts")
	}
	if err := validateParts(msg.Parts); err != nil {
		return Task{}, nil, err
	}

	callerName := strings.TrimSpace(req.Caller)
	if callerName == "" {
		callerName = "anonymous"
	}

	id := strings.TrimSpace(msg.TaskID)
	var existing *Task
	if id != "" {
		if !taskIDPattern.MatchString(id) {
			return Task{}, nil, fmt.Errorf("task id may only contain letters, numbers, '_' and '-'")
		}
		got, err := s.store.Get(ctx, id)
		if err != nil {
			if errors.Is(err, ErrTaskNotFound) {
				return Task{}, nil, ErrTaskNotFound
			}
			return Task{}, nil, err
		}
		if TerminalStates[got.Status.State] {
			return Task{}, nil, ErrTaskNotResumable
		}
		if !InterruptedStates[got.Status.State] {
			return Task{}, nil, ErrTaskNotResumable
		}
		if got.Caller != "" && !strings.EqualFold(got.Caller, callerName) {
			return Task{}, nil, ErrTaskNotFound
		}
		existing = &got
	} else {
		id = uuid.NewString()
	}

	contextID := strings.TrimSpace(msg.ContextID)
	if existing != nil {
		if contextID != "" && existing.ContextID != "" && contextID != existing.ContextID {
			return Task{}, nil, ErrContextMismatch
		}
		if existing.ContextID != "" {
			contextID = existing.ContextID
		}
	}
	if contextID == "" {
		contextID = uuid.NewString()
	}

	userID, err := s.store.EnsureCaller(ctx, callerName)
	if err != nil {
		return Task{}, nil, err
	}

	userMessage := msg
	userMessage.Role = "user"
	if userMessage.MessageID == "" {
		userMessage.MessageID = uuid.NewString()
	}
	userMessage.ContextID = contextID
	userMessage.TaskID = id

	var task Task
	if existing != nil {
		task = *existing
		task.ContextID = contextID
		task.Caller = callerName
		task.Messages = append(task.Messages, userMessage)
		task.Status = TaskStatus{State: state, Timestamp: nowTimestamp()}
		if err := s.store.Update(ctx, task); err != nil {
			return Task{}, nil, err
		}
	} else {
		task = Task{
			ID:        id,
			ContextID: contextID,
			Status:    TaskStatus{State: state, Timestamp: nowTimestamp()},
			Messages:  []Message{userMessage},
			Attempts:  1,
			Caller:    callerName,
			Metadata:  req.Metadata,
		}
		if config := req.Configuration; config != nil && config.TaskPushNotificationConfig != nil {
			pushURL := strings.TrimSpace(config.TaskPushNotificationConfig.URL)
			if pushURL == "" {
				return Task{}, nil, fmt.Errorf("taskPushNotificationConfig.url is required")
			}
			if _, err := validateWebhookURL(pushURL); err != nil {
				return Task{}, nil, err
			}
			task.PushURL = pushURL
		}
		if err := s.store.Create(ctx, task); err != nil {
			return Task{}, nil, err
		}
		if config := req.Configuration; config != nil && config.TaskPushNotificationConfig != nil {
			pushConfig := *config.TaskPushNotificationConfig
			pushConfig.TaskID = task.ID
			if pushConfig.ID == "" {
				pushConfig.ID = uuid.NewString()
			}
			if _, err := s.store.SetPushConfig(ctx, task.ID, pushConfig); err != nil {
				return Task{}, nil, err
			}
		}
	}

	incoming, err := buildIncomingMessage(id, contextID, userID, callerName, userMessage)
	if err != nil {
		_ = s.failTask(ctx, task, err)
		return Task{}, nil, err
	}
	return task, incoming, nil
}

func (s *Service) completeTask(ctx context.Context, task Task, out *channel.OutgoingMessage) (Task, error) {
	if current, ok := s.canceledTask(ctx, task); ok {
		return current, nil
	}
	if out != nil {
		if out.ConversationID != "" {
			task.ContextID = out.ConversationID
		}
		reply := Message{MessageID: uuid.NewString(), Role: "agent", Parts: []Part{{Text: out.Text}}}
		task.Messages = append(task.Messages, reply)
		task.Artifacts = append(task.Artifacts, Artifact{
			ArtifactID: uuid.NewString(),
			Name:       "response",
			Parts:      []Part{{Text: out.Text}},
		})
	}
	task.Artifacts = append(task.Artifacts, collectFileArtifacts(task.ID)...)
	task.Status = TaskStatus{State: StateCompleted, Timestamp: nowTimestamp()}
	if err := s.store.Update(ctx, task); err != nil {
		return Task{}, err
	}
	s.notifyTaskAsync(task.ID)
	return task, nil
}

func (s *Service) inputRequired(ctx context.Context, task Task, question string) Task {
	if current, ok := s.canceledTask(ctx, task); ok {
		return current
	}
	msg := Message{MessageID: uuid.NewString(), Role: "agent", Parts: []Part{{Text: question}}}
	task.Status = TaskStatus{State: StateInputRequired, Timestamp: nowTimestamp(), Message: &msg}
	task.Messages = append(task.Messages, msg)
	_ = s.store.Update(ctx, task)
	s.notifyTaskAsync(task.ID)
	return task
}

func (s *Service) authRequired(ctx context.Context, task Task, request string) Task {
	if current, ok := s.canceledTask(ctx, task); ok {
		return current
	}
	msg := Message{MessageID: uuid.NewString(), Role: "agent", Parts: []Part{{Text: request}}}
	task.Status = TaskStatus{State: StateAuthRequired, Timestamp: nowTimestamp(), Message: &msg}
	task.Messages = append(task.Messages, msg)
	_ = s.store.Update(ctx, task)
	s.notifyTaskAsync(task.ID)
	return task
}

func (s *Service) failTask(ctx context.Context, task Task, runErr error) Task {
	if current, ok := s.canceledTask(ctx, task); ok {
		return current
	}
	failure := Message{MessageID: uuid.NewString(), Role: "agent", Parts: []Part{{Text: runErr.Error()}}}
	task.Status = TaskStatus{State: StateFailed, Timestamp: nowTimestamp(), Message: &failure}
	task.Messages = append(task.Messages, failure)
	_ = s.store.Update(ctx, task)
	s.notifyTaskAsync(task.ID)
	return task
}

func (s *Service) runTask(task Task, msg *channel.IncomingMessage) {
	timeout := time.Duration(config.AgentRunTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultRunTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	s.registerRun(task.ID, cancel)
	defer s.unregisterRun(task.ID)

	// The agent loop is not safe for concurrent runs in one process.
	s.runMu.Lock()
	defer s.runMu.Unlock()

	task.Status = TaskStatus{State: StateWorking, Timestamp: nowTimestamp()}
	if err := s.store.Update(ctx, task); err != nil {
		log.Printf("a2a: update task %s: %v", task.ID, err)
	}

	out, question, authRequest, err := s.runBot(task.ID, func() (*channel.OutgoingMessage, error) {
		return s.runner.BotContext(ctx, msg)
	})
	if err != nil {
		task = s.failTask(ctx, task, err)
	} else if question != "" {
		task = s.inputRequired(ctx, task, question)
	} else if authRequest != "" {
		task = s.authRequired(ctx, task, authRequest)
	} else {
		var completeErr error
		task, completeErr = s.completeTask(ctx, task, out)
		if completeErr != nil {
			log.Printf("a2a: complete task %s: %v", task.ID, completeErr)
		}
	}
}

// notifyTask sends StreamResponse events to every registered webhook: a
// status update for every state change plus, for terminal states, a final
// task snapshot. Deliveries are attempted with exponential backoff so short
// receiver outages do not lose the notification.
func (s *Service) notifyTask(taskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	task, err := s.store.Get(ctx, taskID)
	if err != nil {
		return
	}

	targets := make([]PushNotificationConfig, 0)
	if task.PushURL != "" {
		targets = append(targets, PushNotificationConfig{URL: task.PushURL})
	}
	if configs, err := s.store.ListPushConfigs(ctx, taskID); err == nil {
		targets = append(targets, configs...)
	}

	events := []StreamResponse{{
		StatusUpdate: &TaskStatusUpdateEvent{
			TaskID:    task.ID,
			ContextID: task.ContextID,
			Status:    task.Status,
		},
	}}
	if TerminalStates[task.Status.State] {
		events = append(events, StreamResponse{Task: &task})
	}

	seen := make(map[string]bool, len(targets))
	for _, target := range targets {
		if target.URL == "" || seen[target.URL] {
			continue
		}
		seen[target.URL] = true
		for _, event := range events {
			s.notify(target, event)
		}
	}
}

func (s *Service) notifyTaskAsync(taskID string) {
	go s.notifyTask(taskID)
}

// notify delivers one StreamResponse to a webhook. Transport errors and
// non-2xx responses are retried with exponential backoff (at-least-once).
func (s *Service) notify(target PushNotificationConfig, response StreamResponse) {
	if target.URL == "" {
		return
	}
	data, err := json.Marshal(response)
	if err != nil {
		return
	}
	for attempt, delay := 0, notifyRetryBaseDelay; attempt < notifyRetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(delay)
			delay *= 2
		}
		if s.notifyOnce(target, data) {
			return
		}
	}
}

// notifyOnce performs a single webhook delivery attempt and reports whether
// the receiver acknowledged it.
func (s *Service) notifyOnce(target PushNotificationConfig, data []byte) bool {
	req, err := http.NewRequest(http.MethodPost, target.URL, bytes.NewReader(data))
	if err != nil {
		log.Printf("a2a: push notification request: %v", err)
		return false
	}
	req.Header.Set("Content-Type", "application/a2a+json")
	if target.Authentication != nil && target.Authentication.Credentials != "" {
		scheme := strings.TrimSpace(target.Authentication.Scheme)
		if scheme == "" {
			scheme = "Bearer"
		}
		req.Header.Set("Authorization", scheme+" "+target.Authentication.Credentials)
	} else if target.Token != "" {
		req.Header.Set("Authorization", "Bearer "+target.Token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("a2a: push notification failed: %v", err)
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("a2a: push notification rejected with status %d", resp.StatusCode)
		return false
	}
	return true
}

var errPrivateWebhook = errors.New("url must not point to a loopback, private, or link-local address")

// cgnatRange is the shared address space (RFC 6598) often used by internal
// load balancers; Go's IsPrivate does not cover it.
var cgnatRange = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// validateWebhookURL accepts absolute http(s) URLs and rejects addresses that
// could reach internal services (SSRF guard, spec §13.2). DNS names are
// checked by name only; resolving them is left to the delivery client.
func validateWebhookURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("url must be an absolute http(s) URL")
	}
	if config.A2AAllowPrivateWebhooks {
		return raw, nil
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "", errPrivateWebhook
	}
	if ip := net.ParseIP(host); ip != nil && !isPublicIP(ip) {
		return "", errPrivateWebhook
	}
	return raw, nil
}

func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return !cgnatRange.Contains(ip)
}

func hasContent(msg Message) bool {
	for _, part := range msg.Parts {
		if part.HasContent() {
			return true
		}
	}
	return false
}

func validateParts(parts []Part) error {
	for i, part := range parts {
		if part.Count() == 0 {
			return fmt.Errorf("part %d must contain exactly one of text, raw, url, or data", i)
		}
		if part.Count() > 1 {
			return fmt.Errorf("part %d contains more than one of text, raw, url, and data", i)
		}
	}
	return nil
}

func buildIncomingMessage(taskID, contextID string, userID int, callerName string, message Message) (*channel.IncomingMessage, error) {
	textParts := make([]string, 0, len(message.Parts))
	notes := make([]string, 0, len(message.Parts))
	images := make([]string, 0)
	fileCount := 0

	for _, part := range message.Parts {
		switch {
		case strings.TrimSpace(part.Text) != "":
			textParts = append(textParts, part.Text)
		case part.Data != nil:
			if data, err := json.Marshal(part.Data); err == nil {
				notes = append(notes, "[Data part: "+string(data)+"]")
			}
		case strings.TrimSpace(part.URL) != "":
			notes = append(notes, "[File reference: "+part.URL+"]")
		case len(part.Raw) > 0:
			fileCount++
			if fileCount > maxFilesPerMessage {
				return nil, fmt.Errorf("too many file parts (max %d)", maxFilesPerMessage)
			}
			if len(part.Raw) > maxFileBytes {
				return nil, fmt.Errorf("file %q is too large (max %d bytes)", part.Filename, maxFileBytes)
			}
			name := sanitizeFileName(part.Filename)
			if strings.HasPrefix(strings.ToLower(part.MediaType), "image/") {
				images = append(images, string(part.Raw))
				notes = append(notes, "[Attached image: "+name+"]")
				continue
			}
			dir, err := taskDir(taskID)
			if err != nil {
				return nil, err
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
			abs := filepath.Join(dir, name)
			if err := os.WriteFile(abs, part.Raw, 0o644); err != nil {
				return nil, err
			}
			notes = append(notes, fmt.Sprintf("[Attached file: %s at %s]", name, abs))
		}
	}

	text := strings.TrimSpace(strings.Join(append(textParts, notes...), "\n"))
	if text == "" && len(images) == 0 {
		return nil, fmt.Errorf("message must contain text, raw, url, or data parts")
	}
	if dir, err := taskDir(taskID); err == nil {
		outputDir := filepath.Join(dir, "output")
		text = strings.TrimSpace(text + "\n\n[A2A] Save files for the caller under " + outputDir + "; they will be returned as artifacts.")
	}
	senderName := strings.TrimSpace(callerName)
	if senderName == "" {
		senderName = "anonymous"
	}
	return &channel.IncomingMessage{
		UserID:         userID,
		SenderName:     "a2a:" + senderName,
		Text:           text,
		Images:         images,
		Channel:        "a2a",
		ConversationID: contextID,
	}, nil
}

func taskDir(taskID string) (string, error) {
	jobsDir, err := agentdir.ActiveJobsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(jobsDir, "a2a", taskID), nil
}

// runBot sets MYAAW_A2A_TASK_DIR for the duration of a run. Callers must hold
// runMu because the environment is process-global.
func (s *Service) runBot(taskID string, fn func() (*channel.OutgoingMessage, error)) (*channel.OutgoingMessage, string, string, error) {
	dir, err := taskDir(taskID)
	if err != nil {
		return nil, "", "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", "", err
	}
	previous, hadPrevious := os.LookupEnv(envTaskDir)
	_ = os.Setenv(envTaskDir, dir)
	defer func() {
		if hadPrevious {
			_ = os.Setenv(envTaskDir, previous)
		} else {
			_ = os.Unsetenv(envTaskDir)
		}
	}()

	out, runErr := fn()
	question, _ := readMarker(dir, inputRequiredFile)
	authRequest, _ := readMarker(dir, authRequiredFile)
	return out, question, authRequest, runErr
}

func readMarker(dir, name string) (string, bool) {
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	_ = os.Remove(path)
	var payload struct {
		Message  string `json:"message"`
		Question string `json:"question"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", false
	}
	text := strings.TrimSpace(payload.Message)
	if text == "" {
		text = strings.TrimSpace(payload.Question)
	}
	if text == "" {
		text = "The agent needs more information to continue."
	}
	return text, true
}

// collectFileArtifacts returns files the agent saved under output/ as A2A
// artifacts. Small files are inlined as base64 `raw` parts; larger files are
// exposed as `url` parts and streamed by the artifact endpoint so large files
// (for example video) do not have to fit in one JSON response.
func collectFileArtifacts(taskID string) []Artifact {
	dir, err := taskDir(taskID)
	if err != nil {
		return nil
	}
	outputDir := filepath.Join(dir, "output")
	artifacts := make([]Artifact, 0)
	_ = filepath.WalkDir(outputDir, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry == nil {
			return nil
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if len(artifacts) >= maxFilesPerMessage {
			return fs.SkipAll
		}
		info, err := entry.Info()
		if err != nil || info.Size() > maxArtifactBytes {
			return nil
		}
		rel, err := filepath.Rel(outputDir, filePath)
		if err != nil {
			return nil
		}
		mimeType := mime.TypeByExtension(filepath.Ext(filePath))

		if info.Size() <= inlineFileBytes {
			data, err := os.ReadFile(filePath)
			if err != nil {
				return nil
			}
			if mimeType == "" {
				mimeType = http.DetectContentType(data)
			}
			artifacts = append(artifacts, Artifact{
				ArtifactID: uuid.NewString(),
				Name:       filepath.ToSlash(rel),
				Parts: []Part{{
					Raw:       data,
					Filename:  filepath.Base(rel),
					MediaType: mimeType,
				}},
			})
			return nil
		}

		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		artifacts = append(artifacts, Artifact{
			ArtifactID: uuid.NewString(),
			Name:       filepath.ToSlash(rel),
			Parts: []Part{{
				URL:       artifactURL(taskID, filepath.ToSlash(rel)),
				Filename:  filepath.Base(rel),
				MediaType: mimeType,
			}},
		})
		return nil
	})
	return artifacts
}

// artifactURL builds the absolute download URL for a task artifact.
func artifactURL(taskID, rel string) string {
	return baseURL() + "/tasks/" + url.PathEscape(taskID) + "/artifacts?path=" + url.QueryEscape(rel)
}

// resolveArtifactPath validates a relative artifact path and resolves it under
// the task's output directory, rejecting traversal outside it.
func resolveArtifactPath(taskID, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("path is required")
	}
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("path must be relative")
	}
	dir, err := taskDir(taskID)
	if err != nil {
		return "", err
	}
	outputDir := filepath.Join(dir, "output")
	full := filepath.Join(outputDir, filepath.FromSlash(rel))
	within, err := filepath.Rel(outputDir, full)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes the output directory")
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", fmt.Errorf("artifact not found")
	}
	if info.IsDir() {
		return "", fmt.Errorf("artifact not found")
	}
	return full, nil
}

func sanitizeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "/" || name == "" {
		return "file"
	}
	replacer := strings.NewReplacer("..", "_", "/", "_", "\\", "_", "\x00", "_")
	return replacer.Replace(name)
}

func nowTimestamp() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

func encodeMessageText(msg *Message) string {
	if msg == nil {
		return ""
	}
	parts := make([]string, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		if strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
