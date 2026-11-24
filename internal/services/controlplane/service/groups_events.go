package service

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/channel/api"
	"myaaw/internal/provider"

	"github.com/google/uuid"
)

const groupEventBridgeSyncInterval = 3 * time.Second

// AgentTurnEvent mirrors the JSON payload emitted by a bot server's
// GET /api/events endpoint on `event: conversation.updated`.
type AgentTurnEvent struct {
	Agent          string                  `json:"agent"`
	ConversationID string                  `json:"conversation_id"`
	Scope          string                  `json:"scope,omitempty"`
	Trigger        string                  `json:"trigger"`
	At             string                  `json:"at"`
	MessageID      string                  `json:"message_id,omitempty"`
	Text           string                  `json:"text,omitempty"`
	Timeline       []api.TimelineItem           `json:"timeline,omitempty"`
	Attachments    []provider.MessageAttachment `json:"attachments,omitempty"`
	ExtraMessages  []provider.ExtraMessage      `json:"extra_messages,omitempty"`
}

type groupEventBridge struct {
	mu      sync.Mutex
	wake    chan struct{}
	workers map[string]bridgeWorker
}

type bridgeWorker struct {
	baseURL string
	cancel  context.CancelFunc
}

// StartEventBridge keeps background SSE listeners connected to every running
// agent that belongs to at least one group, so triggered turns (cron,
// heartbeat, subagent) are persisted into group_messages and broadcast to group
// rooms even when no browser tab is open.
func (s *GroupService) StartEventBridge(ctx context.Context) {
	s.startEventBridgeWithInterval(ctx, groupEventBridgeSyncInterval)
}

func (s *GroupService) startEventBridgeWithInterval(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = groupEventBridgeSyncInterval
	}
	bridge := &groupEventBridge{
		wake:    make(chan struct{}, 1),
		workers: make(map[string]bridgeWorker),
	}
	s.mu.Lock()
	s.bridge = bridge
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer func() {
			bridge.mu.Lock()
			for name, w := range bridge.workers {
				w.cancel()
				delete(bridge.workers, name)
			}
			bridge.mu.Unlock()
		}()

		s.reconcileBridgeWorkers(ctx, bridge)
		for {
			select {
			case <-ctx.Done():
				return
			case <-bridge.wake:
				s.reconcileBridgeWorkers(ctx, bridge)
			case <-ticker.C:
				s.reconcileBridgeWorkers(ctx, bridge)
			}
		}
	}()
}

func (s *GroupService) notifyEventBridge() {
	s.mu.Lock()
	bridge := s.bridge
	s.mu.Unlock()
	if bridge == nil {
		return
	}
	select {
	case bridge.wake <- struct{}{}:
	default:
	}
}

func (s *GroupService) reconcileBridgeWorkers(ctx context.Context, bridge *groupEventBridge) {
	if ctx.Err() != nil {
		return
	}
	stored, err := s.store.List()
	if err != nil {
		return
	}
	desired := make(map[string]string)
	for _, sg := range stored {
		for _, member := range sg.Group.Members {
			if _, seen := desired[member.Name]; seen {
				continue
			}
			info, getErr := s.agents.Get(member.Name)
			if getErr != nil || !info.Running || strings.TrimSpace(info.BaseURL) == "" {
				continue
			}
			desired[member.Name] = strings.TrimRight(strings.TrimSpace(info.BaseURL), "/")
		}
	}

	bridge.mu.Lock()
	defer bridge.mu.Unlock()

	for name, w := range bridge.workers {
		nextURL, ok := desired[name]
		if !ok || nextURL != w.baseURL {
			w.cancel()
			delete(bridge.workers, name)
		}
	}
	for name, baseURL := range desired {
		if _, exists := bridge.workers[name]; exists {
			continue
		}
		workerCtx, cancel := context.WithCancel(ctx)
		bridge.workers[name] = bridgeWorker{baseURL: baseURL, cancel: cancel}
		go s.tailAgentTurnEvents(workerCtx, name, baseURL)
	}
}

func (s *GroupService) tailAgentTurnEvents(ctx context.Context, agentName, baseURL string) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/events", nil)
		if err == nil {
			req.Header.Set("Accept", "text/event-stream")
			resp, doErr := s.httpClient.Do(req)
			if doErr == nil {
				if resp.StatusCode == http.StatusOK {
					backoff = time.Second
					s.consumeAgentEventStream(ctx, agentName, resp.Body)
				}
				_ = resp.Body.Close()
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			backoff *= 2
			if backoff > 10*time.Second {
				backoff = 10 * time.Second
			}
		}
	}
}

func (s *GroupService) consumeAgentEventStream(ctx context.Context, agentName string, body interface{ Read([]byte) (int, error) }) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var (
		eventName string
		dataLines []string
	)
	flush := func() {
		if len(dataLines) == 0 {
			eventName = ""
			return
		}
		raw := strings.TrimSpace(strings.Join(dataLines, "\n"))
		name := eventName
		eventName = ""
		dataLines = nil
		if name != "conversation.updated" || raw == "" {
			return
		}
		var ev AgentTurnEvent
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			return
		}
		if _, _, err := s.IngestAgentTurnEvent(agentName, ev); err != nil {
			log.Printf("[groups] failed to ingest agent event agent=%s scope=%s: %v", agentName, ev.Scope, err)
		}
	}

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if ctx.Err() == nil {
		flush()
	}
}

// IngestAgentTurnEvent persists a triggered agent turn (cron, heartbeat,
// subagent) whose scope targets a control-plane group, updates that member's
// read cursor, and broadcasts the message to room and global SSE subscribers.
// Returns (msg, true, nil) when a new message was persisted, or (zero, false, nil)
// when the event was ignored or already ingested.
func (s *GroupService) IngestAgentTurnEvent(agentName string, ev AgentTurnEvent) (GroupMessage, bool, error) {
	trigger := strings.ToLower(strings.TrimSpace(ev.Trigger))
	switch trigger {
	case "cron", "heartbeat", "subagent":
	default:
		return GroupMessage{}, false, nil
	}

	scope := strings.TrimSpace(ev.Scope)
	if !strings.HasPrefix(scope, "group:") {
		return GroupMessage{}, false, nil
	}
	groupID := strings.TrimSpace(strings.TrimPrefix(scope, "group:"))
	if groupID == "" {
		return GroupMessage{}, false, nil
	}

	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		agentName = strings.TrimSpace(ev.Agent)
	}
	if agentName == "" {
		return GroupMessage{}, false, nil
	}

	rawText := strings.TrimSpace(ev.Text)
	if trigger == "heartbeat" && rawText == "HEARTBEAT_OK" {
		return GroupMessage{}, false, nil
	}
	cleanedText, _ := extractDoneSignal(rawText)
	if strings.EqualFold(strings.TrimSpace(cleanedText), "[PASS]") {
		cleanedText = ""
	}
	if cleanedText == "" && len(ev.Attachments) == 0 && len(ev.ExtraMessages) == 0 {
		return GroupMessage{}, false, nil
	}

	agentColor := agentdir.DefaultAgentColor
	if info, err := s.agents.Get(agentName); err == nil {
		agentColor = agentdir.AgentColorOrDefault(info.AgentColor)
	}

	msgID := strings.TrimSpace(ev.MessageID)
	if msgID == "" {
		msgID = uuid.NewString()
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(ev.At)); err == nil {
		createdAt = parsed.UTC().Format(time.RFC3339)
	}

	var (
		savedMsg GroupMessage
		inserted bool
	)
	updatedSG, err := s.store.Update(groupID, func(cur *storedGroup) error {
		if !hasMember(cur.Group.Members, agentName) {
			return nil
		}
		if _, exists := findMessageByID(cur.Messages, msgID); exists {
			return nil
		}

		var msgAttachments []GroupAttachment
		if len(ev.Attachments) > 0 {
			msgAttachments = make([]GroupAttachment, 0, len(ev.Attachments))
			for _, a := range ev.Attachments {
				if strings.TrimSpace(a.Path) == "" {
					continue
				}
				kind := strings.TrimSpace(a.Kind)
				if kind == "" {
					kind = attachmentKindFromMime(a.FileName, a.MimeType)
				}
				msgAttachments = append(msgAttachments, GroupAttachment{
					ID:          uuid.NewString(),
					GroupID:     groupID,
					MessageID:   msgID,
					SenderType:  "agent",
					SenderName:  agentName,
					Agent:       agentName,
					Path:        a.Path,
					ProjectType: a.ProjectType,
					FileName:    a.FileName,
					MimeType:    a.MimeType,
					Kind:        kind,
					Size:        a.Size,
					CreatedAt:   createdAt,
				})
			}
		} else {
			msgAttachments = collectAgentMessageAttachments(groupID, msgID, agentName, cleanedText, createdAt)
		}

		savedMsg = GroupMessage{
			ID:            msgID,
			GroupID:       groupID,
			SenderType:    "agent",
			SenderName:    agentName,
			AgentColor:    agentColor,
			Content:       cleanedText,
			Timeline:      finalizeTimelineText(ev.Timeline, cleanedText),
			Attachments:   msgAttachments,
			ExtraMessages: provider.CloneExtraMessages(ev.ExtraMessages),
			CreatedAt:     createdAt,
		}
		cur.Messages = append(cur.Messages, savedMsg)
		cur.Group.UpdatedAt = createdAt
		for i := range cur.Group.Members {
			if cur.Group.Members[i].Name == agentName {
				cur.Group.Members[i].LastSeenMessageID = savedMsg.ID
				if convID := strings.TrimSpace(ev.ConversationID); convID != "" {
					cur.Group.Members[i].ConversationID = convID
				}
			}
		}
		inserted = true
		return nil
	})
	if err != nil {
		if err == ErrGroupNotFound {
			return GroupMessage{}, false, nil
		}
		return GroupMessage{}, false, err
	}
	if !inserted {
		return GroupMessage{}, false, nil
	}

	rt := s.runtimeFor(groupID, updatedSG.Group.MaxTurns)
	rt.broadcast(GroupSSEEvent{
		Name: "message",
		Data: map[string]any{
			"group_id": groupID,
			"message":  savedMsg,
		},
	})
	return savedMsg, true, nil
}
