package a2a

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
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
	defaultRunTimeout  = 15 * time.Minute
	maxTaskAttempts    = 3
	maxStreamEvents    = 2000
	streamRetention    = 10 * time.Minute

	envTaskDir        = "MYAAW_A2A_TASK_DIR"
	inputRequiredFile = "input-required.json"
)

var taskIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

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

	callers          []callerCredential
	signingKey       ed25519.PrivateKey
	trustedKeys      map[string]ed25519.PublicKey
	trustedNames     map[string]string
	requireSignature bool

	runsMu sync.Mutex
	runs   map[string]context.CancelFunc

	streamsMu sync.Mutex
	streams   map[string]*taskStream
}

func NewService(store *TaskStore, runner Runner) *Service {
	service := &Service{
		store:   store,
		runner:  runner,
		runs:    make(map[string]context.CancelFunc),
		streams: make(map[string]*taskStream),
	}
	service.loadCallers()
	service.loadSigning()
	return service
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
	if len(s.signingKey) > 0 {
		pub := s.signingKey.Public().(ed25519.PublicKey)
		card.PublicKey = EncodePublicKey(pub)
		card.KeyID = KeyID(pub)
		card = s.signCard(card)
	}
	return card
}

// BuildCard builds an Agent Card from explicit inputs. It is exported for
// tests and for future multi-agent card generation.
func BuildCard(name, baseURL string, skillList []skills.Skill, requireAuth bool) AgentCard {
	card := AgentCard{
		Name:               name,
		Description:        "Myaaw agent " + name,
		URL:                strings.TrimRight(baseURL, "/") + "/a2a",
		Version:            "1.0.0",
		Capabilities:       Capabilities{Streaming: true, PushNotifications: true},
		DefaultInputModes:  []string{"text", "file"},
		DefaultOutputModes: []string{"text"},
		Skills:             []AgentSkill{},
	}
	for _, skill := range skillList {
		if !skill.Enabled {
			continue
		}
		card.Skills = append(card.Skills, AgentSkill{
			ID:          skill.Name,
			Name:        skill.DisplayName,
			Description: skill.Description,
		})
	}
	if requireAuth {
		card.SecuritySchemes = []SecurityScheme{{Type: "http", Scheme: "bearer"}}
	}
	return card
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

// authTokens returns all accepted incoming tokens. A2A_TOKENS allows one token
// per caller; A2A_TOKEN remains supported as the single-token shorthand.
func authTokens() []string {
	tokens := make([]string, 0, len(config.A2ATokens)+1)
	if config.A2AToken != "" {
		tokens = append(tokens, config.A2AToken)
	}
	for _, token := range config.A2ATokens {
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// SendTask creates a task and either runs it synchronously or schedules it in
// the background when params.Async is true.
func (s *Service) SendTask(ctx context.Context, params SendTaskParams) (Task, error) {
	if params.Async {
		return s.SendTaskAsync(ctx, params)
	}

	task, msg, err := s.prepareTask(ctx, params, StateWorking)
	if err != nil {
		return Task{}, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.registerRun(task.ID, cancel)
	defer s.unregisterRun(task.ID)

	s.runMu.Lock()
	out, question, err := s.runBot(task.ID, func() (*channel.OutgoingMessage, error) {
		return s.runner.BotContext(runCtx, msg)
	})
	s.runMu.Unlock()
	if err != nil {
		return s.failTask(ctx, task, err), nil
	}
	if question != "" {
		return s.inputRequired(ctx, task, question), nil
	}
	return s.completeTask(ctx, task, out)
}

// SendTaskAsync stores a task in submitted state and runs it in the background.
// The result is written to the task store and, when pushNotificationUrl is set,
// POSTed to that URL as a Task JSON document.
func (s *Service) SendTaskAsync(ctx context.Context, params SendTaskParams) (Task, error) {
	pushURL, err := validatePushURL(params.PushNotificationURL)
	if err != nil {
		return Task{}, err
	}
	params.PushNotificationURL = pushURL

	task, msg, err := s.prepareTask(ctx, params, StateSubmitted)
	if err != nil {
		return Task{}, err
	}
	go s.runTask(task, msg)
	return task, nil
}

// SendTaskStream runs a task and emits status/artifact events as text chunks
// arrive. The final artifact replaces the streamed deltas (append=false) and
// carries lastChunk=true.
func (s *Service) SendTaskStream(ctx context.Context, params SendTaskParams, send func(StreamEvent)) (Task, error) {
	task, msg, err := s.prepareTask(ctx, params, StateWorking)
	if err != nil {
		return Task{}, err
	}
	s.startStream(task.ID)
	defer s.finishStream(task.ID)

	emit := func(event StreamEvent) {
		s.publishEvent(task.ID, event)
		if send != nil {
			send(event)
		}
	}
	emitStatus := func(final bool) {
		status := task.Status
		emit(StreamEvent{Kind: "status", TaskID: task.ID, Status: &status, Final: final})
	}
	emitStatus(false)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.registerRun(task.ID, cancel)
	defer s.unregisterRun(task.ID)

	out, question, err := func() (*channel.OutgoingMessage, string, error) {
		s.runMu.Lock()
		defer s.runMu.Unlock()
		return s.runBot(task.ID, func() (*channel.OutgoingMessage, error) {
			return s.runner.BotStreamContext(runCtx, msg, func(chunk channel.StreamChunk) {
				if chunk.Kind != "text" || chunk.Text == "" {
					return
				}
				emit(StreamEvent{
					Kind:   "artifact",
					TaskID: task.ID,
					Artifact: &Artifact{
						Name:  "response",
						Parts: []Part{{Kind: "text", Text: chunk.Text}},
					},
					Append: true,
				})
			})
		})
	}()
	if err != nil {
		task = s.failTask(ctx, task, err)
		emitStatus(true)
		return task, nil
	}
	if question != "" {
		task = s.inputRequired(ctx, task, question)
		emitStatus(true)
		return task, nil
	}

	task, err = s.completeTask(ctx, task, out)
	if err != nil {
		return Task{}, err
	}
	for i := range task.Artifacts {
		artifact := task.Artifacts[i]
		emit(StreamEvent{Kind: "artifact", TaskID: task.ID, Artifact: &artifact, LastChunk: true})
	}
	emitStatus(true)
	return task, nil
}

func (s *Service) GetTask(ctx context.Context, id string) (Task, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	return s.store.Get(ctx, id)
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
		task.Status = TaskStatus{State: StateSubmitted, Timestamp: time.Now().UTC().Format(time.RFC3339)}
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

// CancelTask marks a non-terminal task as canceled. Terminal tasks are
// returned unchanged.
func (s *Service) CancelTask(ctx context.Context, id string) (Task, error) {
	task, err := s.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	switch task.Status.State {
	case StateCompleted, StateFailed, StateCanceled:
		return task, nil
	}
	task.Status = TaskStatus{State: StateCanceled, Timestamp: time.Now().UTC().Format(time.RFC3339)}
	if err := s.store.Update(ctx, task); err != nil {
		return Task{}, err
	}
	s.cancelRun(id)
	s.notifyTaskAsync(task.ID)
	return task, nil
}

// taskStream buffers events for tasks/sendSubscribe so a reconnecting caller
// can replay them with tasks/resubscribe.
type taskStream struct {
	events []StreamEvent
	subs   map[chan StreamEvent]struct{}
	done   bool
}

func (s *Service) startStream(id string) {
	s.streamsMu.Lock()
	s.streams[id] = &taskStream{subs: make(map[chan StreamEvent]struct{})}
	s.streamsMu.Unlock()
}

func (s *Service) publishEvent(id string, event StreamEvent) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	stream := s.streams[id]
	if stream == nil {
		return
	}
	if len(stream.events) < maxStreamEvents {
		stream.events = append(stream.events, event)
	} else {
		copy(stream.events, stream.events[1:])
		stream.events[len(stream.events)-1] = event
	}
	for ch := range stream.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (s *Service) subscribeStream(id string) ([]StreamEvent, chan StreamEvent, bool, bool) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	stream := s.streams[id]
	if stream == nil {
		return nil, nil, false, false
	}
	snapshot := append([]StreamEvent(nil), stream.events...)
	if stream.done {
		return snapshot, nil, false, true
	}
	events := make(chan StreamEvent, 256)
	stream.subs[events] = struct{}{}
	return snapshot, events, true, true
}

func (s *Service) unsubscribeStream(id string, events chan StreamEvent) {
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

func (s *Service) prepareTask(ctx context.Context, params SendTaskParams, state string) (Task, *channel.IncomingMessage, error) {
	if !hasContent(params.Message) {
		return Task{}, nil, fmt.Errorf("message must contain text, data, or a file part")
	}

	id := strings.TrimSpace(params.ID)
	if id == "" {
		id = strings.TrimSpace(params.Message.TaskID)
	}
	var existing *Task
	if id == "" {
		id = uuid.NewString()
	} else {
		if !taskIDPattern.MatchString(id) {
			return Task{}, nil, fmt.Errorf("task id may only contain letters, numbers, '_' and '-'")
		}
		got, err := s.store.Get(ctx, id)
		if err == nil {
			if got.Status.State != StateInputRequired {
				return Task{}, nil, ErrTaskExists
			}
			existing = &got
		} else if !errors.Is(err, ErrTaskNotFound) {
			return Task{}, nil, err
		}
	}

	contextID := strings.TrimSpace(params.ContextID)
	if contextID == "" {
		contextID = strings.TrimSpace(params.Message.ContextID)
	}
	if existing != nil && existing.ContextID != "" {
		contextID = existing.ContextID
	} else if contextID == "" {
		contextID = "NEW"
	}

	callerName := strings.TrimSpace(params.Caller)
	if callerName == "" {
		callerName = "anonymous"
	}
	if existing != nil && existing.Caller != "" {
		if !strings.EqualFold(existing.Caller, callerName) {
			return Task{}, nil, ErrTaskNotFound
		}
		callerName = existing.Caller
	}
	userID, err := s.store.EnsureCaller(ctx, callerName)
	if err != nil {
		return Task{}, nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	userMessage := Message{
		Kind:      "message",
		MessageID: strings.TrimSpace(params.Message.MessageID),
		Role:      "user",
		Parts:     params.Message.Parts,
	}
	if userMessage.MessageID == "" {
		userMessage.MessageID = uuid.NewString()
	}

	var task Task
	if existing != nil {
		task = *existing
		task.Kind = "task"
		task.ContextID = contextID
		task.Caller = callerName
		task.Messages = append(task.Messages, userMessage)
		task.Status = TaskStatus{State: state, Timestamp: now}
		if err := s.store.Update(ctx, task); err != nil {
			return Task{}, nil, err
		}
	} else {
		task = Task{
			Kind:      "task",
			ID:        id,
			ContextID: contextID,
			Status:    TaskStatus{State: state, Timestamp: now},
			Messages:  []Message{userMessage},
			Attempts:  1,
			PushURL:   strings.TrimSpace(params.PushNotificationURL),
			Caller:    callerName,
		}
		if err := s.store.Create(ctx, task); err != nil {
			return Task{}, nil, err
		}
	}

	msg, err := buildIncomingMessage(id, contextID, userID, callerName, params.Message)
	if err != nil {
		_ = s.failTask(ctx, task, err)
		return Task{}, nil, err
	}
	return task, msg, nil
}

func (s *Service) completeTask(ctx context.Context, task Task, out *channel.OutgoingMessage) (Task, error) {
	if current, ok := s.canceledTask(ctx, task); ok {
		return current, nil
	}
	if out != nil {
		if out.ConversationID != "" {
			task.ContextID = out.ConversationID
		}
		reply := Message{Kind: "message", MessageID: uuid.NewString(), Role: "agent", Parts: []Part{{Kind: "text", Text: out.Text}}}
		task.Messages = append(task.Messages, reply)
		task.Artifacts = append(task.Artifacts, Artifact{Name: "response", Parts: []Part{{Kind: "text", Text: out.Text}}})
	}
	task.Artifacts = append(task.Artifacts, collectFileArtifacts(task.ID)...)
	task.Status = TaskStatus{State: StateCompleted, Timestamp: time.Now().UTC().Format(time.RFC3339)}
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
	msg := Message{Kind: "message", MessageID: uuid.NewString(), Role: "agent", Parts: []Part{{Kind: "text", Text: question}}}
	task.Status = TaskStatus{State: StateInputRequired, Timestamp: time.Now().UTC().Format(time.RFC3339), Message: &msg}
	task.Messages = append(task.Messages, msg)
	_ = s.store.Update(ctx, task)
	s.notifyTaskAsync(task.ID)
	return task
}

func (s *Service) failTask(ctx context.Context, task Task, runErr error) Task {
	if current, ok := s.canceledTask(ctx, task); ok {
		return current
	}
	failure := Message{Kind: "message", MessageID: uuid.NewString(), Role: "agent", Parts: []Part{{Kind: "text", Text: runErr.Error()}}}
	task.Status = TaskStatus{State: StateFailed, Timestamp: time.Now().UTC().Format(time.RFC3339), Message: &failure}
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

	task.Status = TaskStatus{State: StateWorking, Timestamp: time.Now().UTC().Format(time.RFC3339)}
	if err := s.store.Update(ctx, task); err != nil {
		log.Printf("a2a: update task %s: %v", task.ID, err)
	}

	out, question, err := s.runBot(task.ID, func() (*channel.OutgoingMessage, error) {
		return s.runner.BotContext(ctx, msg)
	})
	if err != nil {
		task = s.failTask(ctx, task, err)
	} else if question != "" {
		task = s.inputRequired(ctx, task, question)
	} else {
		var completeErr error
		task, completeErr = s.completeTask(ctx, task, out)
		if completeErr != nil {
			log.Printf("a2a: complete task %s: %v", task.ID, completeErr)
		}
	}
}

// notifyTask sends the final task document to every registered webhook: the
// per-request pushNotificationUrl and any tasks/pushNotificationConfig entries.
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

	seen := make(map[string]bool, len(targets))
	for _, target := range targets {
		if target.URL == "" || seen[target.URL] {
			continue
		}
		seen[target.URL] = true
		s.notify(target.URL, target.Token, task)
	}
}

func (s *Service) notifyTaskAsync(taskID string) {
	go s.notifyTask(taskID)
}

func (s *Service) notify(target, token string, task Task) {
	if target == "" {
		return
	}
	data, err := json.Marshal(task.ForResponse())
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(data))
	if err != nil {
		log.Printf("a2a: push notification request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Myaaw-Task-ID", task.ID)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("a2a: push notification failed: %v", err)
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func validatePushURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("pushNotificationUrl must be an absolute http(s) URL")
	}
	return raw, nil
}

func hasContent(msg Message) bool {
	for _, part := range msg.Parts {
		switch part.Kind {
		case "text":
			if strings.TrimSpace(part.Text) != "" {
				return true
			}
		case "data":
			if part.Data != nil {
				return true
			}
		case "file":
			if part.File != nil && (part.File.Bytes != "" || part.File.URI != "") {
				return true
			}
		}
	}
	return false
}

func buildIncomingMessage(taskID, contextID string, userID int, callerName string, message Message) (*channel.IncomingMessage, error) {
	textParts := make([]string, 0, len(message.Parts))
	notes := make([]string, 0, len(message.Parts))
	images := make([]string, 0)
	fileCount := 0

	for _, part := range message.Parts {
		switch part.Kind {
		case "text":
			if strings.TrimSpace(part.Text) != "" {
				textParts = append(textParts, part.Text)
			}
		case "data":
			if part.Data != nil {
				if data, err := json.Marshal(part.Data); err == nil {
					notes = append(notes, "[Data part: "+string(data)+"]")
				}
			}
		case "file":
			if part.File == nil {
				continue
			}
			fileCount++
			if fileCount > maxFilesPerMessage {
				return nil, fmt.Errorf("too many file parts (max %d)", maxFilesPerMessage)
			}
			if part.File.URI != "" {
				return nil, fmt.Errorf("file URI parts are not supported; send base64 bytes")
			}
			if part.File.Bytes == "" {
				return nil, fmt.Errorf("file part is missing bytes")
			}
			decoded, err := base64.StdEncoding.DecodeString(part.File.Bytes)
			if err != nil {
				return nil, fmt.Errorf("invalid base64 file bytes")
			}
			if len(decoded) > maxFileBytes {
				return nil, fmt.Errorf("file %q is too large (max %d bytes)", part.File.Name, maxFileBytes)
			}
			name := sanitizeFileName(part.File.Name)
			if strings.HasPrefix(strings.ToLower(part.File.MimeType), "image/") {
				images = append(images, part.File.Bytes)
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
			if err := os.WriteFile(abs, decoded, 0o644); err != nil {
				return nil, err
			}
			notes = append(notes, fmt.Sprintf("[Attached file: %s at %s]", name, abs))
		}
	}

	text := strings.TrimSpace(strings.Join(append(textParts, notes...), "\n"))
	if text == "" && len(images) == 0 {
		return nil, fmt.Errorf("message must contain text, data, or a file part")
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
func (s *Service) runBot(taskID string, fn func() (*channel.OutgoingMessage, error)) (*channel.OutgoingMessage, string, error) {
	dir, err := taskDir(taskID)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
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
	question, _ := readInputRequired(dir)
	return out, question, runErr
}

func readInputRequired(dir string) (string, bool) {
	path := filepath.Join(dir, inputRequiredFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	_ = os.Remove(path)
	var payload struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", false
	}
	question := strings.TrimSpace(payload.Question)
	if question == "" {
		question = "The agent needs more information to continue."
	}
	return question, true
}

// collectFileArtifacts returns files the agent saved under output/ as A2A
// artifacts (base64). Files are capped per artifact and per task.
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
		if err != nil || info.Size() > maxFileBytes {
			return nil
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(outputDir, filePath)
		if err != nil {
			return nil
		}
		mimeType := mime.TypeByExtension(filepath.Ext(filePath))
		if mimeType == "" {
			mimeType = http.DetectContentType(data)
		}
		artifacts = append(artifacts, Artifact{
			Name: filepath.ToSlash(rel),
			Parts: []Part{{
				Kind: "file",
				File: &FilePart{
					Name:     filepath.Base(rel),
					MimeType: mimeType,
					Bytes:    base64.StdEncoding.EncodeToString(data),
				},
			}},
		})
		return nil
	})
	return artifacts
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "_")
	name = filepath.Base(name)
	if name == "" || name == "." || name == ".." {
		return "attachment.bin"
	}
	return name
}

func messageText(msg Message) string {
	parts := make([]string, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		if part.Kind == "text" && strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
