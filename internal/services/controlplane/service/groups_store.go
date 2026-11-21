package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	"myaaw/internal/channel/api"
)

var (
	ErrGroupNotFound   = errors.New("group not found")
	ErrInvalidGroupID  = errors.New("invalid group id")
	ErrGroupValidation = errors.New("invalid group request")
	ErrGroupNotPaused  = errors.New("group run is not paused")
)

const (
	DiscussionModeAutonomous  = "autonomous"
	DiscussionModeAll         = "all"
	DiscussionModeMention     = "mention"
	DefaultGroupMaxTurns      = 8
	MaxGroupMaxTurns          = 50
	DefaultContinueExtraTurns = 5
	MaxContinueExtraTurns     = 20
)

var groupIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidateGroupID prevents path traversal by restricting group IDs to a safe
// filesystem filename subset.
func ValidateGroupID(id string) error {
	if !groupIDPattern.MatchString(id) {
		return ErrInvalidGroupID
	}
	return nil
}

// GroupMember represents one agent participating in a group.
// Name, LastSeenMessageID, and ConversationID are persisted; AgentColor and
// Running are hydrated dynamically from AgentService on every read.
type GroupMember struct {
	Name              string `json:"name"`
	AgentColor        string `json:"agent_color"`
	Running           bool   `json:"running"`
	LastSeenMessageID string `json:"last_seen_message_id"`
	ConversationID    string `json:"conversation_id"`
}

// ReplyPreview is the compact metadata rendered on a WhatsApp-style full-message reply.
type ReplyPreview struct {
	ID         string `json:"id"`
	SenderType string `json:"sender_type"`
	SenderName string `json:"sender_name"`
	Excerpt    string `json:"excerpt"`
}

// AskQuote matches webui/src/lib/types/chat.ts verbatim so AskQuoteList works seamlessly.
type AskQuote struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	Source      string `json:"source"`
	Label       string `json:"label,omitempty"`
	SourceID    string `json:"sourceId,omitempty"`
	StartOffset *int   `json:"startOffset,omitempty"`
	EndOffset   *int   `json:"endOffset,omitempty"`
}

// GroupMessage is one persisted turn in a group transcript.
type GroupMessage struct {
	ID           string             `json:"id"`
	GroupID      string             `json:"group_id"`
	SenderType   string             `json:"sender_type"`
	SenderName   string             `json:"sender_name"`
	AgentColor   string             `json:"agent_color"`
	Content      string             `json:"content"`
	Timeline     []api.TimelineItem `json:"timeline,omitempty"`
	ReplyToID    string             `json:"reply_to_id,omitempty"`
	ReplyPreview *ReplyPreview      `json:"reply_preview,omitempty"`
	Quotes       []AskQuote         `json:"quotes,omitempty"`
	CreatedAt    string             `json:"created_at"`
}

// GroupRunState describes the live orchestration state of a group.
type GroupRunState struct {
	Running      bool     `json:"running"`
	Paused       bool     `json:"paused"`
	PauseReason  string   `json:"pause_reason"`
	CurrentAgent string   `json:"current_agent"`
	TurnCount    int      `json:"turn_count"`
	MaxTurns     int      `json:"max_turns"`
	Queue        []string `json:"queue"`
}

// Group holds a group's configuration and member roster.
type Group struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	Members        []GroupMember `json:"members"`
	DiscussionMode string        `json:"discussion_mode"`
	MaxTurns       int           `json:"max_turns"`
	CreatedAt      string        `json:"created_at"`
	UpdatedAt      string        `json:"updated_at"`
}

// GroupLastMessage is the preview of the latest message returned in GET /groups.
type GroupLastMessage struct {
	ID         string `json:"id"`
	SenderType string `json:"sender_type"`
	SenderName string `json:"sender_name"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

// GroupSummary is returned in GET /groups.
type GroupSummary struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Members        []GroupMember     `json:"members"`
	MemberCount    int               `json:"member_count"`
	DiscussionMode string            `json:"discussion_mode"`
	MaxTurns       int               `json:"max_turns"`
	Running        bool              `json:"running"`
	CurrentAgent   string            `json:"current_agent"`
	LastMessage    *GroupLastMessage `json:"last_message,omitempty"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
}

// GroupDetail is returned by GET /groups/:id.
type GroupDetail struct {
	Group    Group          `json:"group"`
	Messages []GroupMessage `json:"messages"`
	RunState GroupRunState  `json:"run_state"`
}

// CreateGroupRequest is the payload for POST /groups.
type CreateGroupRequest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Members        []string `json:"members"`
	DiscussionMode string   `json:"discussion_mode"`
	MaxTurns       int      `json:"max_turns"`
}

// UpdateGroupRequest is the payload for PATCH /groups/:id.
type UpdateGroupRequest struct {
	Name           *string   `json:"name"`
	Description    *string   `json:"description"`
	Members        *[]string `json:"members"`
	DiscussionMode *string   `json:"discussion_mode"`
	MaxTurns       *int      `json:"max_turns"`
}

// SendGroupMessageRequest is the payload for POST /groups/:id/messages.
type SendGroupMessageRequest struct {
	Text      string               `json:"text"`
	ReplyToID string               `json:"reply_to_id,omitempty"`
	Quotes    []AskQuote           `json:"quotes,omitempty"`
	Images    []string             `json:"images,omitempty"`
	Files     []channel.FileUpload `json:"files,omitempty"`
}

// ContinueGroupRequest is the payload for POST /groups/:id/continue.
type ContinueGroupRequest struct {
	ExtraTurns int `json:"extra_turns"`
}

// GroupSSEEvent is one Server-Sent Event broadcast on GET /groups/:id/stream.
type GroupSSEEvent struct {
	Name string
	Data any
}

type storedGroup struct {
	Group    Group          `json:"group"`
	Messages []GroupMessage `json:"messages"`
}

// GroupStore persists one atomic JSON file per group under ~/.myaaw/groups/.
type GroupStore struct {
	dir string
	mu  sync.RWMutex
}

// NewGroupStore resolves ~/.myaaw/groups/ and returns a GroupStore rooted there.
func NewGroupStore() (*GroupStore, error) {
	root, err := agentdir.MyaawRoot()
	if err != nil {
		return nil, err
	}
	return NewGroupStoreAt(filepath.Join(root, "groups")), nil
}

// NewGroupStoreAt returns a GroupStore rooted at dir (useful in tests).
func NewGroupStoreAt(dir string) *GroupStore {
	return &GroupStore{dir: dir}
}

func (s *GroupStore) filePath(id string) (string, error) {
	if err := ValidateGroupID(id); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, id+".json"), nil
}

// List returns all persisted groups sorted by UpdatedAt descending.
func (s *GroupStore) List() ([]storedGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []storedGroup{}, nil
		}
		return nil, err
	}

	result := make([]storedGroup, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if ValidateGroupID(id) != nil {
			continue
		}
		sg, err := s.readFileLocked(filepath.Join(s.dir, name))
		if err != nil {
			continue
		}
		result = append(result, *sg)
	}

	sort.SliceStable(result, func(i, j int) bool {
		ti, ei := time.Parse(time.RFC3339Nano, result[i].Group.UpdatedAt)
		tj, ej := time.Parse(time.RFC3339Nano, result[j].Group.UpdatedAt)
		if ei == nil && ej == nil && !ti.Equal(tj) {
			return ti.After(tj)
		}
		if result[i].Group.UpdatedAt != result[j].Group.UpdatedAt {
			return result[i].Group.UpdatedAt > result[j].Group.UpdatedAt
		}
		return result[i].Group.ID < result[j].Group.ID
	})

	return result, nil
}

// Get reads one stored group by ID.
func (s *GroupStore) Get(id string) (*storedGroup, error) {
	path, err := s.filePath(id)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.readFileLocked(path)
}

// Create persists a new group file atomically.
func (s *GroupStore) Create(sg *storedGroup) error {
	path, err := s.filePath(sg.Group.ID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.writeFileLocked(path, sg)
}

// Update runs fn under the store write lock and atomically saves the modified group.
func (s *GroupStore) Update(id string, fn func(sg *storedGroup) error) (*storedGroup, error) {
	path, err := s.filePath(id)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sg, err := s.readFileLocked(path)
	if err != nil {
		return nil, err
	}
	if err := fn(sg); err != nil {
		return nil, err
	}
	if err := s.writeFileLocked(path, sg); err != nil {
		return nil, err
	}
	return cloneStoredGroup(sg), nil
}

// Delete removes the group's JSON file from disk.
func (s *GroupStore) Delete(id string) error {
	path, err := s.filePath(id)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrGroupNotFound
		}
		return err
	}
	return nil
}

func (s *GroupStore) readFileLocked(path string) (*storedGroup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrGroupNotFound
		}
		return nil, err
	}

	var sg storedGroup
	if err := json.Unmarshal(data, &sg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	normalizeStoredGroup(&sg)
	return &sg, nil
}

func (s *GroupStore) writeFileLocked(path string, sg *storedGroup) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	normalizeStoredGroup(sg)

	data, err := json.MarshalIndent(sg, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(s.dir, "group-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func normalizeStoredGroup(sg *storedGroup) {
	if sg.Group.Members == nil {
		sg.Group.Members = []GroupMember{}
	}
	if sg.Messages == nil {
		sg.Messages = []GroupMessage{}
	}
	if sg.Group.DiscussionMode == "" {
		sg.Group.DiscussionMode = DiscussionModeAutonomous
	}
	if sg.Group.MaxTurns <= 0 {
		sg.Group.MaxTurns = DefaultGroupMaxTurns
	}
}

func cloneStoredGroup(sg *storedGroup) *storedGroup {
	if sg == nil {
		return nil
	}
	out := &storedGroup{
		Group:    sg.Group,
		Messages: make([]GroupMessage, len(sg.Messages)),
	}
	out.Group.Members = make([]GroupMember, len(sg.Group.Members))
	copy(out.Group.Members, sg.Group.Members)
	for i, msg := range sg.Messages {
		m := msg
		if len(msg.Timeline) > 0 {
			m.Timeline = make([]api.TimelineItem, len(msg.Timeline))
			copy(m.Timeline, msg.Timeline)
		}
		if msg.ReplyPreview != nil {
			rp := *msg.ReplyPreview
			m.ReplyPreview = &rp
		}
		if len(msg.Quotes) > 0 {
			m.Quotes = make([]AskQuote, len(msg.Quotes))
			copy(m.Quotes, msg.Quotes)
		}
		out.Messages[i] = m
	}
	return out
}
