package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/agentdir"
	"myaaw/internal/provider"

	_ "github.com/mattn/go-sqlite3"
)

var (
	ErrConversationNotFound  = errors.New("conversation not found")
	ErrConversationForbidden = errors.New("conversation belongs to another user")
	ErrInvalidTitle          = errors.New("title must not be empty")
)

type ConversationSummary struct {
	ID           string    `json:"id"`
	UserID       int       `json:"user_id"`
	Title        string    `json:"title"`
	Active       bool      `json:"active"`
	MessageCount int       `json:"message_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ConversationPage struct {
	Conversations []ConversationSummary `json:"conversations"`
	Total         int                   `json:"total"`
	Limit         int                   `json:"limit"`
	Offset        int                   `json:"offset"`
	HasMore       bool                  `json:"has_more"`
}

type ConversationDetail struct {
	ID            string             `json:"id"`
	UserID        int                `json:"user_id"`
	Title         string             `json:"title"`
	Active        bool               `json:"active"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	Messages      []provider.Message `json:"messages"`
	TotalMessages int                `json:"total_messages"`
	Limit         int                `json:"limit"`
	Offset        int                `json:"offset"`
	HasMore       bool               `json:"has_more"`
}

// ConversationService reads and mutates conversations in an agent's SQLite database.
type ConversationService struct{}

func NewConversationService() *ConversationService {
	return &ConversationService{}
}

// List returns conversations owned by userID. search matches the title only.
func (s *ConversationService) List(agentName string, userID, limit, offset int, search string) (ConversationPage, error) {
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

	query := `SELECT id, user_id, title, active, created_at, updated_at, messages FROM conversations` +
		where + ` ORDER BY updated_at DESC, created_at DESC LIMIT ? OFFSET ?`
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
		)
		if err := rows.Scan(
			&summary.ID,
			&summary.UserID,
			&summary.Title,
			&summary.Active,
			&summary.CreatedAt,
			&summary.UpdatedAt,
			&messages,
		); err != nil {
			return page, err
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

	const query = `SELECT id, user_id, title, messages, active, created_at, updated_at FROM conversations WHERE id = ? LIMIT 1`
	var (
		detail       ConversationDetail
		messagesJSON string
	)
	err = db.QueryRow(query, id).Scan(
		&detail.ID,
		&detail.UserID,
		&detail.Title,
		&messagesJSON,
		&detail.Active,
		&detail.CreatedAt,
		&detail.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ConversationDetail{}, ErrConversationNotFound
	}
	if err != nil {
		return ConversationDetail{}, err
	}
	if detail.UserID != userID {
		return ConversationDetail{}, ErrConversationForbidden
	}

	if err := json.Unmarshal([]byte(messagesJSON), &detail.Messages); err != nil {
		return ConversationDetail{}, err
	}
	detail.Messages = agent.MigrateLegacyTranscript(detail.Messages)
	detail.TotalMessages = len(detail.Messages)

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
	return db, nil
}
