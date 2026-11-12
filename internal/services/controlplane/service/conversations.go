package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	pathpkg "path"
	"sort"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/agentdir"
	"myaaw/internal/chatdb"
	"myaaw/internal/provider"

	_ "github.com/mattn/go-sqlite3"
)

var (
	ErrConversationNotFound  = errors.New("conversation not found")
	ErrConversationForbidden = errors.New("conversation belongs to another user")
	ErrInvalidTitle          = errors.New("title must not be empty")
)

type ConversationSummary struct {
	ID     string `json:"id"`
	UserID int    `json:"user_id"`
	Title  string `json:"title"`
	// Scope identifies the chat target the session belongs to, e.g. a Telegram
	// topic ("tg:<chatID>:<threadID>"). Empty for CLI/API/DM sessions, so
	// clients that do not know the field keep working unchanged.
	Scope string `json:"scope,omitempty"`
	// ChatName/TopicName are display names learned from the channel (group name
	// and forum topic). Empty for CLI/API/DM sessions and unknown scopes.
	ChatName  string `json:"chat_name,omitempty"`
	TopicName string `json:"topic_name,omitempty"`
	IsForum   bool   `json:"is_forum,omitempty"`
	// Channel is the platform that owns the session (telegram, discord, api,
	// cli). Empty for sessions created before the column existed.
	Channel      string    `json:"channel,omitempty"`
	Active       bool      `json:"active"`
	MessageCount int       `json:"message_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// ReadAt is the newest turn this user has acknowledged. Null means the
	// conversation was created after read tracking shipped and was never opened;
	// a client can read that as unread. Shared by every device.
	ReadAt   *time.Time `json:"read_at"`
	Pinned   bool       `json:"pinned"`
	PinnedAt *time.Time `json:"pinned_at"`
	FolderID *string    `json:"folder_id"`
}

// ConversationFilter narrows a list to one section of the sidebar. The
// handler rejects combinations; the service applies whatever is set.
type ConversationFilter struct {
	Pinned   bool
	Unfiled  bool
	FolderID string
	// Channel keeps only sessions owned by one platform (telegram, discord,
	// api, cli). Empty lists every platform.
	Channel string
}

type ConversationPage struct {
	Conversations []ConversationSummary `json:"conversations"`
	Total         int                   `json:"total"`
	Limit         int                   `json:"limit"`
	Offset        int                   `json:"offset"`
	HasMore       bool                  `json:"has_more"`
}

// ConversationTurn locates one of the user's questions inside a transcript. The
// turn rail draws one tick per turn, so a client needs the whole list even when
// it has only loaded a page of messages.
type ConversationTurn struct {
	// Index is the position of the question in the stored transcript.
	Index int    `json:"index"`
	Text  string `json:"text"`
}

type ConversationDetail struct {
	ID     string `json:"id"`
	UserID int    `json:"user_id"`
	Title  string `json:"title"`
	// Scope identifies the chat target (see ConversationSummary.Scope).
	Scope         string             `json:"scope,omitempty"`
	ChatName      string             `json:"chat_name,omitempty"`
	TopicName     string             `json:"topic_name,omitempty"`
	IsForum       bool               `json:"is_forum,omitempty"`
	Channel       string             `json:"channel,omitempty"`
	Active        bool               `json:"active"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	Pinned        bool               `json:"pinned"`
	PinnedAt      *time.Time         `json:"pinned_at"`
	FolderID      *string            `json:"folder_id"`
	Messages      []provider.Message `json:"messages"`
	TotalMessages int                `json:"total_messages"`
	Limit         int                `json:"limit"`
	Offset        int                `json:"offset"`
	HasMore       bool               `json:"has_more"`
	// Turns covers the whole conversation, not only the returned messages, because
	// the client draws one tick per question. It is not paginated: every detail
	// response carries the full list. If improvement is needed (thousands of turns,
	// or a payload that starts to hurt), either ship the indexes only and fetch
	// labels per window, cap the list with a `turns_truncated` flag, or lower
	// turnTextLimit.
	Turns []ConversationTurn `json:"turns"`
}

// ConversationAttachment is one file that appears in a conversation, either
// delivered by the agent or uploaded by the user.
//
// Path is always a virtual path the dashboard can fetch through /fs/raw, so
// user uploads stored under <agent>/user/uploads are exposed from that folder
// on. MessageIndex points at the transcript entry that carried the file, which
// is what "Show in chat" scrolls to.
type ConversationAttachment struct {
	Path         string     `json:"path"`
	FileName     string     `json:"file_name"`
	MimeType     string     `json:"mime_type,omitempty"`
	Kind         string     `json:"kind"`
	Size         int64      `json:"size,omitempty"`
	Origin       string     `json:"origin"`
	MessageID    string     `json:"message_id,omitempty"`
	MessageIndex int        `json:"message_index"`
	CreatedAt    *time.Time `json:"created_at,omitempty"`
}

// ConversationAttachments is one page of the files found in a conversation.
// Total counts every file after deduplication and kind filtering, so a client
// can page without re-reading the transcript and use total for its badge.
type ConversationAttachments struct {
	Items   []ConversationAttachment `json:"items"`
	Total   int                      `json:"total"`
	Offset  int                      `json:"offset"`
	HasMore bool                     `json:"has_more"`
}

// ActivityDay is one calendar day with the number of conversations created
// that day.
type ActivityDay struct {
	Date  string `json:"date" example:"2026-10-02"`
	Count int    `json:"count" example:"3"`
}

// ConversationActivity is a per-day histogram of created conversations, used by
// the dashboard activity map.
type ConversationActivity struct {
	From  int64         `json:"from" example:"1730419200000"`
	To    int64         `json:"to" example:"1733011199999"`
	Total int           `json:"total" example:"42"`
	Days  []ActivityDay `json:"days"`
}

// Activity counts conversations created per local day inside [from, to].
// Rows are read for the user and bucketed in Go: the daemon stores created_at
// with time.Now(), so dates follow the machine's local timezone, while SQLite's
// date() would convert them to UTC and shift days around midnight.
func (s *ConversationService) Activity(agentName string, userID int, from, to time.Time) (ConversationActivity, error) {
	from = startOfDay(from)
	to = startOfDay(to)
	activity := ConversationActivity{From: from.UnixMilli(), To: to.UnixMilli(), Days: []ActivityDay{}}

	db, err := openAgentDB(agentName)
	if err != nil {
		return activity, err
	}
	if db == nil {
		return fillActivityDays(activity, nil, from, to), nil
	}
	defer db.Close()

	rows, err := db.Query(`SELECT created_at FROM conversations WHERE user_id = ?`, userID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return fillActivityDays(activity, nil, from, to), nil
		}
		return activity, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var created time.Time
		if err := rows.Scan(&created); err != nil {
			return activity, err
		}
		day := startOfDay(created.In(time.Local))
		if day.Before(from) || day.After(to) {
			continue
		}
		counts[day.Format(activityDateLayout)]++
	}
	if err := rows.Err(); err != nil {
		return activity, err
	}

	return fillActivityDays(activity, counts, from, to), nil
}

const activityDateLayout = "2006-01-02"

// startOfDay returns midnight of value's day in its own location.
func startOfDay(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

// fillActivityDays appends every date in [from, to] so the client never has to
// fill gaps, and sums the total while doing so.
func fillActivityDays(activity ConversationActivity, counts map[string]int, from, to time.Time) ConversationActivity {
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		key := day.Format(activityDateLayout)
		count := counts[key]
		activity.Total += count
		activity.Days = append(activity.Days, ActivityDay{Date: key, Count: count})
	}
	return activity
}

// visibleMessageCount counts the rows a chat actually shows as bubbles: the
// user's messages and the assistant answers that carry text. Tool calls, tool
// results, and the empty assistant stubs that only hold `tool_calls` belong to a
// step inside one answer, so counting them made the sidebar claim six messages
// for a chat that reads as two.
func visibleMessageCount(messages []provider.Message) int {
	count := 0
	for _, message := range messages {
		switch strings.ToLower(message.Role) {
		case "user":
			count++
		case "assistant":
			if strings.TrimSpace(provider.MessageText(message)) != "" {
				count++
			}
		}
	}
	return count
}

// ConversationService reads and mutates conversations in an agent's SQLite database.
type ConversationService struct{}

func NewConversationService() *ConversationService {
	return &ConversationService{}
}

// List returns conversations owned by userID. search matches the title only.
func (s *ConversationService) List(
	agentName string,
	userID, limit, offset int,
	search string,
	filter ConversationFilter,
) (ConversationPage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	search = strings.TrimSpace(search)

	page := ConversationPage{
		Conversations: []ConversationSummary{},
		Limit:         limit,
		Offset:        offset,
	}

	db, err := openAgentDB(agentName)
	if err != nil {
		return page, err
	}
	if db == nil {
		return page, nil
	}
	defer db.Close()

	conditions := []string{"user_id = ?"}
	args := []any{userID}
	if filter.Pinned {
		conditions = append(conditions, "pinned_at IS NOT NULL")
	}
	if filter.Unfiled {
		conditions = append(conditions, "pinned_at IS NULL", "folder_id IS NULL")
	}
	if filter.FolderID != "" {
		conditions = append(conditions, "folder_id = ?")
		args = append(args, filter.FolderID)
	}
	if filter.Channel != "" {
		conditions = append(conditions, "channel = ?")
		args = append(args, filter.Channel)
	}
	if search != "" {
		conditions = append(conditions, `title LIKE ? ESCAPE '\'`)
		args = append(args, "%"+escapeLike(search)+"%")
	}
	where := " WHERE " + strings.Join(conditions, " AND ")

	if err := db.QueryRow(`SELECT COUNT(*) FROM conversations`+where, args...).Scan(&page.Total); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return page, nil
		}
		return page, err
	}
	if page.Total == 0 {
		return page, nil
	}

	order := ` ORDER BY c.updated_at DESC, c.created_at DESC`
	if filter.Pinned {
		order = ` ORDER BY c.pinned_at DESC, c.updated_at DESC, c.created_at DESC`
	}
	query := `SELECT c.id, c.user_id, c.scope, c.channel, c.title, c.active, c.created_at, c.updated_at,
			c.read_at, c.pinned_at, c.folder_id, c.messages,
			COALESCE(s.chat_name, ''), COALESCE(s.topic_name, ''), COALESCE(s.is_forum, 0)
		FROM conversations c
		LEFT JOIN chat_scopes s ON s.scope = c.scope` +
		where + order + ` LIMIT ? OFFSET ?`
	queryArgs := append(append([]any{}, args...), limit, offset)

	rows, err := db.Query(query, queryArgs...)
	if err != nil {
		return page, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			summary  ConversationSummary
			messages string
			readAt   sql.NullTime
			pinnedAt sql.NullTime
			folderID sql.NullString
		)
		if err := rows.Scan(
			&summary.ID,
			&summary.UserID,
			&summary.Scope,
			&summary.Channel,
			&summary.Title,
			&summary.Active,
			&summary.CreatedAt,
			&summary.UpdatedAt,
			&readAt,
			&pinnedAt,
			&folderID,
			&messages,
			&summary.ChatName,
			&summary.TopicName,
			&summary.IsForum,
		); err != nil {
			return page, err
		}
		if readAt.Valid {
			value := readAt.Time
			summary.ReadAt = &value
		}
		summary.Pinned = pinnedAt.Valid
		if pinnedAt.Valid {
			value := pinnedAt.Time
			summary.PinnedAt = &value
		}
		if folderID.Valid {
			value := folderID.String
			summary.FolderID = &value
		}
		var parsed []provider.Message
		if json.Unmarshal([]byte(messages), &parsed) == nil {
			summary.MessageCount = visibleMessageCount(parsed)
		}
		page.Conversations = append(page.Conversations, summary)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}

	page.HasMore = page.Offset+len(page.Conversations) < page.Total
	return page, nil
}

// turnTextLimit caps how much of a question travels with the turn summary. The
// rail only needs a label; the full text is already in the messages page.
const turnTextLimit = 160

// conversationTurns summarises the user's questions: where each one sits in the
// transcript and a one-line label. Images and tool output are left out so the
// summary stays small enough to ship with every transcript page.
func conversationTurns(messages []provider.Message) []ConversationTurn {
	turns := make([]ConversationTurn, 0, len(messages)/4)
	for index, message := range messages {
		if message.Role != "user" {
			continue
		}
		text := strings.Join(strings.Fields(provider.StripContextBlocks(provider.MessageText(message))), " ")
		if runes := []rune(text); len(runes) > turnTextLimit {
			text = string(runes[:turnTextLimit])
		}
		turns = append(turns, ConversationTurn{Index: index, Text: text})
	}
	return turns
}

// Get returns one conversation owned by userID with a page of its transcript.
func (s *ConversationService) Get(agentName, id string, userID, limit, offset int) (ConversationDetail, error) {
	db, err := openAgentDB(agentName)
	if err != nil {
		return ConversationDetail{}, err
	}
	if db == nil {
		return ConversationDetail{}, ErrConversationNotFound
	}
	defer db.Close()

	const query = `SELECT c.id, c.user_id, c.scope, c.channel, c.title, c.messages, c.active, c.created_at, c.updated_at,
			c.pinned_at, c.folder_id, COALESCE(s.chat_name, ''), COALESCE(s.topic_name, ''), COALESCE(s.is_forum, 0)
		FROM conversations c
		LEFT JOIN chat_scopes s ON s.scope = c.scope
		WHERE c.id = ? LIMIT 1`
	var (
		detail       ConversationDetail
		messagesJSON string
		pinnedAt     sql.NullTime
		folderID     sql.NullString
	)
	err = db.QueryRow(query, id).Scan(
		&detail.ID,
		&detail.UserID,
		&detail.Scope,
		&detail.Channel,
		&detail.Title,
		&messagesJSON,
		&detail.Active,
		&detail.CreatedAt,
		&detail.UpdatedAt,
		&pinnedAt,
		&folderID,
		&detail.ChatName,
		&detail.TopicName,
		&detail.IsForum,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ConversationDetail{}, ErrConversationNotFound
	}
	if err != nil {
		return ConversationDetail{}, err
	}
	detail.Pinned = pinnedAt.Valid
	if pinnedAt.Valid {
		value := pinnedAt.Time
		detail.PinnedAt = &value
	}
	if folderID.Valid {
		value := folderID.String
		detail.FolderID = &value
	}
	if detail.UserID != userID {
		return ConversationDetail{}, ErrConversationForbidden
	}

	if err := json.Unmarshal([]byte(messagesJSON), &detail.Messages); err != nil {
		return ConversationDetail{}, err
	}
	detail.Messages = agent.MigrateLegacyTranscript(detail.Messages)
	detail.TotalMessages = len(detail.Messages)
	detail.Turns = conversationTurns(detail.Messages)

	// A negative offset asks for the last page, so a client can open a long
	// conversation on its newest messages without first reading the total.
	if offset < 0 && limit > 0 {
		offset = detail.TotalMessages - limit
	}
	if offset < 0 {
		offset = 0
	}
	if offset > detail.TotalMessages {
		offset = detail.TotalMessages
	}
	if limit < 0 {
		limit = 0
	}
	if limit > 500 {
		limit = 500
	}

	if limit == 0 {
		detail.Messages = detail.Messages[offset:]
	} else {
		end := offset + limit
		if end > detail.TotalMessages {
			end = detail.TotalMessages
		}
		detail.Messages = detail.Messages[offset:end]
	}

	detail.Limit = limit
	detail.Offset = offset
	detail.HasMore = offset+len(detail.Messages) < detail.TotalMessages
	return detail, nil
}

// Attachments lists every file in a conversation, oldest first, deduplicated by
// path. It reads the stored transcript and scans it in Go: chat transcripts are
// one JSON column, so there is no per-file table to query and a compact scan is
// cheaper than shipping the whole transcript to a browser.
//
// limit defaults to 50 and is capped at 200; offset walks the deduplicated list.
// kinds keeps only the listed rendering kinds (image, video, audio, pdf, file).
func (s *ConversationService) Attachments(
	agentName, id string,
	userID, limit, offset int,
	kinds []string,
) (ConversationAttachments, error) {
	page := ConversationAttachments{Items: []ConversationAttachment{}, Offset: 0}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	db, err := openAgentDB(agentName)
	if err != nil {
		return page, err
	}
	if db == nil {
		return page, ErrConversationNotFound
	}
	defer db.Close()

	var (
		owner        int
		messagesJSON string
	)
	err = db.QueryRow(
		`SELECT user_id, messages FROM conversations WHERE id = ? LIMIT 1`, id,
	).Scan(&owner, &messagesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return page, ErrConversationNotFound
	}
	if err != nil {
		return page, err
	}
	if owner != userID {
		return page, ErrConversationForbidden
	}

	var messages []provider.Message
	if err := json.Unmarshal([]byte(messagesJSON), &messages); err != nil {
		return page, err
	}
	messages = agent.MigrateLegacyTranscript(messages)

	items := collectConversationAttachments(messages)
	if len(kinds) > 0 {
		allowed := make(map[string]bool, len(kinds))
		for _, kind := range kinds {
			allowed[kind] = true
		}
		filtered := items[:0]
		for _, item := range items {
			if allowed[item.Kind] {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	page.Total = len(items)
	page.Offset = offset
	if offset > len(items) {
		offset = len(items)
		page.Offset = offset
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	page.Items = append(page.Items, items[offset:end]...)
	page.HasMore = end < len(items)
	return page, nil
}

// collectConversationAttachments walks a transcript in order and keeps the
// first turn each path appears in. Agent deliveries come first because they are
// stored on the answer that carried them; user uploads follow their own turn.
func collectConversationAttachments(messages []provider.Message) []ConversationAttachment {
	items := make([]ConversationAttachment, 0, 8)
	seen := make(map[string]bool)

	for index, message := range messages {
		for _, attachment := range message.Attachments {
			path := strings.TrimSpace(attachment.Path)
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			items = append(items, ConversationAttachment{
				Path:         path,
				FileName:     attachmentFileName(attachment.FileName, path),
				MimeType:     attachment.MimeType,
				Kind:         attachmentKind(attachment.Kind, attachment.MimeType, path),
				Size:         attachment.Size,
				Origin:       "agent",
				MessageID:    message.ID,
				MessageIndex: index,
				CreatedAt:    message.CreatedAt,
			})
		}

		for _, upload := range message.Uploads {
			path := uploadVirtualPath(upload.Path)
			// A path outside /user/uploads cannot be served by /fs/raw; leaving it
			// out also keeps the agent's host paths out of the browser.
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			items = append(items, ConversationAttachment{
				Path:         path,
				FileName:     attachmentFileName(upload.Name, path),
				MimeType:     upload.MimeType,
				Kind:         attachmentKind("", upload.MimeType, path),
				Size:         upload.Size,
				Origin:       "user",
				MessageID:    message.ID,
				MessageIndex: index,
				CreatedAt:    message.CreatedAt,
			})
		}
	}

	// The transcript is appended chronologically, so index order is already the
	// 	oldest-first order. Sorting by created_at first keeps pages stable if a
	// clock ever wrote a turn out of order, with the index as the tie-breaker.
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if left.CreatedAt != nil && right.CreatedAt != nil && !left.CreatedAt.Equal(*right.CreatedAt) {
			return left.CreatedAt.Before(*right.CreatedAt)
		}
		return left.MessageIndex < right.MessageIndex
	})
	return items
}

// attachmentFileName falls back to the path's base name for entries stored
// before file names were recorded.
func attachmentFileName(name, path string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return pathpkg.Base(strings.ReplaceAll(path, "\\", "/"))
}

// attachmentKind normalises the stored rendering kind and derives one from the
// MIME type or extension when an older entry has none.
func attachmentKind(kind, mimeType, path string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "image", "video", "audio", "pdf", "file":
		return strings.ToLower(strings.TrimSpace(kind))
	}

	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	case mimeType == "application/pdf":
		return "pdf"
	case mimeType != "":
		return "file"
	}

	switch strings.ToLower(pathpkg.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".avif", ".ico":
		return "image"
	case ".mp4", ".m4v", ".webm", ".mov", ".mkv", ".avi", ".ogv":
		return "video"
	case ".mp3", ".wav", ".ogg", ".flac", ".m4a", ".aac", ".opus":
		return "audio"
	case ".pdf":
		return "pdf"
	default:
		return "file"
	}
}

// uploadVirtualPath keeps a user upload addressable by the Files API. The
// transcript stores the absolute path on the agent host, while /fs/raw speaks in
// virtual roots, so the path from /user/uploads on is what the browser gets.
func uploadVirtualPath(raw string) string {
	path := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	if path == "" {
		return ""
	}
	const marker = "/user/uploads/"
	if index := strings.Index(path, marker); index >= 0 {
		return path[index:]
	}
	return ""
}

// Rename updates a conversation title. It only affects conversations owned by userID.
func (s *ConversationService) Rename(agentName, id string, userID int, title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", ErrInvalidTitle
	}

	db, err := openAgentDB(agentName)
	if err != nil {
		return "", err
	}
	if db == nil {
		return "", ErrConversationNotFound
	}
	defer db.Close()

	owner, err := conversationOwner(db, id)
	if err != nil {
		return "", err
	}
	if owner != userID {
		return "", ErrConversationForbidden
	}

	if _, err := db.Exec(
		`UPDATE conversations SET title = ?, updated_at = ? WHERE id = ?`,
		title, time.Now().UTC(), id,
	); err != nil {
		return "", err
	}
	return title, nil
}

// MarkRead records the newest turn the reader has acknowledged, so "unread" is
// shared by every device instead of living in one browser's storage. It is
// monotonic and clamped to the conversation's own updated_at, so a replayed or
// future timestamp can never move the marker past a turn that exists and hide
// it. A nil `at` means "now".
func (s *ConversationService) MarkRead(agentName, id string, userID int, at *time.Time) error {
	db, err := openAgentDB(agentName)
	if err != nil {
		return err
	}
	if db == nil {
		return ErrConversationNotFound
	}
	defer db.Close()

	owner, err := conversationOwner(db, id)
	if err != nil {
		return err
	}
	if owner != userID {
		return ErrConversationForbidden
	}

	var updatedAt, current sql.NullTime
	if err := db.QueryRow(
		`SELECT updated_at, read_at FROM conversations WHERE id = ? LIMIT 1`, id,
	).Scan(&updatedAt, &current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConversationNotFound
		}
		return err
	}

	when := time.Now().UTC()
	if at != nil {
		when = *at
	}
	if updatedAt.Valid && when.After(updatedAt.Time) {
		when = updatedAt.Time
	}
	if current.Valid && !when.After(current.Time) {
		return nil
	}

	_, err = db.Exec(
		`UPDATE conversations SET read_at = ? WHERE id = ? AND user_id = ?`, when, id, userID,
	)
	return err
}

// Delete removes a conversation. It only affects conversations owned by userID.
func (s *ConversationService) Delete(agentName, id string, userID int) error {
	db, err := openAgentDB(agentName)
	if err != nil {
		return err
	}
	if db == nil {
		return ErrConversationNotFound
	}
	defer db.Close()

	owner, err := conversationOwner(db, id)
	if err != nil {
		return err
	}
	if owner != userID {
		return ErrConversationForbidden
	}

	_, err = db.Exec(`DELETE FROM conversations WHERE id = ?`, id)
	return err
}

func conversationOwner(db *sql.DB, id string) (int, error) {
	var userID int
	err := db.QueryRow(`SELECT user_id FROM conversations WHERE id = ? LIMIT 1`, id).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrConversationNotFound
	}
	return userID, err
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func openAgentDB(agentName string) (*sql.DB, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return nil, err
	}
	dbPath, err := agentdir.DatabasePath(agentName)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureChatSchema(dbPath, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// ensureChatSchema runs the additive migration once per database file in this
// process. The control plane may open a database created by an older agent
// build, so it cannot rely on config.initSchema having run.
var ensuredChatSchema sync.Map

func ensureChatSchema(dbPath string, db *sql.DB) error {
	if _, done := ensuredChatSchema.Load(dbPath); done {
		return nil
	}
	if err := chatdb.EnsureSchema(db); err != nil {
		return err
	}
	ensuredChatSchema.Store(dbPath, true)
	return nil
}
