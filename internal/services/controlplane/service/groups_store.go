package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"myaaw/internal/channel"
	"myaaw/internal/channel/api"
	"myaaw/internal/provider"
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
	MaxGroupMessagePageLimit  = 500
)

var groupIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidateGroupID restricts group IDs to a safe identifier subset.
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

// GroupAttachment describes one uploaded file or agent-delivered @attach(...)
// artifact persisted for a group message.
type GroupAttachment struct {
	ID          string `json:"id"`
	GroupID     string `json:"group_id"`
	MessageID   string `json:"message_id"`
	SenderType  string `json:"sender_type,omitempty"`
	SenderName  string `json:"sender_name,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Path        string `json:"path,omitempty"`
	ProjectType string `json:"project_type,omitempty"`
	FileName    string `json:"file_name"`
	StoredName  string `json:"stored_name,omitempty"`
	MimeType    string `json:"mime_type,omitempty"`
	Kind        string `json:"kind"` // "image" | "video" | "audio" | "pdf" | "file" | "project"
	Size        int64  `json:"size,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// GroupAttachmentsPage is returned by GET /groups/:id/attachments.
type GroupAttachmentsPage struct {
	Items   []GroupAttachment `json:"items"`
	Total   int               `json:"total"`
	Limit   int               `json:"limit"`
	Offset  int               `json:"offset"`
	HasMore bool              `json:"has_more"`
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
	Quotes        []AskQuote              `json:"quotes,omitempty"`
	Attachments   []GroupAttachment       `json:"attachments,omitempty"`
	ExtraMessages []provider.ExtraMessage `json:"extra_messages,omitempty"`
	CreatedAt     string                  `json:"created_at"`
}

// GroupPendingApproval describes one tool call waiting for user approval inside a group turn.
type GroupPendingApproval struct {
	ApprovalID     string `json:"approval_id"`
	GroupID        string `json:"group_id"`
	Agent          string `json:"agent"`
	AgentColor     string `json:"agent_color,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	ToolName       string `json:"tool_name"`
	Action         string `json:"action,omitempty"`
	ArgsSummary    string `json:"args_summary,omitempty"`
	Args           string `json:"args,omitempty"`
	SubAgent       string `json:"subagent,omitempty"`
}

// GroupRunState describes the live orchestration state of a group.
type GroupRunState struct {
	Running          bool                   `json:"running"`
	Paused           bool                   `json:"paused"`
	PauseReason      string                 `json:"pause_reason"`
	CurrentAgent     string                 `json:"current_agent"`
	TurnCount        int                    `json:"turn_count"`
	MaxTurns         int                    `json:"max_turns"`
	Queue            []string               `json:"queue"`
	PendingApprovals []GroupPendingApproval `json:"pending_approvals,omitempty"`
}

// Group holds a group's configuration and member roster.
type Group struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	Description       string        `json:"description"`
	Members           []GroupMember `json:"members"`
	DiscussionMode    string        `json:"discussion_mode"`
	MaxTurns          int           `json:"max_turns"`
	HideThinking      bool          `json:"hide_thinking"`
	HideTools         bool          `json:"hide_tools"`
	LastReadMessageID string        `json:"last_read_message_id,omitempty"`
	ReadAt            string        `json:"read_at,omitempty"`
	CreatedAt         string        `json:"created_at"`
	UpdatedAt         string        `json:"updated_at"`
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
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Description          string            `json:"description"`
	Members              []GroupMember     `json:"members"`
	MemberCount          int               `json:"member_count"`
	DiscussionMode       string            `json:"discussion_mode"`
	MaxTurns             int               `json:"max_turns"`
	Running              bool              `json:"running"`
	CurrentAgent         string            `json:"current_agent"`
	PendingApprovalCount int               `json:"pending_approval_count,omitempty"`
	UnreadCount          int               `json:"unread_count"`
	FirstUnreadMessageID string            `json:"first_unread_message_id,omitempty"`
	LastMessage          *GroupLastMessage `json:"last_message,omitempty"`
	CreatedAt            string            `json:"created_at"`
	UpdatedAt            string            `json:"updated_at"`
}

// GroupDetail is returned by GET /groups/:id.
type GroupDetail struct {
	Group                Group          `json:"group"`
	Messages             []GroupMessage `json:"messages"`
	TotalMessages        int            `json:"total_messages"`
	Offset               int            `json:"offset"`
	Limit                int            `json:"limit"`
	HasMore              bool           `json:"has_more"`
	UnreadCount          int            `json:"unread_count"`
	FirstUnreadMessageID string         `json:"first_unread_message_id,omitempty"`
	RunState             GroupRunState  `json:"run_state"`
}

// CreateGroupRequest is the payload for POST /groups.
type CreateGroupRequest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Members        []string `json:"members"`
	DiscussionMode string   `json:"discussion_mode"`
	MaxTurns       int      `json:"max_turns"`
	HideThinking   bool     `json:"hide_thinking"`
	HideTools      bool     `json:"hide_tools"`
}

// UpdateGroupRequest is the payload for PATCH /groups/:id.
type UpdateGroupRequest struct {
	Name           *string   `json:"name"`
	Description    *string   `json:"description"`
	Members        *[]string `json:"members"`
	DiscussionMode *string   `json:"discussion_mode"`
	MaxTurns       *int      `json:"max_turns"`
	HideThinking   *bool     `json:"hide_thinking"`
	HideTools      *bool     `json:"hide_tools"`
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
	Group                Group          `json:"group"`
	Messages             []GroupMessage `json:"messages"`
	UnreadCount          int            `json:"unread_count"`
	FirstUnreadMessageID string         `json:"first_unread_message_id,omitempty"`
}

// GroupStore persists groups, member rosters, and transcripts in controlplane.db.
type GroupStore struct {
	db     *ControlPlaneDB
	ownsDB bool
	mu     sync.Mutex
}

// NewGroupStore opens ~/.myaaw/controlplane/controlplane.db and returns a GroupStore.
func NewGroupStore() (*GroupStore, error) {
	cpdb, err := OpenControlPlaneDB()
	if err != nil {
		return nil, err
	}
	return &GroupStore{db: cpdb, ownsDB: true}, nil
}

// NewGroupStoreWithDB returns a GroupStore backed by a shared *ControlPlaneDB.
func NewGroupStoreWithDB(db *ControlPlaneDB) *GroupStore {
	return &GroupStore{db: db, ownsDB: false}
}

// NewGroupStoreAt opens a dedicated SQLite database at path (ideal for tests).
func NewGroupStoreAt(path string) (*GroupStore, error) {
	cpdb, err := OpenControlPlaneDBAt(path)
	if err != nil {
		return nil, err
	}
	return &GroupStore{db: cpdb, ownsDB: true}, nil
}

// Close closes the underlying database when owned by this GroupStore.
func (s *GroupStore) Close() error {
	if s.ownsDB && s.db != nil {
		return s.db.Close()
	}
	return nil
}

// List returns all persisted groups sorted by UpdatedAt descending, loading
// ordered members and only the latest message per group (not full transcripts).
func (s *GroupStore) List() ([]storedGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.db.DB()

	rows, err := db.Query(`
		SELECT id, name, description, discussion_mode, max_turns, hide_thinking, hide_tools, last_read_message_id, read_at, created_at, updated_at
		FROM groups
		ORDER BY updated_at DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]storedGroup, 0)
	indexByID := make(map[string]int)
	for rows.Next() {
		var (
			g            Group
			hideThinking int
			hideTools    int
		)
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.DiscussionMode, &g.MaxTurns, &hideThinking, &hideTools, &g.LastReadMessageID, &g.ReadAt, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		g.HideThinking = hideThinking != 0
		g.HideTools = hideTools != 0
		g.Members = []GroupMember{}
		sg := storedGroup{Group: g, Messages: []GroupMessage{}}
		normalizeStoredGroup(&sg)
		indexByID[g.ID] = len(result)
		result = append(result, sg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return result, nil
	}

	// Load ordered members for all groups in one query.
	mRows, err := db.Query(`
		SELECT group_id, name, last_seen_message_id, conversation_id
		FROM group_members
		ORDER BY group_id ASC, position ASC`)
	if err != nil {
		return nil, err
	}
	defer mRows.Close()
	for mRows.Next() {
		var (
			groupID string
			member  GroupMember
		)
		if err := mRows.Scan(&groupID, &member.Name, &member.LastSeenMessageID, &member.ConversationID); err != nil {
			return nil, err
		}
		if idx, ok := indexByID[groupID]; ok {
			result[idx].Group.Members = append(result[idx].Group.Members, member)
		}
	}
	if err := mRows.Err(); err != nil {
		return nil, err
	}

	// Load only the latest message per group without scanning full transcripts.
	msgRows, err := db.Query(`
		SELECT id, group_id, sender_type, sender_name, agent_color, content,
		       timeline_json, reply_to_id, reply_preview_json, quotes_json, attachments_json, extra_messages_json, created_at
		FROM group_messages
		WHERE seq IN (SELECT MAX(seq) FROM group_messages GROUP BY group_id)`)
	if err != nil {
		return nil, err
	}
	defer msgRows.Close()
	for msgRows.Next() {
		msg, err := scanGroupMessage(msgRows)
		if err != nil {
			return nil, err
		}
		if idx, ok := indexByID[msg.GroupID]; ok {
			result[idx].Messages = []GroupMessage{msg}
		}
	}
	if err := msgRows.Err(); err != nil {
		return nil, err
	}

	for i := range result {
		unread, firstUnread, err := computeUnreadStats(db, result[i].Group.ID, result[i].Group.LastReadMessageID)
		if err != nil {
			return nil, err
		}
		result[i].UnreadCount = unread
		result[i].FirstUnreadMessageID = firstUnread
	}

	return result, nil
}

// Get reads one stored group, its ordered member roster, and its full transcript.
func (s *GroupStore) Get(id string) (*storedGroup, error) {
	if err := ValidateGroupID(id); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return loadStoredGroup(s.db.DB(), id)
}

type storedGroupPage struct {
	Group                Group
	Messages             []GroupMessage
	TotalMessages        int
	Offset               int
	Limit                int
	HasMore              bool
	UnreadCount          int
	FirstUnreadMessageID string
}

// GetPage reads one stored group and a slice of its transcript using indexed
// seq queries so paged requests never load the entire history into memory.
//
// Pagination rules:
//   - limit <= 0 returns all messages (capped at MaxGroupMessagePageLimit when > 0)
//   - beforeID != "" returns up to limit messages strictly older than beforeID's seq
//     in chronological (ASC) order, immune to concurrent appends at the tail
//   - offset < 0 with limit > 0 returns the tail page of the latest limit messages
//   - HasMore is true iff older messages exist before the returned page (Offset > 0)
func (s *GroupStore) GetPage(id string, limit, offset int, beforeID string) (storedGroupPage, error) {
	if err := ValidateGroupID(id); err != nil {
		return storedGroupPage{}, err
	}

	if limit < 0 {
		limit = 0
	}
	if limit > MaxGroupMessagePageLimit {
		limit = MaxGroupMessagePageLimit
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.db.DB()

	group, err := loadGroupAndMembers(db, id)
	if err != nil {
		return storedGroupPage{}, err
	}

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM group_messages WHERE group_id = ?`, id).Scan(&total); err != nil {
		return storedGroupPage{}, err
	}

	page := storedGroupPage{
		Group:         group,
		Messages:      []GroupMessage{},
		TotalMessages: total,
		Limit:         limit,
	}

	if beforeID != "" {
		var targetSeq int64
		err := db.QueryRow(`SELECT seq FROM group_messages WHERE group_id = ? AND id = ?`, id, beforeID).Scan(&targetSeq)
		if errors.Is(err, sql.ErrNoRows) {
			return page, nil
		}
		if err != nil {
			return storedGroupPage{}, err
		}

		var rows *sql.Rows
		if limit > 0 {
			rows, err = db.Query(`
				SELECT seq, id, group_id, sender_type, sender_name, agent_color, content,
				       timeline_json, reply_to_id, reply_preview_json, quotes_json, attachments_json, extra_messages_json, created_at
				FROM group_messages
				WHERE group_id = ? AND seq < ?
				ORDER BY seq DESC
				LIMIT ?`, id, targetSeq, limit)
		} else {
			rows, err = db.Query(`
				SELECT seq, id, group_id, sender_type, sender_name, agent_color, content,
				       timeline_json, reply_to_id, reply_preview_json, quotes_json, attachments_json, extra_messages_json, created_at
				FROM group_messages
				WHERE group_id = ? AND seq < ?
				ORDER BY seq ASC`, id, targetSeq)
		}
		if err != nil {
			return storedGroupPage{}, err
		}
		defer rows.Close()

		var (
			seqs []int64
			msgs = []GroupMessage{}
		)
		for rows.Next() {
			seq, msg, err := scanGroupMessageWithSeq(rows)
			if err != nil {
				return storedGroupPage{}, err
			}
			seqs = append(seqs, seq)
			msgs = append(msgs, msg)
		}
		if err := rows.Err(); err != nil {
			return storedGroupPage{}, err
		}

		if limit > 0 {
			for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
				msgs[i], msgs[j] = msgs[j], msgs[i]
				seqs[i], seqs[j] = seqs[j], seqs[i]
			}
		}

		page.Messages = msgs
		if len(seqs) > 0 {
			var olderCount int
			if err := db.QueryRow(`SELECT COUNT(*) FROM group_messages WHERE group_id = ? AND seq < ?`, id, seqs[0]).Scan(&olderCount); err != nil {
				return storedGroupPage{}, err
			}
			page.Offset = olderCount
			page.HasMore = olderCount > 0
		}
		return page, nil
	}

	// Offset-based paging (including negative tail offset).
	if offset < 0 && limit > 0 {
		offset = total - limit
	}
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	page.Offset = offset
	page.HasMore = offset > 0

	var rows *sql.Rows
	switch {
	case limit > 0:
		rows, err = db.Query(`
			SELECT id, group_id, sender_type, sender_name, agent_color, content,
			       timeline_json, reply_to_id, reply_preview_json, quotes_json, attachments_json, extra_messages_json, created_at
			FROM group_messages
			WHERE group_id = ?
			ORDER BY seq ASC
			LIMIT ? OFFSET ?`, id, limit, offset)
	case offset > 0:
		rows, err = db.Query(`
			SELECT id, group_id, sender_type, sender_name, agent_color, content,
			       timeline_json, reply_to_id, reply_preview_json, quotes_json, attachments_json, extra_messages_json, created_at
			FROM group_messages
			WHERE group_id = ?
			ORDER BY seq ASC
			LIMIT -1 OFFSET ?`, id, offset)
	default:
		rows, err = db.Query(`
			SELECT id, group_id, sender_type, sender_name, agent_color, content,
			       timeline_json, reply_to_id, reply_preview_json, quotes_json, attachments_json, extra_messages_json, created_at
			FROM group_messages
			WHERE group_id = ?
			ORDER BY seq ASC`, id)
	}
	if err != nil {
		return storedGroupPage{}, err
	}
	defer rows.Close()

	for rows.Next() {
		msg, err := scanGroupMessage(rows)
		if err != nil {
			return storedGroupPage{}, err
		}
		page.Messages = append(page.Messages, msg)
	}
	if err := rows.Err(); err != nil {
		return storedGroupPage{}, err
	}
	unread, firstUnread, err := computeUnreadStats(db, id, group.LastReadMessageID)
	if err != nil {
		return storedGroupPage{}, err
	}
	page.UnreadCount = unread
	page.FirstUnreadMessageID = firstUnread
	return page, nil
}

// MarkRead advances a group's persistent read watermark to messageID (or the
// newest message in the group when messageID is empty).
func (s *GroupStore) MarkRead(id, messageID string) (string, string, error) {
	if err := ValidateGroupID(id); err != nil {
		return "", "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.db.DB()
	var exists int
	if err := db.QueryRow(`SELECT 1 FROM groups WHERE id = ?`, id).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrGroupNotFound
	} else if err != nil {
		return "", "", err
	}

	targetID := strings.TrimSpace(messageID)
	if targetID == "" {
		_ = db.QueryRow(`SELECT id FROM group_messages WHERE group_id = ? ORDER BY seq DESC LIMIT 1`, id).Scan(&targetID)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE groups SET last_read_message_id = ?, read_at = ? WHERE id = ?`, targetID, now, id); err != nil {
		return "", "", err
	}
	return targetID, now, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func computeUnreadStats(q sqlQueryer, groupID, lastReadID string) (int, string, error) {
	var readSeq int64
	if lastReadID != "" {
		_ = q.QueryRow(`SELECT seq FROM group_messages WHERE group_id = ? AND id = ?`, groupID, lastReadID).Scan(&readSeq)
	}
	var (
		count  int
		minSeq int64
	)
	if err := q.QueryRow(`
		SELECT COUNT(*), COALESCE(MIN(seq), 0)
		FROM group_messages
		WHERE group_id = ? AND sender_type = 'agent' AND seq > ?`,
		groupID, readSeq,
	).Scan(&count, &minSeq); err != nil {
		return 0, "", err
	}
	var firstUnreadID string
	if minSeq > 0 {
		_ = q.QueryRow(`SELECT id FROM group_messages WHERE seq = ?`, minSeq).Scan(&firstUnreadID)
	}
	return count, firstUnreadID, nil
}

// Create inserts a new group, its members, and any initial messages atomically.
func (s *GroupStore) Create(sg *storedGroup) error {
	if err := ValidateGroupID(sg.Group.ID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	normalizeStoredGroup(sg)

	tx, err := s.db.DB().Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`
		INSERT INTO groups (id, name, description, discussion_mode, max_turns, hide_thinking, hide_tools, last_read_message_id, read_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sg.Group.ID, sg.Group.Name, sg.Group.Description,
		sg.Group.DiscussionMode, sg.Group.MaxTurns,
		boolToInt(sg.Group.HideThinking), boolToInt(sg.Group.HideTools),
		sg.Group.LastReadMessageID, sg.Group.ReadAt,
		sg.Group.CreatedAt, sg.Group.UpdatedAt,
	); err != nil {
		return err
	}

	if err = replaceMembersTx(tx, sg.Group.ID, sg.Group.Members); err != nil {
		return err
	}
	for _, msg := range sg.Messages {
		if err = insertMessageTx(tx, msg); err != nil {
			return err
		}
	}

	err = tx.Commit()
	return err
}

// Update runs fn against the latest group state inside a SQLite transaction and
// persists all metadata, member roster/cursor, and transcript updates atomically.
func (s *GroupStore) Update(id string, fn func(sg *storedGroup) error) (*storedGroup, error) {
	if err := ValidateGroupID(id); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.DB().Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	sg, err := loadStoredGroup(tx, id)
	if err != nil {
		return nil, err
	}
	existingMsgIDs := make(map[string]bool, len(sg.Messages))
	for _, m := range sg.Messages {
		existingMsgIDs[m.ID] = true
	}

	if err = fn(sg); err != nil {
		return nil, err
	}
	normalizeStoredGroup(sg)

	if _, err = tx.Exec(`
		UPDATE groups
		SET name = ?, description = ?, discussion_mode = ?, max_turns = ?, hide_thinking = ?, hide_tools = ?, last_read_message_id = ?, read_at = ?, updated_at = ?
		WHERE id = ?`,
		sg.Group.Name, sg.Group.Description,
		sg.Group.DiscussionMode, sg.Group.MaxTurns,
		boolToInt(sg.Group.HideThinking), boolToInt(sg.Group.HideTools),
		sg.Group.LastReadMessageID, sg.Group.ReadAt,
		sg.Group.UpdatedAt, id,
	); err != nil {
		return nil, err
	}

	if err = replaceMembersTx(tx, id, sg.Group.Members); err != nil {
		return nil, err
	}

	if len(sg.Messages) == 0 {
		if _, err = tx.Exec(`DELETE FROM group_messages WHERE group_id = ?`, id); err != nil {
			return nil, err
		}
	} else {
		for _, msg := range sg.Messages {
			if existingMsgIDs[msg.ID] {
				continue
			}
			if err = insertMessageTx(tx, msg); err != nil {
				return nil, err
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return cloneStoredGroup(sg), nil
}

// Delete removes the group row (ON DELETE CASCADE cleans up members and messages).
func (s *GroupStore) Delete(id string) error {
	if err := ValidateGroupID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.DB().Exec(`DELETE FROM groups WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrGroupNotFound
	}
	return nil
}

// ApplyExtraBlockInteraction atomically validates and marks an interactive
// extra_messages block on one group message as submitted.
func (s *GroupStore) ApplyExtraBlockInteraction(groupID string, req InteractionRequest, actor string) (provider.InteractionState, string, error) {
	if err := ValidateGroupID(groupID); err != nil {
		return provider.InteractionState{}, "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.db.DB()
	tx, err := db.Begin()
	if err != nil {
		return provider.InteractionState{}, "", err
	}
	defer func() { _ = tx.Rollback() }()

	var extraJSON string
	err = tx.QueryRow(
		`SELECT extra_messages_json FROM group_messages WHERE group_id = ? AND id = ? LIMIT 1`,
		groupID, req.MessageID,
	).Scan(&extraJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return provider.InteractionState{}, "", ErrInteractionNotFound
	}
	if err != nil {
		return provider.InteractionState{}, "", err
	}
	if extraJSON == "" {
		return provider.InteractionState{}, "", ErrInteractionNotFound
	}

	var extras []provider.ExtraMessage
	if err := json.Unmarshal([]byte(extraJSON), &extras); err != nil {
		return provider.InteractionState{}, "", fmt.Errorf("unmarshal extra_messages: %w", err)
	}

	state, text, matched, applyErr := applyBlockInteraction(extras, req, actor)
	if applyErr != nil {
		return state, "", applyErr
	}
	if !matched {
		return provider.InteractionState{}, "", ErrInteractionNotFound
	}

	encoded, err := json.Marshal(extras)
	if err != nil {
		return provider.InteractionState{}, "", fmt.Errorf("marshal extra_messages: %w", err)
	}
	if _, err := tx.Exec(
		`UPDATE group_messages SET extra_messages_json = ? WHERE group_id = ? AND id = ?`,
		string(encoded), groupID, req.MessageID,
	); err != nil {
		return provider.InteractionState{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return provider.InteractionState{}, "", err
	}
	return state, text, nil
}

type sqlQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

func loadGroupAndMembers(q sqlQueryer, id string) (Group, error) {
	var g Group
	var (
		hideThinking int
		hideTools    int
	)
	err := q.QueryRow(`
		SELECT id, name, description, discussion_mode, max_turns, hide_thinking, hide_tools, last_read_message_id, read_at, created_at, updated_at
		FROM groups
		WHERE id = ?`, id,
	).Scan(&g.ID, &g.Name, &g.Description, &g.DiscussionMode, &g.MaxTurns, &hideThinking, &hideTools, &g.LastReadMessageID, &g.ReadAt, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Group{}, ErrGroupNotFound
	}
	if err != nil {
		return Group{}, err
	}
	g.HideThinking = hideThinking != 0
	g.HideTools = hideTools != 0

	mRows, err := q.Query(`
		SELECT name, last_seen_message_id, conversation_id
		FROM group_members
		WHERE group_id = ?
		ORDER BY position ASC`, id)
	if err != nil {
		return Group{}, err
	}
	g.Members = []GroupMember{}
	for mRows.Next() {
		var m GroupMember
		if err := mRows.Scan(&m.Name, &m.LastSeenMessageID, &m.ConversationID); err != nil {
			_ = mRows.Close()
			return Group{}, err
		}
		g.Members = append(g.Members, m)
	}
	if err := mRows.Err(); err != nil {
		_ = mRows.Close()
		return Group{}, err
	}
	_ = mRows.Close()

	if g.DiscussionMode == "" {
		g.DiscussionMode = DiscussionModeAutonomous
	}
	if g.MaxTurns <= 0 {
		g.MaxTurns = DefaultGroupMaxTurns
	}
	return g, nil
}

func loadStoredGroup(q sqlQueryer, id string) (*storedGroup, error) {
	g, err := loadGroupAndMembers(q, id)
	if err != nil {
		return nil, err
	}

	msgRows, err := q.Query(`
		SELECT id, group_id, sender_type, sender_name, agent_color, content,
		       timeline_json, reply_to_id, reply_preview_json, quotes_json, attachments_json, extra_messages_json, created_at
		FROM group_messages
		WHERE group_id = ?
		ORDER BY seq ASC`, id)
	if err != nil {
		return nil, err
	}
	messages := []GroupMessage{}
	for msgRows.Next() {
		msg, err := scanGroupMessage(msgRows)
		if err != nil {
			_ = msgRows.Close()
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := msgRows.Err(); err != nil {
		_ = msgRows.Close()
		return nil, err
	}
	_ = msgRows.Close()

	sg := &storedGroup{Group: g, Messages: messages}
	normalizeStoredGroup(sg)
	return sg, nil
}

func replaceMembersTx(tx *sql.Tx, groupID string, members []GroupMember) error {
	if _, err := tx.Exec(`DELETE FROM group_members WHERE group_id = ?`, groupID); err != nil {
		return err
	}
	for i, m := range members {
		if _, err := tx.Exec(`
			INSERT INTO group_members (group_id, name, position, last_seen_message_id, conversation_id)
			VALUES (?, ?, ?, ?, ?)`,
			groupID, m.Name, i, m.LastSeenMessageID, m.ConversationID,
		); err != nil {
			return err
		}
	}
	return nil
}

func insertMessageTx(tx *sql.Tx, msg GroupMessage) error {
	var (
		timelineJSON      string
		replyPreviewJSON  string
		quotesJSON        string
		attachmentsJSON   string
		extraMessagesJSON string
	)
	if len(msg.Timeline) > 0 {
		b, err := json.Marshal(msg.Timeline)
		if err != nil {
			return fmt.Errorf("marshal timeline: %w", err)
		}
		timelineJSON = string(b)
	}
	if msg.ReplyPreview != nil {
		b, err := json.Marshal(msg.ReplyPreview)
		if err != nil {
			return fmt.Errorf("marshal reply_preview: %w", err)
		}
		replyPreviewJSON = string(b)
	}
	if len(msg.Quotes) > 0 {
		b, err := json.Marshal(msg.Quotes)
		if err != nil {
			return fmt.Errorf("marshal quotes: %w", err)
		}
		quotesJSON = string(b)
	}
	if len(msg.Attachments) > 0 {
		b, err := json.Marshal(msg.Attachments)
		if err != nil {
			return fmt.Errorf("marshal attachments: %w", err)
		}
		attachmentsJSON = string(b)
	}
	if len(msg.ExtraMessages) > 0 {
		b, err := json.Marshal(msg.ExtraMessages)
		if err != nil {
			return fmt.Errorf("marshal extra_messages: %w", err)
		}
		extraMessagesJSON = string(b)
	}

	_, err := tx.Exec(`
		INSERT OR IGNORE INTO group_messages (
			id, group_id, sender_type, sender_name, agent_color, content,
			timeline_json, reply_to_id, reply_preview_json, quotes_json, attachments_json, extra_messages_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.GroupID, msg.SenderType, msg.SenderName, msg.AgentColor, msg.Content,
		timelineJSON, msg.ReplyToID, replyPreviewJSON, quotesJSON, attachmentsJSON, extraMessagesJSON, msg.CreatedAt,
	)
	return err
}

func scanGroupMessage(rows *sql.Rows) (GroupMessage, error) {
	var (
		msg               GroupMessage
		timelineJSON      string
		replyPreviewJSON  string
		quotesJSON        string
		attachmentsJSON   string
		extraMessagesJSON string
	)
	if err := rows.Scan(
		&msg.ID, &msg.GroupID, &msg.SenderType, &msg.SenderName, &msg.AgentColor, &msg.Content,
		&timelineJSON, &msg.ReplyToID, &replyPreviewJSON, &quotesJSON, &attachmentsJSON, &extraMessagesJSON, &msg.CreatedAt,
	); err != nil {
		return GroupMessage{}, err
	}
	if err := decodeMessageJSON(&msg, timelineJSON, replyPreviewJSON, quotesJSON, attachmentsJSON, extraMessagesJSON); err != nil {
		return GroupMessage{}, err
	}
	return msg, nil
}

func scanGroupMessageWithSeq(rows *sql.Rows) (int64, GroupMessage, error) {
	var (
		seq               int64
		msg               GroupMessage
		timelineJSON      string
		replyPreviewJSON  string
		quotesJSON        string
		attachmentsJSON   string
		extraMessagesJSON string
	)
	if err := rows.Scan(
		&seq, &msg.ID, &msg.GroupID, &msg.SenderType, &msg.SenderName, &msg.AgentColor, &msg.Content,
		&timelineJSON, &msg.ReplyToID, &replyPreviewJSON, &quotesJSON, &attachmentsJSON, &extraMessagesJSON, &msg.CreatedAt,
	); err != nil {
		return 0, GroupMessage{}, err
	}
	if err := decodeMessageJSON(&msg, timelineJSON, replyPreviewJSON, quotesJSON, attachmentsJSON, extraMessagesJSON); err != nil {
		return 0, GroupMessage{}, err
	}
	return seq, msg, nil
}

func decodeMessageJSON(msg *GroupMessage, timelineJSON, replyPreviewJSON, quotesJSON, attachmentsJSON, extraMessagesJSON string) error {
	if timelineJSON != "" {
		if err := json.Unmarshal([]byte(timelineJSON), &msg.Timeline); err != nil {
			return fmt.Errorf("unmarshal timeline: %w", err)
		}
	}
	if replyPreviewJSON != "" {
		var rp ReplyPreview
		if err := json.Unmarshal([]byte(replyPreviewJSON), &rp); err != nil {
			return fmt.Errorf("unmarshal reply_preview: %w", err)
		}
		msg.ReplyPreview = &rp
	}
	if quotesJSON != "" {
		if err := json.Unmarshal([]byte(quotesJSON), &msg.Quotes); err != nil {
			return fmt.Errorf("unmarshal quotes: %w", err)
		}
	}
	if attachmentsJSON != "" {
		if err := json.Unmarshal([]byte(attachmentsJSON), &msg.Attachments); err != nil {
			return fmt.Errorf("unmarshal attachments: %w", err)
		}
	}
	if extraMessagesJSON != "" {
		if err := json.Unmarshal([]byte(extraMessagesJSON), &msg.ExtraMessages); err != nil {
			return fmt.Errorf("unmarshal extra_messages: %w", err)
		}
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
		if len(msg.Attachments) > 0 {
			m.Attachments = make([]GroupAttachment, len(msg.Attachments))
			copy(m.Attachments, msg.Attachments)
		}
		if len(msg.ExtraMessages) > 0 {
			m.ExtraMessages = provider.CloneExtraMessages(msg.ExtraMessages)
		}
		out.Messages[i] = m
	}
	return out
}

// UploadsDir returns the on-disk directory holding uploaded files for groupID,
// rooted next to controlplane.db so test databases stay isolated.
func (s *GroupStore) UploadsDir(groupID string) string {
	return filepath.Join(filepath.Dir(s.db.Path()), "group_uploads", groupID)
}

// ListAttachments returns paginated attachments for a group, newest first.
func (s *GroupStore) ListAttachments(groupID, kindFilter string, limit, offset int) (GroupAttachmentsPage, error) {
	if err := ValidateGroupID(groupID); err != nil {
		return GroupAttachmentsPage{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	allowedKinds := map[string]bool{}
	for _, k := range strings.Split(strings.TrimSpace(kindFilter), ",") {
		k = strings.ToLower(strings.TrimSpace(k))
		if k != "" {
			allowedKinds[k] = true
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.db.DB()
	if _, err := loadGroupAndMembers(db, groupID); err != nil {
		return GroupAttachmentsPage{}, err
	}

	rows, err := db.Query(`
		SELECT id, sender_type, sender_name, attachments_json, extra_messages_json, created_at
		FROM group_messages
		WHERE group_id = ? AND (attachments_json != '' OR extra_messages_json != '')
		ORDER BY seq DESC`, groupID)
	if err != nil {
		return GroupAttachmentsPage{}, err
	}
	defer rows.Close()

	all := []GroupAttachment{}
	for rows.Next() {
		var (
			msgID          string
			senderType     string
			senderName     string
			rawJSON        string
			rawExtraJSON   string
			createdAt      string
		)
		if err := rows.Scan(&msgID, &senderType, &senderName, &rawJSON, &rawExtraJSON, &createdAt); err != nil {
			return GroupAttachmentsPage{}, err
		}
		seenPaths := map[string]bool{}
		if rawJSON != "" {
			var atts []GroupAttachment
			if err := json.Unmarshal([]byte(rawJSON), &atts); err == nil {
				for _, att := range atts {
					if att.Path != "" {
						seenPaths[strings.TrimSpace(att.Path)] = true
					}
					if len(allowedKinds) > 0 && !allowedKinds[strings.ToLower(att.Kind)] {
						continue
					}
					if att.GroupID == "" {
						att.GroupID = groupID
					}
					if att.MessageID == "" {
						att.MessageID = msgID
					}
					if att.SenderType == "" {
						att.SenderType = senderType
					}
					if att.SenderName == "" {
						att.SenderName = senderName
					}
					if att.CreatedAt == "" {
						att.CreatedAt = createdAt
					}
					all = append(all, att)
				}
			}
		}
		if rawExtraJSON != "" {
			var extras []provider.ExtraMessage
			if err := json.Unmarshal([]byte(rawExtraJSON), &extras); err == nil {
				for idx, extraAtt := range extractExtraMessageLocalAttachments(extras) {
					p := strings.TrimSpace(extraAtt.Path)
					if p == "" || seenPaths[p] {
						continue
					}
					seenPaths[p] = true
					kind := attachmentKind(extraAtt.Kind, extraAtt.MimeType, p)
					if len(allowedKinds) > 0 && !allowedKinds[strings.ToLower(kind)] {
						continue
					}
					all = append(all, GroupAttachment{
						ID:         fmt.Sprintf("%s:em:%d", msgID, idx),
						GroupID:    groupID,
						MessageID:  msgID,
						SenderType: senderType,
						SenderName: senderName,
						Agent:      senderName,
						Path:       p,
						FileName:   attachmentFileName(extraAtt.FileName, p),
						MimeType:   extraAtt.MimeType,
						Kind:       kind,
						Size:       extraAtt.Size,
						CreatedAt:  createdAt,
					})
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return GroupAttachmentsPage{}, err
	}

	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	items := all[offset:end]
	if items == nil {
		items = []GroupAttachment{}
	}
	return GroupAttachmentsPage{
		Items:   items,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: end < total,
	}, nil
}

// FindAttachment locates one attachment metadata record by (groupID, messageID, storedName).
func (s *GroupStore) FindAttachment(groupID, messageID, storedName string) (GroupAttachment, error) {
	if err := ValidateGroupID(groupID); err != nil {
		return GroupAttachment{}, err
	}
	if strings.TrimSpace(messageID) == "" || strings.TrimSpace(storedName) == "" {
		return GroupAttachment{}, ErrGroupNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var rawJSON string
	err := s.db.DB().QueryRow(`
		SELECT attachments_json
		FROM group_messages
		WHERE group_id = ? AND id = ?`, groupID, messageID).Scan(&rawJSON)
	if errors.Is(err, sql.ErrNoRows) || rawJSON == "" {
		return GroupAttachment{}, ErrGroupNotFound
	}
	if err != nil {
		return GroupAttachment{}, err
	}

	var atts []GroupAttachment
	if err := json.Unmarshal([]byte(rawJSON), &atts); err != nil {
		return GroupAttachment{}, err
	}
	for _, att := range atts {
		if att.StoredName == storedName {
			return att, nil
		}
	}
	return GroupAttachment{}, ErrGroupNotFound
}
