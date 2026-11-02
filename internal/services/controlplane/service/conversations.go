package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
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
	ID           string     `json:"id"`
	UserID       int        `json:"user_id"`
	Title        string     `json:"title"`
	Active       bool       `json:"active"`
	MessageCount int        `json:"message_count"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Pinned       bool       `json:"pinned"`
	PinnedAt     *time.Time `json:"pinned_at"`
	FolderID     *string    `json:"folder_id"`
}

// ConversationFilter narrows a list to one section of the sidebar. The
// handler rejects combinations; the service applies whatever is set.
type ConversationFilter struct {
	Pinned   bool
	Unfiled  bool
	FolderID string
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
	ID            string             `json:"id"`
	UserID        int                `json:"user_id"`
	Title         string             `json:"title"`
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

	order := ` ORDER BY updated_at DESC, created_at DESC`
	if filter.Pinned {
		order = ` ORDER BY pinned_at DESC, updated_at DESC, created_at DESC`
	}
	query := `SELECT id, user_id, title, active, created_at, updated_at, pinned_at, folder_id, messages FROM conversations` +
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
			pinnedAt sql.NullTime
			folderID sql.NullString
		)
		if err := rows.Scan(
			&summary.ID,
			&summary.UserID,
			&summary.Title,
			&summary.Active,
			&summary.CreatedAt,
			&summary.UpdatedAt,
			&pinnedAt,
			&folderID,
			&messages,
		); err != nil {
			return page, err
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
			summary.MessageCount = len(parsed)
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
		text := strings.Join(strings.Fields(provider.MessageText(message)), " ")
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

	const query = `SELECT id, user_id, title, messages, active, created_at, updated_at, pinned_at, folder_id FROM conversations WHERE id = ? LIMIT 1`
	var (
		detail       ConversationDetail
		messagesJSON string
		pinnedAt     sql.NullTime
		folderID     sql.NullString
	)
	err = db.QueryRow(query, id).Scan(
		&detail.ID,
		&detail.UserID,
		&detail.Title,
		&messagesJSON,
		&detail.Active,
		&detail.CreatedAt,
		&detail.UpdatedAt,
		&pinnedAt,
		&folderID,
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
