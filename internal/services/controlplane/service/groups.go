package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"myaaw/internal/agentdir"
	"myaaw/internal/attachment"
	"myaaw/internal/channel"
	"myaaw/internal/channel/api"

	"github.com/google/uuid"
)

const (
	maxGroupNameRunes        = 100
	maxGroupDescriptionRunes = 2000
	maxReplyExcerptRunes     = 180
	maxReplyPromptRunes      = 1200
	defaultHistoryWindow     = 30
	groupStopHTTPTimeout     = 3 * time.Second
	groupSubBufferSize       = 256
)

var (
	errAgentConversationNotFound = errors.New("agent conversation not found")

	fencedCodeBlockPattern = regexp.MustCompile("(?s)```.*?```")
	inlineCodePattern      = regexp.MustCompile("`[^`]*`")
	mentionPattern         = regexp.MustCompile(`(?:^|[^\w.@/])@([A-Za-z0-9][A-Za-z0-9_-]*)`)
	doneTokenPattern       = regexp.MustCompile(`(?i)(?:^|[\s\n])\[DONE\](?:$|[\s\n.,!?;:])`)
	doneStripPattern       = regexp.MustCompile(`(?i)(^|\s)\[DONE\][.,!?;:]*(\s|$)`)
	closingSelectionTag    = regexp.MustCompile(`(?i)</(selection|selections)>`)
	closingGroupTag        = regexp.MustCompile(`(?i)</(group|group_context|chats)>`)
	attrEscapePattern      = regexp.MustCompile(`["<>&]`)
)

// AgentDirectory is the subset of AgentService needed by GroupService.
type AgentDirectory interface {
	Get(name string) (AgentInfo, error)
	BaseURL(name string) (string, int, error)
}

type groupRuntime struct {
	mu                sync.Mutex
	state             GroupRunState
	runGen            uint64
	cancel            context.CancelFunc
	activeAgent       string
	activeRequestID   string
	triggerMsgID      string
	triggerImages     []string
	triggerFiles      []channel.FileUpload
	initialMembers    map[string]bool
	spokenInRun       map[string]bool
	directlyAddressed map[string]bool
	subs              map[chan GroupSSEEvent]struct{}
}

func newGroupRuntime(maxTurns int) *groupRuntime {
	if maxTurns <= 0 {
		maxTurns = DefaultGroupMaxTurns
	}
	return &groupRuntime{
		state: GroupRunState{
			MaxTurns: maxTurns,
			Queue:    []string{},
		},
		subs: make(map[chan GroupSSEEvent]struct{}),
	}
}

func (rt *groupRuntime) snapshotLocked() GroupRunState {
	st := rt.state
	st.Queue = append([]string{}, rt.state.Queue...)
	if len(rt.state.PendingApprovals) > 0 {
		st.PendingApprovals = append([]GroupPendingApproval{}, rt.state.PendingApprovals...)
	} else {
		st.PendingApprovals = nil
	}
	return st
}

func (rt *groupRuntime) upsertApproval(approval GroupPendingApproval) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for i, existing := range rt.state.PendingApprovals {
		if existing.ApprovalID == approval.ApprovalID {
			rt.state.PendingApprovals[i] = approval
			return
		}
	}
	rt.state.PendingApprovals = append(rt.state.PendingApprovals, approval)
}

func (rt *groupRuntime) removeApproval(approvalID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.state.PendingApprovals) == 0 {
		return
	}
	next := rt.state.PendingApprovals[:0]
	for _, existing := range rt.state.PendingApprovals {
		if existing.ApprovalID != approvalID {
			next = append(next, existing)
		}
	}
	if len(next) == 0 {
		rt.state.PendingApprovals = nil
	} else {
		rt.state.PendingApprovals = next
	}
}

func (rt *groupRuntime) clearRunApprovals(runID string) {
	if runID == "" {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.state.PendingApprovals) == 0 {
		return
	}
	next := rt.state.PendingApprovals[:0]
	for _, existing := range rt.state.PendingApprovals {
		if existing.RunID != runID {
			next = append(next, existing)
		}
	}
	if len(next) == 0 {
		rt.state.PendingApprovals = nil
	} else {
		rt.state.PendingApprovals = next
	}
}

func (rt *groupRuntime) broadcast(ev GroupSSEEvent) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.broadcastLocked(ev)
}

func (rt *groupRuntime) broadcastLocked(ev GroupSSEEvent) {
	for ch := range rt.subs {
		select {
		case ch <- ev:
		default:
			// Drop for slow consumers so one stalled browser never blocks orchestration.
		}
	}
}

// GroupService owns group CRUD, live SSE fan-out, and sequential multi-agent turn orchestration.
type GroupService struct {
	store      *GroupStore
	agents     AgentDirectory
	httpClient *http.Client
	stopClient *http.Client

	mu       sync.Mutex
	runtimes map[string]*groupRuntime
}

// NewGroupService builds a GroupService backed by ~/.myaaw/controlplane/controlplane.db.
func NewGroupService(agents AgentDirectory) (*GroupService, error) {
	store, err := NewGroupStore()
	if err != nil {
		return nil, err
	}
	return NewGroupServiceWithStore(store, agents, nil), nil
}

// NewGroupServiceWithDB builds a GroupService backed by a shared *ControlPlaneDB.
func NewGroupServiceWithDB(db *ControlPlaneDB, agents AgentDirectory) *GroupService {
	return NewGroupServiceWithStore(NewGroupStoreWithDB(db), agents, nil)
}

// NewGroupServiceWithStore builds a GroupService with custom store and HTTP client (ideal for tests).
func NewGroupServiceWithStore(store *GroupStore, agents AgentDirectory, httpClient *http.Client) *GroupService {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	stopClient := &http.Client{
		Timeout:   groupStopHTTPTimeout,
		Transport: httpClient.Transport,
	}
	return &GroupService{
		store:      store,
		agents:     agents,
		httpClient: httpClient,
		stopClient: stopClient,
		runtimes:   make(map[string]*groupRuntime),
	}
}

func (s *GroupService) runtimeFor(id string, maxTurns int) *groupRuntime {
	s.mu.Lock()
	defer s.mu.Unlock()
	rt, ok := s.runtimes[id]
	if !ok {
		rt = newGroupRuntime(maxTurns)
		s.runtimes[id] = rt
	}
	return rt
}

func (s *GroupService) getRuntime(id string) *groupRuntime {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runtimes[id]
}

// List returns all groups with hydrated member status, live run status, and last message preview.
func (s *GroupService) List() ([]GroupSummary, error) {
	stored, err := s.store.List()
	if err != nil {
		return nil, err
	}

	summaries := make([]GroupSummary, 0, len(stored))
	for _, sg := range stored {
		members := s.hydrateMembers(sg.Group.Members)
		summary := GroupSummary{
			ID:             sg.Group.ID,
			Name:           sg.Group.Name,
			Description:    sg.Group.Description,
			Members:        members,
			MemberCount:    len(members),
			DiscussionMode: sg.Group.DiscussionMode,
			MaxTurns:       sg.Group.MaxTurns,
			CreatedAt:      sg.Group.CreatedAt,
			UpdatedAt:      sg.Group.UpdatedAt,
		}
		if rt := s.getRuntime(sg.Group.ID); rt != nil {
			rt.mu.Lock()
			summary.Running = rt.state.Running
			summary.CurrentAgent = rt.state.CurrentAgent
			summary.PendingApprovalCount = len(rt.state.PendingApprovals)
			rt.mu.Unlock()
		}
		if n := len(sg.Messages); n > 0 {
			last := sg.Messages[n-1]
			summary.LastMessage = &GroupLastMessage{
				ID:         last.ID,
				SenderType: last.SenderType,
				SenderName: last.SenderName,
				Content:    last.Content,
				CreatedAt:  last.CreatedAt,
			}
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// Create validates and creates a new group.
func (s *GroupService) Create(req CreateGroupRequest) (Group, error) {
	name, err := validateGroupName(req.Name)
	if err != nil {
		return Group{}, err
	}
	description, err := validateGroupDescription(req.Description)
	if err != nil {
		return Group{}, err
	}
	mode, err := validateDiscussionMode(req.DiscussionMode, true)
	if err != nil {
		return Group{}, err
	}
	maxTurns, err := validateMaxTurns(req.MaxTurns, true)
	if err != nil {
		return Group{}, err
	}
	memberNames, err := s.validateMemberNames(req.Members)
	if err != nil {
		return Group{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	members := make([]GroupMember, 0, len(memberNames))
	for _, m := range memberNames {
		members = append(members, GroupMember{Name: m})
	}

	group := Group{
		ID:             uuid.NewString(),
		Name:           name,
		Description:    description,
		Members:        members,
		DiscussionMode: mode,
		MaxTurns:       maxTurns,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	sg := &storedGroup{
		Group:    group,
		Messages: []GroupMessage{},
	}
	if err := s.store.Create(sg); err != nil {
		return Group{}, err
	}

	group.Members = s.hydrateMembers(group.Members)
	return group, nil
}

// Get returns full group metadata, full transcript, and current run state.
func (s *GroupService) Get(id string) (GroupDetail, error) {
	return s.GetPage(id, 0, 0, "")
}

// GetPage returns group metadata, a page of its transcript, and current run state.
func (s *GroupService) GetPage(id string, limit, offset int, beforeID string) (GroupDetail, error) {
	page, err := s.store.GetPage(id, limit, offset, strings.TrimSpace(beforeID))
	if err != nil {
		return GroupDetail{}, err
	}

	page.Group.Members = s.hydrateMembers(page.Group.Members)
	for i := range page.Messages {
		if page.Messages[i].SenderType == "agent" && len(page.Messages[i].Attachments) == 0 && page.Messages[i].SenderName != "" {
			if collected := collectAgentMessageAttachments(page.Group.ID, page.Messages[i].ID, page.Messages[i].SenderName, page.Messages[i].Content, page.Messages[i].CreatedAt); len(collected) > 0 {
				page.Messages[i].Attachments = collected
			}
		}
	}
	runState := GroupRunState{
		MaxTurns: page.Group.MaxTurns,
		Queue:    []string{},
	}
	if rt := s.getRuntime(id); rt != nil {
		rt.mu.Lock()
		runState = rt.snapshotLocked()
		if !runState.Running && !runState.Paused {
			runState.MaxTurns = page.Group.MaxTurns
		}
		rt.mu.Unlock()
	}

	return GroupDetail{
		Group:         page.Group,
		Messages:      page.Messages,
		TotalMessages: page.TotalMessages,
		Offset:        page.Offset,
		Limit:         page.Limit,
		HasMore:       page.HasMore,
		RunState:      runState,
	}, nil
}

// Update modifies group settings and/or member roster.
func (s *GroupService) Update(id string, req UpdateGroupRequest) (Group, error) {
	var validatedName *string
	if req.Name != nil {
		v, err := validateGroupName(*req.Name)
		if err != nil {
			return Group{}, err
		}
		validatedName = &v
	}
	var validatedDesc *string
	if req.Description != nil {
		v, err := validateGroupDescription(*req.Description)
		if err != nil {
			return Group{}, err
		}
		validatedDesc = &v
	}
	var validatedMode *string
	if req.DiscussionMode != nil {
		v, err := validateDiscussionMode(*req.DiscussionMode, false)
		if err != nil {
			return Group{}, err
		}
		validatedMode = &v
	}
	var validatedMaxTurns *int
	if req.MaxTurns != nil {
		v, err := validateMaxTurns(*req.MaxTurns, false)
		if err != nil {
			return Group{}, err
		}
		validatedMaxTurns = &v
	}
	var validatedMembers []string
	if req.Members != nil {
		names, err := s.validateMemberNames(*req.Members)
		if err != nil {
			return Group{}, err
		}
		validatedMembers = names
	}

	updated, err := s.store.Update(id, func(sg *storedGroup) error {
		if validatedName != nil {
			sg.Group.Name = *validatedName
		}
		if validatedDesc != nil {
			sg.Group.Description = *validatedDesc
		}
		if validatedMode != nil {
			sg.Group.DiscussionMode = *validatedMode
		}
		if validatedMaxTurns != nil {
			sg.Group.MaxTurns = *validatedMaxTurns
		}
		if req.Members != nil {
			existing := make(map[string]GroupMember, len(sg.Group.Members))
			for _, m := range sg.Group.Members {
				existing[m.Name] = m
			}
			nextMembers := make([]GroupMember, 0, len(validatedMembers))
			for _, name := range validatedMembers {
				if prev, ok := existing[name]; ok {
					nextMembers = append(nextMembers, GroupMember{
						Name:              prev.Name,
						LastSeenMessageID: prev.LastSeenMessageID,
						ConversationID:    prev.ConversationID,
					})
				} else {
					nextMembers = append(nextMembers, GroupMember{Name: name})
				}
			}
			sg.Group.Members = nextMembers
		}
		sg.Group.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	})
	if err != nil {
		return Group{}, err
	}

	if rt := s.getRuntime(id); rt != nil {
		rt.mu.Lock()
		if !rt.state.Running && !rt.state.Paused {
			rt.state.MaxTurns = updated.Group.MaxTurns
		}
		rt.mu.Unlock()
	}

	updated.Group.Members = s.hydrateMembers(updated.Group.Members)
	return updated.Group, nil
}

// Delete stops any active run, closes subscriber channels, and deletes the group from disk.
func (s *GroupService) Delete(id string, userID int) error {
	if _, err := s.store.Get(id); err != nil {
		return err
	}

	s.cancelRun(id, userID, "stopped", true)

	s.mu.Lock()
	rt := s.runtimes[id]
	delete(s.runtimes, id)
	s.mu.Unlock()

	if rt != nil {
		rt.mu.Lock()
		for ch := range rt.subs {
			delete(rt.subs, ch)
			close(ch)
		}
		rt.mu.Unlock()
	}

	if err := s.store.Delete(id); err != nil {
		return err
	}
	_ = os.RemoveAll(s.store.UploadsDir(id))
	return nil
}

// ClearMessages stops any active run, clears history, and resets per-member cursors & sessions.
func (s *GroupService) ClearMessages(id string, userID int) error {
	if _, err := s.store.Get(id); err != nil {
		return err
	}

	s.cancelRun(id, userID, "stopped", true)
	_ = os.RemoveAll(s.store.UploadsDir(id))

	updated, err := s.store.Update(id, func(sg *storedGroup) error {
		sg.Messages = []GroupMessage{}
		for i := range sg.Group.Members {
			sg.Group.Members[i].LastSeenMessageID = ""
			sg.Group.Members[i].ConversationID = ""
		}
		sg.Group.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	})
	if err != nil {
		return err
	}

	rt := s.runtimeFor(id, updated.Group.MaxTurns)
	rt.mu.Lock()
	rt.state = GroupRunState{
		MaxTurns: updated.Group.MaxTurns,
		Queue:    []string{},
	}
	snap := rt.snapshotLocked()
	rt.broadcastLocked(GroupSSEEvent{
		Name: "status",
		Data: map[string]any{
			"group_id":  id,
			"run_state": snap,
		},
	})
	rt.mu.Unlock()
	return nil
}

// SendMessage persists a user message, interrupts any active run, and kicks off sequential orchestration.
func (s *GroupService) SendMessage(id string, userID int, req SendGroupMessageRequest) (GroupMessage, GroupRunState, error) {
	trimmedText := strings.TrimSpace(req.Text)
	if trimmedText == "" && len(req.Images) == 0 && len(req.Files) == 0 {
		return GroupMessage{}, GroupRunState{}, fmt.Errorf("%w: text, images, or files are required", ErrGroupValidation)
	}

	replyToID := strings.TrimSpace(req.ReplyToID)
	msgID := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)

	attachments, attachErr := s.persistGroupUploads(id, msgID, now, req.Images, req.Files)
	if attachErr != nil {
		return GroupMessage{}, GroupRunState{}, attachErr
	}

	var (
		userMsg      GroupMessage
		repliedAgent string
		groupSnap    Group
	)

	updated, err := s.store.Update(id, func(sg *storedGroup) error {
		var preview *ReplyPreview
		if replyToID != "" {
			orig, ok := findMessageByID(sg.Messages, replyToID)
			if !ok {
				return fmt.Errorf("%w: reply_to_id not found in group", ErrGroupValidation)
			}
			preview = &ReplyPreview{
				ID:         orig.ID,
				SenderType: orig.SenderType,
				SenderName: orig.SenderName,
				Excerpt:    truncateRunes(collapseSpaces(orig.Content), maxReplyExcerptRunes),
			}
			if orig.SenderType == "agent" && hasMember(sg.Group.Members, orig.SenderName) {
				repliedAgent = orig.SenderName
			}
		}

		userMsg = GroupMessage{
			ID:           msgID,
			GroupID:      id,
			SenderType:   "user",
			SenderName:   "User",
			AgentColor:   "",
			Content:      req.Text,
			ReplyToID:    replyToID,
			ReplyPreview: preview,
			Quotes:       sanitizeQuotes(req.Quotes),
			Attachments:  attachments,
			CreatedAt:    now,
		}
		sg.Messages = append(sg.Messages, userMsg)
		sg.Group.UpdatedAt = now
		groupSnap = sg.Group
		return nil
	})
	if err != nil {
		if len(attachments) > 0 {
			_ = os.RemoveAll(filepath.Join(s.store.UploadsDir(id), msgID))
		}
		return GroupMessage{}, GroupRunState{}, err
	}
	_ = updated

	// Interrupt any active or paused run before starting the new one.
	s.cancelRun(id, userID, "interrupted", false)

	rt := s.runtimeFor(id, groupSnap.MaxTurns)
	rt.broadcast(GroupSSEEvent{
		Name: "message",
		Data: map[string]any{
			"group_id": id,
			"message":  userMsg,
		},
	})

	queue, addressed := buildInitialQueue(groupSnap, req.Text, repliedAgent)
	if len(queue) == 0 {
		rt.mu.Lock()
		rt.runGen++
		rt.initialMembers = nil
		rt.spokenInRun = nil
		rt.directlyAddressed = nil
		rt.state = GroupRunState{
			Running:  false,
			Paused:   false,
			MaxTurns: groupSnap.MaxTurns,
			Queue:    []string{},
		}
		snap := rt.snapshotLocked()
		rt.broadcastLocked(GroupSSEEvent{
			Name: "run_end",
			Data: map[string]any{
				"group_id":   id,
				"reason":     "completed",
				"turn_count": 0,
				"max_turns":  groupSnap.MaxTurns,
				"paused":     false,
				"run_state":  snap,
			},
		})
		rt.broadcastLocked(GroupSSEEvent{
			Name: "status",
			Data: map[string]any{
				"group_id":  id,
				"run_state": snap,
			},
		})
		rt.mu.Unlock()
		return userMsg, snap, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	initialSet := make(map[string]bool, len(queue))
	for _, m := range queue {
		initialSet[m] = true
	}

	rt.mu.Lock()
	rt.runGen++
	gen := rt.runGen
	rt.cancel = cancel
	rt.activeAgent = ""
	rt.activeRequestID = ""
	rt.triggerMsgID = userMsg.ID
	rt.triggerImages = append([]string(nil), req.Images...)
	rt.triggerFiles = append([]channel.FileUpload(nil), req.Files...)
	rt.initialMembers = initialSet
	rt.spokenInRun = make(map[string]bool, len(queue))
	rt.directlyAddressed = addressed
	rt.state = GroupRunState{
		Running:      true,
		Paused:       false,
		PauseReason:  "",
		CurrentAgent: "",
		TurnCount:    0,
		MaxTurns:     groupSnap.MaxTurns,
		Queue:        queue,
	}
	snap := rt.snapshotLocked()
	rt.broadcastLocked(GroupSSEEvent{
		Name: "status",
		Data: map[string]any{
			"group_id":  id,
			"run_state": snap,
		},
	})
	rt.mu.Unlock()

	go s.runLoop(ctx, id, userID, gen, rt)

	return userMsg, snap, nil
}

// Stop cancels an active or paused group run and sends a best-effort stop to the running agent.
func (s *GroupService) Stop(id string, userID int) (bool, GroupRunState, error) {
	sg, err := s.store.Get(id)
	if err != nil {
		return false, GroupRunState{}, err
	}

	wasActive, snap := s.cancelRun(id, userID, "stopped", true)
	if snap.MaxTurns == 0 {
		snap.MaxTurns = sg.Group.MaxTurns
	}
	return wasActive, snap, nil
}

// Continue resumes a run that paused at max_turns for extraTurns more turns.
func (s *GroupService) Continue(id string, userID int, extraTurns int) (GroupRunState, error) {
	sg, err := s.store.Get(id)
	if err != nil {
		return GroupRunState{}, err
	}

	if extraTurns == 0 {
		extraTurns = DefaultContinueExtraTurns
	} else if extraTurns < 1 {
		extraTurns = 1
	} else if extraTurns > MaxContinueExtraTurns {
		extraTurns = MaxContinueExtraTurns
	}

	rt := s.runtimeFor(id, sg.Group.MaxTurns)

	ctx, cancel := context.WithCancel(context.Background())

	rt.mu.Lock()
	if !rt.state.Paused || len(rt.state.Queue) == 0 {
		rt.mu.Unlock()
		cancel()
		return GroupRunState{}, ErrGroupNotPaused
	}
	rt.runGen++
	gen := rt.runGen
	rt.cancel = cancel
	rt.activeAgent = ""
	rt.activeRequestID = ""
	rt.state.MaxTurns += extraTurns
	rt.state.Paused = false
	rt.state.PauseReason = ""
	rt.state.Running = true
	snap := rt.snapshotLocked()
	rt.broadcastLocked(GroupSSEEvent{
		Name: "status",
		Data: map[string]any{
			"group_id":  id,
			"run_state": snap,
		},
	})
	rt.mu.Unlock()

	go s.runLoop(ctx, id, userID, gen, rt)

	return snap, nil
}

// Subscribe attaches a browser SSE subscriber to a group's live event hub.
func (s *GroupService) Subscribe(id string) (<-chan GroupSSEEvent, GroupRunState, func(), error) {
	sg, err := s.store.Get(id)
	if err != nil {
		return nil, GroupRunState{}, nil, err
	}

	rt := s.runtimeFor(id, sg.Group.MaxTurns)
	ch := make(chan GroupSSEEvent, groupSubBufferSize)

	rt.mu.Lock()
	rt.subs[ch] = struct{}{}
	initial := rt.snapshotLocked()
	if !initial.Running && !initial.Paused {
		initial.MaxTurns = sg.Group.MaxTurns
	}
	rt.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			rt.mu.Lock()
			if _, ok := rt.subs[ch]; ok {
				delete(rt.subs, ch)
				close(ch)
			}
			rt.mu.Unlock()
		})
	}

	return ch, initial, unsubscribe, nil
}

func (s *GroupService) cancelRun(id string, userID int, reason string, emitStatus bool) (bool, GroupRunState) {
	rt := s.getRuntime(id)
	if rt == nil {
		return false, GroupRunState{Queue: []string{}}
	}

	rt.mu.Lock()
	wasActive := rt.state.Running || rt.state.Paused
	prevCancel := rt.cancel
	activeAgent := rt.activeAgent
	activeReqID := rt.activeRequestID
	prevTurnCount := rt.state.TurnCount
	prevMaxTurns := rt.state.MaxTurns

	if wasActive {
		rt.runGen++
		rt.cancel = nil
		rt.activeAgent = ""
		rt.activeRequestID = ""
		rt.state.Running = false
		rt.state.Paused = false
		rt.state.PauseReason = ""
		rt.state.CurrentAgent = ""
		rt.state.Queue = []string{}
		rt.state.PendingApprovals = nil
	}
	snap := rt.snapshotLocked()
	if wasActive {
		rt.broadcastLocked(GroupSSEEvent{
			Name: "run_end",
			Data: map[string]any{
				"group_id":   id,
				"reason":     reason,
				"turn_count": prevTurnCount,
				"max_turns":  prevMaxTurns,
				"paused":     false,
				"run_state":  snap,
			},
		})
		if emitStatus {
			rt.broadcastLocked(GroupSSEEvent{
				Name: "status",
				Data: map[string]any{
					"group_id":  id,
					"run_state": snap,
				},
			})
		}
	}
	rt.mu.Unlock()

	if prevCancel != nil {
		prevCancel()
	}
	if activeAgent != "" && activeReqID != "" {
		s.stopAgentBestEffort(activeAgent, userID, activeReqID)
	}
	return wasActive, snap
}

func (s *GroupService) stopAgentBestEffort(agentName string, userID int, requestID string) {
	if s.agents == nil {
		return
	}
	baseURL, _, err := s.agents.BaseURL(agentName)
	if err != nil || baseURL == "" {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"user_id":    userID,
		"request_id": requestID,
	})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), groupStopHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/chat/stop", bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.stopClient.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func (s *GroupService) runLoop(ctx context.Context, groupID string, userID int, gen uint64, rt *groupRuntime) {
	completedTurns := 0
	offlineSkipped := 0
	hadStreamErr := false

	for {
		if ctx.Err() != nil {
			return
		}

		// Inspect queue & turn cap under lock.
		rt.mu.Lock()
		if rt.runGen != gen {
			rt.mu.Unlock()
			return
		}
		if len(rt.state.Queue) == 0 {
			reason := "completed"
			if completedTurns == 0 && offlineSkipped > 0 {
				reason = "no_online_members"
			} else if completedTurns == 0 && hadStreamErr {
				reason = "error"
			}
			rt.state.Running = false
			rt.state.Paused = false
			rt.state.PauseReason = ""
			rt.state.CurrentAgent = ""
			rt.activeAgent = ""
			rt.activeRequestID = ""
			rt.cancel = nil
			snap := rt.snapshotLocked()
			rt.broadcastLocked(GroupSSEEvent{
				Name: "run_end",
				Data: map[string]any{
					"group_id":   groupID,
					"reason":     reason,
					"turn_count": snap.TurnCount,
					"max_turns":  snap.MaxTurns,
					"paused":     false,
					"run_state":  snap,
				},
			})
			rt.broadcastLocked(GroupSSEEvent{
				Name: "status",
				Data: map[string]any{
					"group_id":  groupID,
					"run_state": snap,
				},
			})
			rt.mu.Unlock()
			return
		}

		if rt.state.TurnCount >= rt.state.MaxTurns {
			rt.state.Running = false
			rt.state.Paused = true
			rt.state.PauseReason = "max_turns"
			rt.state.CurrentAgent = ""
			rt.activeAgent = ""
			rt.activeRequestID = ""
			rt.cancel = nil
			snap := rt.snapshotLocked()
			rt.broadcastLocked(GroupSSEEvent{
				Name: "run_end",
				Data: map[string]any{
					"group_id":   groupID,
					"reason":     "max_turns",
					"turn_count": snap.TurnCount,
					"max_turns":  snap.MaxTurns,
					"paused":     true,
					"run_state":  snap,
				},
			})
			rt.broadcastLocked(GroupSSEEvent{
				Name: "status",
				Data: map[string]any{
					"group_id":  groupID,
					"run_state": snap,
				},
			})
			rt.mu.Unlock()
			return
		}

		nextAgent := rt.state.Queue[0]
		rt.state.Queue = append([]string{}, rt.state.Queue[1:]...)
		addressedDirectly := rt.directlyAddressed[nextAgent]
		delete(rt.directlyAddressed, nextAgent)
		triggerMsgID := rt.triggerMsgID
		triggerImages := rt.triggerImages
		triggerFiles := rt.triggerFiles
		rt.mu.Unlock()

		// Verify agent is initialized and reachable before consuming a turn.
		agentInfo, err := s.agents.Get(nextAgent)
		if err != nil || !agentInfo.Running || agentInfo.BaseURL == "" {
			offlineSkipped++
			rt.broadcast(GroupSSEEvent{
				Name: "error",
				Data: map[string]any{
					"group_id": groupID,
					"agent":    nextAgent,
					"error":    "agent is stopped or unreachable",
				},
			})
			continue
		}

		agentColor := agentdir.AgentColorOrDefault(agentInfo.AgentColor)

		// Load fresh group snapshot to build delta prompt.
		sg, err := s.store.Get(groupID)
		if err != nil {
			return
		}
		member, ok := findMember(sg.Group.Members, nextAgent)
		if !ok {
			// Member was removed from group while queued.
			continue
		}

		requestID := uuid.NewString()

		rt.mu.Lock()
		if rt.runGen != gen {
			rt.mu.Unlock()
			return
		}
		rt.state.TurnCount++
		turnNum := rt.state.TurnCount
		maxTurns := rt.state.MaxTurns
		rt.state.CurrentAgent = nextAgent
		rt.activeAgent = nextAgent
		rt.activeRequestID = requestID
		snap := rt.snapshotLocked()
		rt.broadcastLocked(GroupSSEEvent{
			Name: "turn_start",
			Data: map[string]any{
				"group_id":    groupID,
				"agent":       nextAgent,
				"agent_color": agentColor,
				"turn":        turnNum,
				"max_turns":   maxTurns,
				"request_id":  requestID,
			},
		})
		rt.broadcastLocked(GroupSSEEvent{
			Name: "status",
			Data: map[string]any{
				"group_id":  groupID,
				"run_state": snap,
			},
		})
		rt.mu.Unlock()

		prompt, images, files := buildDeltaPrompt(sg, member, triggerMsgID, triggerImages, triggerFiles, addressedDirectly)
		res, callErr := s.streamAgentTurn(ctx, groupID, nextAgent, agentColor, agentInfo.BaseURL, userID, requestID, member.ConversationID, prompt, images, files, rt, gen)

		// Self-healing 404 fallback if the agent's stored conversation was wiped.
		if errors.Is(callErr, errAgentConversationNotFound) && member.ConversationID != "" {
			_, _ = s.store.Update(groupID, func(cur *storedGroup) error {
				for i := range cur.Group.Members {
					if cur.Group.Members[i].Name == nextAgent {
						cur.Group.Members[i].ConversationID = ""
						cur.Group.Members[i].LastSeenMessageID = ""
					}
				}
				return nil
			})
			if freshSG, getErr := s.store.Get(groupID); getErr == nil {
				sg = freshSG
				member.ConversationID = ""
				member.LastSeenMessageID = ""
				requestID = uuid.NewString()
				rt.mu.Lock()
				if rt.runGen == gen {
					rt.activeRequestID = requestID
				}
				rt.mu.Unlock()
				prompt, images, files = buildDeltaPrompt(sg, member, triggerMsgID, triggerImages, triggerFiles, addressedDirectly)
				res, callErr = s.streamAgentTurn(ctx, groupID, nextAgent, agentColor, agentInfo.BaseURL, userID, requestID, "", prompt, images, files, rt, gen)
			}
		}

		// If cancelled or superseded by stop/interruption while streaming, exit quietly.
		if ctx.Err() != nil || !s.isCurrentGen(rt, gen) {
			return
		}

		if callErr != nil {
			if res.Unreachable {
				// Connection refused / dial failed before streaming: don't consume a turn.
				offlineSkipped++
				rt.mu.Lock()
				var snap GroupRunState
				if rt.runGen == gen {
					if rt.state.TurnCount > 0 {
						rt.state.TurnCount--
					}
					rt.state.CurrentAgent = ""
					rt.activeAgent = ""
					rt.activeRequestID = ""
					snap = rt.snapshotLocked()
				}
				rt.mu.Unlock()
				rt.broadcast(GroupSSEEvent{
					Name: "error",
					Data: map[string]any{
						"group_id": groupID,
						"agent":    nextAgent,
						"error":    "agent is stopped or unreachable",
					},
				})
				rt.broadcast(GroupSSEEvent{
					Name: "turn_end",
					Data: map[string]any{
						"group_id":    groupID,
						"agent":       nextAgent,
						"turn":        turnNum,
						"max_turns":   maxTurns,
						"message_id":  "",
						"stopped":     false,
						"passed":      false,
						"done_signal": false,
					},
				})
				rt.broadcast(GroupSSEEvent{
					Name: "status",
					Data: map[string]any{
						"group_id":  groupID,
						"run_state": snap,
					},
				})
				continue
			}

			hadStreamErr = true
			if !res.EmittedError {
				rt.broadcast(GroupSSEEvent{
					Name: "error",
					Data: map[string]any{
						"group_id": groupID,
						"agent":    nextAgent,
						"error":    callErr.Error(),
					},
				})
			}
			rt.broadcast(GroupSSEEvent{
				Name: "turn_end",
				Data: map[string]any{
					"group_id":    groupID,
					"agent":       nextAgent,
					"turn":        turnNum,
					"max_turns":   maxTurns,
					"message_id":  "",
					"stopped":     res.Stopped,
					"passed":      false,
					"done_signal": false,
				},
			})
			continue
		}

		completedTurns++
		rt.mu.Lock()
		if rt.runGen == gen {
			if rt.spokenInRun == nil {
				rt.spokenInRun = make(map[string]bool)
			}
			rt.spokenInRun[nextAgent] = true
		}
		rt.mu.Unlock()
		rawText := strings.TrimSpace(res.Text)

		// Detect [PASS] or empty output (when not stopped with partial text).
		if rawText == "" || strings.EqualFold(rawText, "[PASS]") {
			_, _ = s.store.Update(groupID, func(cur *storedGroup) error {
				latestID := latestMessageID(cur.Messages)
				for i := range cur.Group.Members {
					if cur.Group.Members[i].Name == nextAgent {
						if latestID != "" {
							cur.Group.Members[i].LastSeenMessageID = latestID
						}
						if res.ConversationID != "" {
							cur.Group.Members[i].ConversationID = res.ConversationID
						}
					}
				}
				return nil
			})
			rt.broadcast(GroupSSEEvent{
				Name: "turn_end",
				Data: map[string]any{
					"group_id":    groupID,
					"agent":       nextAgent,
					"turn":        turnNum,
					"max_turns":   maxTurns,
					"message_id":  "",
					"stopped":     res.Stopped,
					"passed":      true,
					"done_signal": false,
				},
			})
			continue
		}

		// Detect standalone [DONE] and strip it from persisted visible content.
		cleanedText, doneSignal := extractDoneSignal(rawText)
		if cleanedText == "" && doneSignal {
			_, _ = s.store.Update(groupID, func(cur *storedGroup) error {
				latestID := latestMessageID(cur.Messages)
				for i := range cur.Group.Members {
					if cur.Group.Members[i].Name == nextAgent {
						if latestID != "" {
							cur.Group.Members[i].LastSeenMessageID = latestID
						}
						if res.ConversationID != "" {
							cur.Group.Members[i].ConversationID = res.ConversationID
						}
					}
				}
				return nil
			})
			rt.broadcast(GroupSSEEvent{
				Name: "turn_end",
				Data: map[string]any{
					"group_id":    groupID,
					"agent":       nextAgent,
					"turn":        turnNum,
					"max_turns":   maxTurns,
					"message_id":  "",
					"stopped":     res.Stopped,
					"passed":      true,
					"done_signal": true,
				},
			})
			if sg.Group.DiscussionMode == DiscussionModeAutonomous {
				rt.mu.Lock()
				if rt.runGen == gen {
					rt.retainUnspokenInitialMembersLocked()
				}
				rt.mu.Unlock()
			}
			continue
		}

		timeline := finalizeTimelineText(res.Timeline, cleanedText)

		var savedMsg GroupMessage
		updatedSG, updateErr := s.store.Update(groupID, func(cur *storedGroup) error {
			now := time.Now().UTC().Format(time.RFC3339)
			msgID := uuid.NewString()
			var msgAttachments []GroupAttachment
			if len(res.Attachments) > 0 {
				msgAttachments = make([]GroupAttachment, len(res.Attachments))
				for i, att := range res.Attachments {
					att.MessageID = msgID
					att.CreatedAt = now
					msgAttachments[i] = att
				}
			} else {
				msgAttachments = collectAgentMessageAttachments(groupID, msgID, nextAgent, cleanedText, now)
			}
			savedMsg = GroupMessage{
				ID:          msgID,
				GroupID:     groupID,
				SenderType:  "agent",
				SenderName:  nextAgent,
				AgentColor:  agentColor,
				Content:     cleanedText,
				Timeline:    timeline,
				Attachments: msgAttachments,
				CreatedAt:   now,
			}
			cur.Messages = append(cur.Messages, savedMsg)
			cur.Group.UpdatedAt = now
			for i := range cur.Group.Members {
				if cur.Group.Members[i].Name == nextAgent {
					cur.Group.Members[i].LastSeenMessageID = savedMsg.ID
					if res.ConversationID != "" {
						cur.Group.Members[i].ConversationID = res.ConversationID
					}
				}
			}
			return nil
		})
		if updateErr != nil {
			continue
		}

		rt.broadcast(GroupSSEEvent{
			Name: "message",
			Data: map[string]any{
				"group_id": groupID,
				"message":  savedMsg,
			},
		})
		rt.broadcast(GroupSSEEvent{
			Name: "turn_end",
			Data: map[string]any{
				"group_id":    groupID,
				"agent":       nextAgent,
				"turn":        turnNum,
				"max_turns":   maxTurns,
				"message_id":  savedMsg.ID,
				"stopped":     res.Stopped,
				"passed":      false,
				"done_signal": doneSignal,
			},
		})

		// Autonomous handoff logic (only in autonomous mode when not stopped).
		if updatedSG.Group.DiscussionMode == DiscussionModeAutonomous && !res.Stopped {
			mentioned, hasAll := extractMentions(cleanedText, updatedSG.Group.Members)
			candidates := mentioned
			if hasAll {
				candidates = memberNames(updatedSG.Group.Members)
			}
			var handoffs []string
			for _, candidate := range candidates {
				if strings.EqualFold(candidate, nextAgent) {
					continue
				}
				handoffs = append(handoffs, candidate)
			}

			rt.mu.Lock()
			if rt.runGen == gen {
				if len(handoffs) > 0 {
					// Explicit @mention always takes precedence over [DONE].
					if rt.directlyAddressed == nil {
						rt.directlyAddressed = make(map[string]bool)
					}
					for _, candidate := range handoffs {
						rt.directlyAddressed[candidate] = true
						if !containsString(rt.state.Queue, candidate) {
							rt.state.Queue = append(rt.state.Queue, candidate)
						}
					}
				} else if doneSignal {
					// [DONE] stops follow-up handoffs but preserves unspoken initial-round members.
					rt.retainUnspokenInitialMembersLocked()
				}
			}
			rt.mu.Unlock()
		}
	}
}

func (rt *groupRuntime) retainUnspokenInitialMembersLocked() {
	if len(rt.state.Queue) == 0 {
		return
	}
	remaining := make([]string, 0, len(rt.state.Queue))
	for _, queued := range rt.state.Queue {
		if rt.initialMembers[queued] && !rt.spokenInRun[queued] {
			remaining = append(remaining, queued)
		}
	}
	rt.state.Queue = remaining
}

func (s *GroupService) isCurrentGen(rt *groupRuntime, gen uint64) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.runGen == gen
}

type agentTurnResult struct {
	Text           string
	ConversationID string
	Timeline       []api.TimelineItem
	Attachments    []GroupAttachment
	Stopped        bool
	Unreachable    bool
	EmittedError   bool
}

type agentChatStreamRequest struct {
	UserID         int                  `json:"user_id"`
	Text           string               `json:"text"`
	RequestID      string               `json:"request_id"`
	ConversationID string               `json:"conversation_id,omitempty"`
	Scope          string               `json:"scope,omitempty"`
	Images         []string             `json:"images,omitempty"`
	Files          []channel.FileUpload `json:"files,omitempty"`
}

func (s *GroupService) streamAgentTurn(
	ctx context.Context,
	groupID, agentName, agentColor, baseURL string,
	userID int,
	requestID, conversationID, prompt string,
	images []string,
	files []channel.FileUpload,
	rt *groupRuntime,
	gen uint64,
) (agentTurnResult, error) {
	var result agentTurnResult
	defer rt.clearRunApprovals(requestID)

	payload, err := json.Marshal(agentChatStreamRequest{
		UserID:         userID,
		Text:           prompt,
		RequestID:      requestID,
		ConversationID: conversationID,
		Scope:          "group:" + groupID,
		Images:         images,
		Files:          files,
	})
	if err != nil {
		return result, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/chat/stream", bytes.NewReader(payload))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		result.Unreachable = true
		return result, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound && conversationID != "" {
		_, _ = io.Copy(io.Discard, resp.Body)
		return result, errAgentConversationNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(body))
		var errBody struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errBody) == nil && errBody.Error != "" {
			msg = errBody.Error
		}
		if msg == "" {
			msg = fmt.Sprintf("agent returned HTTP %d", resp.StatusCode)
		}
		return result, errors.New(msg)
	}

	var (
		streamedText strings.Builder
		finalText    string
		doneReceived bool
		streamErr    error
	)

	appendTimelineText := func(kind, delta string) {
		if delta == "" {
			return
		}
		if n := len(result.Timeline); n > 0 && result.Timeline[n-1].Kind == kind {
			result.Timeline[n-1].Text += delta
			return
		}
		result.Timeline = append(result.Timeline, api.TimelineItem{Kind: kind, Text: delta})
	}

	dispatch := func(eventName, rawData string) {
		if !s.isCurrentGen(rt, gen) {
			return
		}
		rawData = strings.TrimSpace(rawData)
		if rawData == "" {
			return
		}
		switch eventName {
		case "thought":
			var ev struct {
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(rawData), &ev) == nil && ev.Text != "" {
				appendTimelineText("thought", ev.Text)
				rt.broadcast(GroupSSEEvent{
					Name: "thinking",
					Data: map[string]any{
						"group_id": groupID,
						"agent":    agentName,
						"text":     ev.Text,
					},
				})
			}
		case "chunk":
			var ev struct {
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(rawData), &ev) == nil && ev.Text != "" {
				streamedText.WriteString(ev.Text)
				appendTimelineText("text", ev.Text)
				rt.broadcast(GroupSSEEvent{
					Name: "chunk",
					Data: map[string]any{
						"group_id": groupID,
						"agent":    agentName,
						"text":     ev.Text,
					},
				})
			}
		case "tool":
			var ev struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			if json.Unmarshal([]byte(rawData), &ev) == nil {
				result.Timeline = append(result.Timeline, api.TimelineItem{
					Kind:      "tool",
					ID:        ev.ID,
					Name:      ev.Name,
					Arguments: ev.Arguments,
				})
				rt.broadcast(GroupSSEEvent{
					Name: "tool_start",
					Data: map[string]any{
						"group_id":  groupID,
						"agent":     agentName,
						"id":        ev.ID,
						"name":      ev.Name,
						"arguments": ev.Arguments,
					},
				})
			}
		case "tool_result":
			var ev struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Result  string `json:"result"`
				IsError bool   `json:"is_error"`
			}
			if json.Unmarshal([]byte(rawData), &ev) == nil {
				attached := false
				for i := len(result.Timeline) - 1; i >= 0; i-- {
					item := &result.Timeline[i]
					if item.Kind == "tool" && item.ID == ev.ID {
						item.Result = ev.Result
						item.IsError = ev.IsError
						attached = true
						break
					}
				}
				if !attached {
					result.Timeline = append(result.Timeline, api.TimelineItem{
						Kind:    "tool_result",
						ID:      ev.ID,
						Name:    ev.Name,
						Result:  ev.Result,
						IsError: ev.IsError,
					})
				}
				rt.broadcast(GroupSSEEvent{
					Name: "tool_end",
					Data: map[string]any{
						"group_id": groupID,
						"agent":    agentName,
						"id":       ev.ID,
						"name":     ev.Name,
						"result":   ev.Result,
						"is_error": ev.IsError,
					},
				})
			}
		case "conversation":
			var ev struct {
				ConversationID string `json:"conversation_id"`
			}
			if json.Unmarshal([]byte(rawData), &ev) == nil && ev.ConversationID != "" {
				result.ConversationID = ev.ConversationID
			}
		case "approval_required":
			var ev struct {
				ApprovalID  string `json:"approval_id"`
				ToolName    string `json:"tool_name"`
				Action      string `json:"action"`
				ArgsSummary string `json:"args_summary"`
				Args        string `json:"args"`
				SubAgent    string `json:"subagent"`
			}
			if json.Unmarshal([]byte(rawData), &ev) == nil && ev.ApprovalID != "" {
				pending := GroupPendingApproval{
					ApprovalID:     ev.ApprovalID,
					GroupID:        groupID,
					Agent:          agentName,
					AgentColor:     agentColor,
					ConversationID: result.ConversationID,
					RunID:          requestID,
					ToolName:       ev.ToolName,
					Action:         ev.Action,
					ArgsSummary:    ev.ArgsSummary,
					Args:           ev.Args,
					SubAgent:       ev.SubAgent,
				}
				rt.upsertApproval(pending)
				rt.broadcast(GroupSSEEvent{
					Name: "approval_required",
					Data: pending,
				})
			}
		case "approval_resolved":
			var ev struct {
				ApprovalID string `json:"approval_id"`
				Decision   string `json:"decision"`
				Reason     string `json:"reason"`
			}
			if json.Unmarshal([]byte(rawData), &ev) == nil && ev.ApprovalID != "" {
				rt.removeApproval(ev.ApprovalID)
				rt.broadcast(GroupSSEEvent{
					Name: "approval_resolved",
					Data: map[string]any{
						"group_id":    groupID,
						"agent":       agentName,
						"approval_id": ev.ApprovalID,
						"decision":    ev.Decision,
						"reason":      ev.Reason,
					},
				})
			}
		case "done":
			var ev struct {
				OK             bool   `json:"ok"`
				Text           string `json:"text"`
				ConversationID string `json:"conversation_id"`
				Stopped        bool   `json:"stopped"`
				Attachments    []struct {
					FileName    string `json:"file_name"`
					MimeType    string `json:"mime_type"`
					Path        string `json:"path"`
					Kind        string `json:"kind"`
					ProjectType string `json:"project_type"`
					Size        int64  `json:"size"`
				} `json:"attachments"`
			}
			if json.Unmarshal([]byte(rawData), &ev) == nil {
				doneReceived = true
				if ev.Text != "" {
					finalText = ev.Text
				}
				if ev.ConversationID != "" {
					result.ConversationID = ev.ConversationID
				}
				if ev.Stopped {
					result.Stopped = true
				}
				if len(ev.Attachments) > 0 {
					atts := make([]GroupAttachment, 0, len(ev.Attachments))
					for _, a := range ev.Attachments {
						if strings.TrimSpace(a.Path) == "" {
							continue
						}
						kind := strings.TrimSpace(a.Kind)
						if kind == "" {
							kind = attachmentKindFromMime(a.FileName, a.MimeType)
						}
						atts = append(atts, GroupAttachment{
							ID:          uuid.NewString(),
							GroupID:     groupID,
							SenderType:  "agent",
							SenderName:  agentName,
							Agent:       agentName,
							Path:        a.Path,
							ProjectType: a.ProjectType,
							FileName:    a.FileName,
							MimeType:    a.MimeType,
							Kind:        kind,
							Size:        a.Size,
						})
					}
					result.Attachments = atts
				}
			}
		case "error":
			var ev struct {
				Error string `json:"error"`
			}
			if json.Unmarshal([]byte(rawData), &ev) == nil && ev.Error != "" {
				streamErr = errors.New(ev.Error)
				result.EmittedError = true
				rt.broadcast(GroupSSEEvent{
					Name: "error",
					Data: map[string]any{
						"group_id": groupID,
						"agent":    agentName,
						"error":    ev.Error,
					},
				})
			}
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var (
		eventName string
		dataLines []string
	)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			if len(dataLines) > 0 {
				dispatch(eventName, strings.Join(dataLines, "\n"))
			}
			eventName = ""
			dataLines = nil
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
	if len(dataLines) > 0 {
		dispatch(eventName, strings.Join(dataLines, "\n"))
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return result, err
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}

	if doneReceived && finalText != "" {
		result.Text = finalText
	} else {
		result.Text = streamedText.String()
	}

	if streamErr != nil && strings.TrimSpace(result.Text) == "" {
		return result, streamErr
	}
	return result, nil
}

// buildDeltaPrompt constructs the exact context header + unseen transcript for one agent turn.
func buildDeltaPrompt(
	sg *storedGroup,
	member GroupMember,
	triggerMsgID string,
	triggerImages []string,
	triggerFiles []channel.FileUpload,
	addressedDirectly bool,
) (string, []string, []channel.FileUpload) {
	unseen := unseenMessages(sg.Messages, member.LastSeenMessageID)

	var sb strings.Builder
	memberTags := make([]string, 0, len(sg.Group.Members))
	for _, m := range sg.Group.Members {
		memberTags = append(memberTags, "@"+sanitizeTagText(m.Name))
	}
	membersList := strings.Join(memberTags, ", ")
	safeGroupName := sanitizeTagText(sg.Group.Name)
	safeAgentName := sanitizeTagText(member.Name)

	sb.WriteString("<group>\n")
	if member.ConversationID == "" {
		sb.WriteString("<group_context>\n")
		sb.WriteString("Group: " + safeGroupName + "\n")
		if desc := sanitizeTagText(sg.Group.Description); desc != "" {
			sb.WriteString("Topic: " + desc + "\n")
		}
		sb.WriteString("Members: " + membersList + "\n")
		sb.WriteString("Your identity: @" + safeAgentName + "\n")
		sb.WriteString("Discussion mode: " + sg.Group.DiscussionMode + "\n")
		sb.WriteString("Rules:\n")
		sb.WriteString("- You are participating in a shared multi-agent group chat. Keep responses concise and in your own persona.\n")
		sb.WriteString("- Never impersonate other agents or prefix your reply with \"[" + safeAgentName + "]:\".\n")
		sb.WriteString("- In autonomous mode, @mention a teammate (e.g. @alice) when handing off or asking for their input.\n")
		sb.WriteString("- Append [DONE] at the end of your reply ONLY if the topic is resolved and no other member needs to speak. Never append [DONE] when you @mention a teammate or when other members still need to answer.\n")
		sb.WriteString("- Reply with only [PASS] if you have nothing meaningful to add.\n")
		if addressedDirectly {
			sb.WriteString("- You were directly mentioned or addressed — respond directly and do NOT reply with [PASS].\n")
		}
		sb.WriteString("</group_context>\n")
	} else {
		sb.WriteString("<group_context>\n")
		sb.WriteString(fmt.Sprintf("Group: %s | You are: @%s | Members: %s | Mode: %s\n",
			safeGroupName, safeAgentName, membersList, sg.Group.DiscussionMode))
		sb.WriteString("Note: Never append [DONE] when you @mention a teammate or when other members still need to answer.\n")
		if addressedDirectly {
			sb.WriteString("Note: You were directly mentioned or addressed — respond directly and do NOT reply with [PASS].\n")
		}
		sb.WriteString("</group_context>\n")
	}

	var (
		attachImages []string
		attachFiles  []channel.FileUpload
	)

	sb.WriteString("<chats>\n")
	for i, msg := range unseen {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		if triggerMsgID != "" && msg.ID == triggerMsgID {
			attachImages = triggerImages
			attachFiles = triggerFiles
		}

		label := senderLabel(msg)
		if msg.ReplyToID != "" {
			origSender, origText := resolveReplyTarget(sg.Messages, msg)
			origText = closingGroupTag.ReplaceAllString(origText, `<\/$1>`)
			sb.WriteString(fmt.Sprintf("%s (replying to %s: %q)\n", label, origSender, origText))
		} else {
			sb.WriteString(label + "\n")
		}

		if len(msg.Quotes) > 0 {
			sb.WriteString(formatSelectionsBlock(msg.Quotes))
			sb.WriteString("\n\n")
		}
		sb.WriteString(closingGroupTag.ReplaceAllString(msg.Content, `<\/$1>`))
	}
	sb.WriteString("\n</chats>\n</group>")

	return sb.String(), attachImages, attachFiles
}

func unseenMessages(messages []GroupMessage, lastSeenID string) []GroupMessage {
	if len(messages) == 0 {
		return nil
	}
	if lastSeenID != "" {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].ID == lastSeenID {
				if i+1 < len(messages) {
					return messages[i+1:]
				}
				return messages[len(messages)-1:]
			}
		}
	}
	if len(messages) > defaultHistoryWindow {
		return messages[len(messages)-defaultHistoryWindow:]
	}
	return messages
}

func resolveReplyTarget(messages []GroupMessage, msg GroupMessage) (string, string) {
	if orig, ok := findMessageByID(messages, msg.ReplyToID); ok {
		sender := sanitizeTagText(orig.SenderName)
		if orig.SenderType == "user" {
			sender = "User"
		} else if orig.SenderType == "system" {
			sender = "System"
		}
		return sender, truncateRunes(strings.TrimSpace(orig.Content), maxReplyPromptRunes)
	}
	if msg.ReplyPreview != nil {
		sender := sanitizeTagText(msg.ReplyPreview.SenderName)
		if sender == "" {
			sender = msg.ReplyPreview.SenderType
		}
		return sender, msg.ReplyPreview.Excerpt
	}
	return "unknown", ""
}

func formatSelectionsBlock(quotes []AskQuote) string {
	var sb strings.Builder
	sb.WriteString(`<selections note="Excerpts the user selected from earlier messages. Treat them as quoted context for the request that follows.">`)
	for i, q := range quotes {
		from := q.Source
		if from == "tool" {
			from = "tool:" + escapeXMLAttr(q.Label)
		} else if from == "" {
			from = "answer"
		} else {
			from = escapeXMLAttr(from)
		}
		escaped := closingSelectionTag.ReplaceAllString(q.Text, `<\/$1>`)
		sb.WriteString(fmt.Sprintf("\n<selection n=\"%d\" from=\"%s\">\n%s\n</selection>", i+1, from, escaped))
	}
	sb.WriteString("\n</selections>")
	return sb.String()
}

func escapeXMLAttr(v string) string {
	return strings.TrimSpace(attrEscapePattern.ReplaceAllString(v, ""))
}

func senderLabel(msg GroupMessage) string {
	switch msg.SenderType {
	case "user":
		return "[User]"
	case "system":
		return "[System]"
	default:
		return "[" + sanitizeTagText(msg.SenderName) + "]"
	}
}

func sanitizeTagText(s string) string {
	s = strings.ReplaceAll(s, "<", "")
	s = strings.ReplaceAll(s, ">", "")
	return collapseSpaces(s)
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func extractMentions(text string, members []GroupMember) ([]string, bool) {
	stripped := fencedCodeBlockPattern.ReplaceAllString(text, " ")
	stripped = inlineCodePattern.ReplaceAllString(stripped, " ")

	lookup := make(map[string]string, len(members))
	for _, m := range members {
		lookup[strings.ToLower(m.Name)] = m.Name
	}

	matches := mentionPattern.FindAllStringSubmatch(stripped, -1)
	seen := make(map[string]bool)
	var mentioned []string
	hasAll := false
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		raw := strings.ToLower(match[1])
		if raw == "all" {
			hasAll = true
			continue
		}
		canonical, ok := lookup[raw]
		if !ok || seen[canonical] {
			continue
		}
		seen[canonical] = true
		mentioned = append(mentioned, canonical)
	}
	return mentioned, hasAll
}

func extractPlainNameMentions(text string, members []GroupMember) []string {
	stripped := fencedCodeBlockPattern.ReplaceAllString(text, " ")
	stripped = inlineCodePattern.ReplaceAllString(stripped, " ")

	type nameMatch struct {
		name      string
		pos       int
		rosterIdx int
	}
	var found []nameMatch
	for idx, m := range members {
		if m.Name == "" {
			continue
		}
		pat, err := regexp.Compile(`(?i)(?:^|[^A-Za-z0-9_-])(` + regexp.QuoteMeta(m.Name) + `)(?:$|[^A-Za-z0-9_-])`)
		if err != nil {
			continue
		}
		loc := pat.FindStringSubmatchIndex(stripped)
		if loc != nil && len(loc) >= 4 {
			found = append(found, nameMatch{name: m.Name, pos: loc[2], rosterIdx: idx})
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].pos != found[j].pos {
			return found[i].pos < found[j].pos
		}
		return found[i].rosterIdx < found[j].rosterIdx
	})
	out := make([]string, 0, len(found))
	for _, f := range found {
		out = append(out, f.name)
	}
	return out
}

func moveGroupToFront(items []string, front []string) []string {
	if len(front) == 0 {
		return items
	}
	seen := make(map[string]bool, len(front))
	out := make([]string, 0, len(items))
	for _, f := range front {
		if !seen[f] && containsString(items, f) {
			seen[f] = true
			out = append(out, f)
		}
	}
	for _, item := range items {
		if !seen[item] {
			out = append(out, item)
		}
	}
	return out
}

func buildInitialQueue(group Group, text string, repliedAgent string) ([]string, map[string]bool) {
	mentioned, hasAll := extractMentions(text, group.Members)
	if hasAll {
		return memberNames(group.Members), make(map[string]bool)
	}
	if len(mentioned) > 0 {
		addressed := make(map[string]bool, len(mentioned))
		for _, m := range mentioned {
			addressed[m] = true
		}
		return mentioned, addressed
	}

	addressed := make(map[string]bool)
	switch group.DiscussionMode {
	case DiscussionModeMention:
		if repliedAgent != "" {
			addressed[repliedAgent] = true
			return []string{repliedAgent}, addressed
		}
		return nil, addressed
	case DiscussionModeAll:
		names := memberNames(group.Members)
		if repliedAgent != "" {
			addressed[repliedAgent] = true
			return moveToFront(names, repliedAgent), addressed
		}
		if plain := extractPlainNameMentions(text, group.Members); len(plain) > 0 {
			for _, m := range plain {
				addressed[m] = true
			}
			return moveGroupToFront(names, plain), addressed
		}
		return names, addressed
	default: // autonomous
		if repliedAgent != "" {
			addressed[repliedAgent] = true
			return []string{repliedAgent}, addressed
		}
		names := memberNames(group.Members)
		if plain := extractPlainNameMentions(text, group.Members); len(plain) > 0 {
			for _, m := range plain {
				addressed[m] = true
			}
			return moveGroupToFront(names, plain), addressed
		}
		return names, addressed
	}
}

func extractDoneSignal(text string) (string, bool) {
	stripped := fencedCodeBlockPattern.ReplaceAllString(text, " ")
	stripped = inlineCodePattern.ReplaceAllString(stripped, " ")
	if !doneTokenPattern.MatchString(stripped) {
		return text, false
	}

	// Remove standalone [DONE] occurrences (including trailing punctuation) outside
	// code blocks while preserving paragraphs and backtick-quoted `[DONE]`.
	lines := strings.Split(text, "\n")
	outLines := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			outLines = append(outLines, line)
			continue
		}
		if inFence {
			outLines = append(outLines, line)
			continue
		}
		cleaned := doneStripPattern.ReplaceAllStringFunc(line, func(match string) string {
			startsSpace := len(match) > 0 && (match[0] == ' ' || match[0] == '\t')
			endsSpace := len(match) > 0 && (match[len(match)-1] == ' ' || match[len(match)-1] == '\t')
			if startsSpace && endsSpace {
				return " "
			}
			return ""
		})
		cleaned = strings.TrimRight(cleaned, " \t")
		if trimmed != "" && strings.TrimSpace(cleaned) == "" {
			continue
		}
		outLines = append(outLines, cleaned)
	}
	return strings.TrimSpace(strings.Join(outLines, "\n")), true
}

func finalizeTimelineText(items []api.TimelineItem, finalText string) []api.TimelineItem {
	if finalText == "" {
		return items
	}
	lastText, lastTool := -1, -1
	for i := len(items) - 1; i >= 0; i-- {
		if lastText == -1 && items[i].Kind == "text" {
			lastText = i
		}
		if lastTool == -1 && items[i].Kind == "tool" {
			lastTool = i
		}
		if lastText != -1 && lastTool != -1 {
			break
		}
	}
	if lastText != -1 && lastText > lastTool {
		items[lastText].Text = finalText
		return items
	}
	return append(items, api.TimelineItem{Kind: "text", Text: finalText})
}

func (s *GroupService) hydrateMembers(members []GroupMember) []GroupMember {
	out := make([]GroupMember, len(members))
	for i, m := range members {
		hydrated := GroupMember{
			Name:              m.Name,
			AgentColor:        agentdir.DefaultAgentColor,
			Running:           false,
			LastSeenMessageID: m.LastSeenMessageID,
			ConversationID:    m.ConversationID,
		}
		if s.agents != nil {
			if info, err := s.agents.Get(m.Name); err == nil {
				hydrated.AgentColor = agentdir.AgentColorOrDefault(info.AgentColor)
				hydrated.Running = info.Running
			}
		}
		out[i] = hydrated
	}
	return out
}

func (s *GroupService) validateMemberNames(raw []string) ([]string, error) {
	seen := make(map[string]bool, len(raw))
	names := make([]string, 0, len(raw))
	for _, entry := range raw {
		name := strings.TrimSpace(entry)
		if name == "" {
			continue
		}
		if seen[name] {
			continue
		}
		if s.agents == nil {
			return nil, fmt.Errorf("%w: agent directory unavailable", ErrGroupValidation)
		}
		if _, err := s.agents.Get(name); err != nil {
			return nil, fmt.Errorf("%w: unknown agent %q", ErrGroupValidation, name)
		}
		seen[name] = true
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%w: at least one valid member is required", ErrGroupValidation)
	}
	return names, nil
}

func validateGroupName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	count := utf8.RuneCountInString(name)
	if count < 1 || count > maxGroupNameRunes {
		return "", fmt.Errorf("%w: name must be 1-%d characters", ErrGroupValidation, maxGroupNameRunes)
	}
	return name, nil
}

func validateGroupDescription(raw string) (string, error) {
	desc := strings.TrimSpace(raw)
	if utf8.RuneCountInString(desc) > maxGroupDescriptionRunes {
		return "", fmt.Errorf("%w: description must be at most %d characters", ErrGroupValidation, maxGroupDescriptionRunes)
	}
	return desc, nil
}

func validateDiscussionMode(raw string, allowEmpty bool) (string, error) {
	mode := strings.TrimSpace(raw)
	if mode == "" && allowEmpty {
		return DiscussionModeAutonomous, nil
	}
	switch mode {
	case DiscussionModeAutonomous, DiscussionModeAll, DiscussionModeMention:
		return mode, nil
	default:
		return "", fmt.Errorf("%w: discussion_mode must be autonomous, all, or mention", ErrGroupValidation)
	}
}

func validateMaxTurns(val int, allowZeroDefault bool) (int, error) {
	if val == 0 && allowZeroDefault {
		return DefaultGroupMaxTurns, nil
	}
	if val < 1 || val > MaxGroupMaxTurns {
		return 0, fmt.Errorf("%w: max_turns must be between 1 and %d", ErrGroupValidation, MaxGroupMaxTurns)
	}
	return val, nil
}

func sanitizeQuotes(quotes []AskQuote) []AskQuote {
	if len(quotes) == 0 {
		return nil
	}
	out := make([]AskQuote, 0, len(quotes))
	for _, q := range quotes {
		if strings.TrimSpace(q.Text) == "" {
			continue
		}
		if q.Source == "" {
			q.Source = "answer"
		}
		out = append(out, q)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func findMessageByID(messages []GroupMessage, id string) (GroupMessage, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].ID == id {
			return messages[i], true
		}
	}
	return GroupMessage{}, false
}

func latestMessageID(messages []GroupMessage) string {
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].ID
}

func findMember(members []GroupMember, name string) (GroupMember, bool) {
	for _, m := range members {
		if m.Name == name {
			return m, true
		}
	}
	return GroupMember{}, false
}

func hasMember(members []GroupMember, name string) bool {
	_, ok := findMember(members, name)
	return ok
}

func memberNames(members []GroupMember) []string {
	names := make([]string, 0, len(members))
	for _, m := range members {
		names = append(names, m.Name)
	}
	return names
}

func moveToFront(items []string, target string) []string {
	idx := -1
	for i, v := range items {
		if v == target {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return items
	}
	out := make([]string, 0, len(items))
	out = append(out, items[idx])
	out = append(out, items[:idx]...)
	out = append(out, items[idx+1:]...)
	return out
}

func containsString(items []string, target string) bool {
	for _, v := range items {
		if v == target {
			return true
		}
	}
	return false
}

// ListAttachments returns paginated uploaded images and files for a group, newest first.
func (s *GroupService) ListAttachments(id, kind string, limit, offset int) (GroupAttachmentsPage, error) {
	return s.store.ListAttachments(id, kind, limit, offset)
}

// AttachmentFile resolves one persisted group upload to its safe on-disk path.
func (s *GroupService) AttachmentFile(groupID, messageID, storedName string) (string, string, string, error) {
	if err := ValidateGroupID(groupID); err != nil {
		return "", "", "", err
	}
	if !isSafePathSegment(messageID) || !isSafePathSegment(storedName) {
		return "", "", "", ErrGroupNotFound
	}
	att, err := s.store.FindAttachment(groupID, messageID, storedName)
	if err != nil {
		return "", "", "", err
	}
	baseDir := s.store.UploadsDir(groupID)
	absPath := filepath.Join(baseDir, messageID, att.StoredName)
	if !strings.HasPrefix(absPath, filepath.Clean(baseDir)+string(os.PathSeparator)) {
		return "", "", "", ErrGroupNotFound
	}
	if _, err := os.Stat(absPath); err != nil {
		return "", "", "", ErrGroupNotFound
	}
	return absPath, att.FileName, att.MimeType, nil
}

func (s *GroupService) persistGroupUploads(
	groupID, messageID, createdAt string,
	images []string,
	files []channel.FileUpload,
) ([]GroupAttachment, error) {
	if len(images) == 0 && len(files) == 0 {
		return nil, nil
	}

	msgDir := filepath.Join(s.store.UploadsDir(groupID), messageID)
	if err := os.MkdirAll(msgDir, 0o700); err != nil {
		return nil, fmt.Errorf("create group uploads dir: %w", err)
	}

	usedNames := map[string]int{}
	uniqueStoredName := func(base string) string {
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		if stem == "" {
			stem = "file"
		}
		candidate := stem + ext
		key := strings.ToLower(candidate)
		if count, exists := usedNames[key]; exists {
			count++
			usedNames[key] = count
			candidate = fmt.Sprintf("%s-%d%s", stem, count, ext)
		} else {
			usedNames[key] = 1
		}
		return candidate
	}

	attachments := make([]GroupAttachment, 0, len(images)+len(files))

	for i, raw := range images {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		// Accept both raw base64 and data URLs ("data:image/png;base64,...").
		if idx := strings.Index(trimmed, ";base64,"); idx >= 0 && strings.HasPrefix(trimmed, "data:") {
			trimmed = trimmed[idx+len(";base64,"):]
		}
		data, err := base64.StdEncoding.DecodeString(trimmed)
		if err != nil || len(data) == 0 {
			continue
		}
		mimeType := http.DetectContentType(data)
		if !strings.HasPrefix(mimeType, "image/") {
			mimeType = "image/png"
		}
		ext := imageExtensionForMime(mimeType)
		fileName := fmt.Sprintf("image-%d%s", i+1, ext)
		storedName := uniqueStoredName(fileName)
		if err := os.WriteFile(filepath.Join(msgDir, storedName), data, 0o600); err != nil {
			_ = os.RemoveAll(msgDir)
			return nil, fmt.Errorf("write group image: %w", err)
		}
		attachments = append(attachments, GroupAttachment{
			ID:         uuid.NewString(),
			GroupID:    groupID,
			MessageID:  messageID,
			SenderType: "user",
			SenderName: "User",
			FileName:   fileName,
			StoredName: storedName,
			MimeType:   mimeType,
			Kind:       "image",
			Size:       int64(len(data)),
			CreatedAt:  createdAt,
		})
	}

	for _, f := range files {
		trimmed := strings.TrimSpace(f.DataBase64)
		if trimmed == "" {
			continue
		}
		if idx := strings.Index(trimmed, ";base64,"); idx >= 0 && strings.HasPrefix(trimmed, "data:") {
			trimmed = trimmed[idx+len(";base64,"):]
		}
		data, err := base64.StdEncoding.DecodeString(trimmed)
		if err != nil || len(data) == 0 {
			continue
		}
		cleanName := sanitizeUploadFileName(f.Name)
		if cleanName == "" {
			cleanName = "attachment"
		}
		storedName := uniqueStoredName(cleanName)
		if err := os.WriteFile(filepath.Join(msgDir, storedName), data, 0o600); err != nil {
			_ = os.RemoveAll(msgDir)
			return nil, fmt.Errorf("write group file: %w", err)
		}
		mimeType := detectUploadMime(cleanName, data)
		attachments = append(attachments, GroupAttachment{
			ID:         uuid.NewString(),
			GroupID:    groupID,
			MessageID:  messageID,
			SenderType: "user",
			SenderName: "User",
			FileName:   cleanName,
			StoredName: storedName,
			MimeType:   mimeType,
			Kind:       attachmentKindFromMime(cleanName, mimeType),
			Size:       int64(len(data)),
			CreatedAt:  createdAt,
		})
	}

	if len(attachments) == 0 {
		_ = os.RemoveAll(msgDir)
		return nil, nil
	}
	return attachments, nil
}

func isSafePathSegment(v string) bool {
	if v == "" || v == "." || v == ".." {
		return false
	}
	return !strings.ContainsAny(v, `/\`) && !strings.Contains(v, "..")
}

func sanitizeUploadFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if name == "." || name == ".." || name == "/" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func imageExtensionForMime(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func detectUploadMime(fileName string, data []byte) string {
	if ext := strings.ToLower(filepath.Ext(fileName)); ext != "" {
		if byExt := mime.TypeByExtension(ext); byExt != "" {
			return byExt
		}
		switch ext {
		case ".md", ".markdown":
			return "text/markdown; charset=utf-8"
		case ".yaml", ".yml":
			return "text/yaml; charset=utf-8"
		case ".log":
			return "text/plain; charset=utf-8"
		}
	}
	return http.DetectContentType(data)
}

func attachmentKindFromMime(fileName, mimeType string) string {
	lowerMime := strings.ToLower(mimeType)
	lowerExt := strings.ToLower(filepath.Ext(fileName))
	switch {
	case strings.HasPrefix(lowerMime, "image/"):
		return "image"
	case strings.HasPrefix(lowerMime, "video/"):
		return "video"
	case strings.HasPrefix(lowerMime, "audio/"):
		return "audio"
	case strings.HasPrefix(lowerMime, "application/pdf") || lowerExt == ".pdf":
		return "pdf"
	default:
		return "file"
	}
}

// collectAgentMessageAttachments uses the shared internal/attachment package
// to resolve @attach(...) directives and previewable media/project paths inside
// an agent's own sandboxed roots.
func collectAgentMessageAttachments(groupID, messageID, agentName, text, createdAt string) []GroupAttachment {
	if agentName == "" || strings.TrimSpace(text) == "" {
		return nil
	}
	home, err := agentdir.Home(agentName)
	if err != nil {
		return nil
	}
	user, _ := agentdir.UserDir(agentName)
	skills, _ := agentdir.SkillsDir(agentName)
	jobs, _ := agentdir.JobsDir(agentName)
	items := attachment.Collect(text, attachment.Roots{
		Home:   home,
		User:   user,
		Skills: skills,
		Jobs:   jobs,
	})
	if len(items) == 0 {
		return nil
	}
	out := make([]GroupAttachment, 0, len(items))
	for _, item := range items {
		out = append(out, GroupAttachment{
			ID:          uuid.NewString(),
			GroupID:     groupID,
			MessageID:   messageID,
			SenderType:  "agent",
			SenderName:  agentName,
			Agent:       agentName,
			Path:        item.Virtual,
			ProjectType: item.ProjectType,
			FileName:    item.Name,
			MimeType:    item.Mime,
			Kind:        string(item.Kind),
			Size:        item.Size,
			CreatedAt:   createdAt,
		})
	}
	return out
}
