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

var ErrConversationNotFound = errors.New("conversation not found")

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

// ConversationService reads conversations from an agent's SQLite database.
type ConversationService struct{}

func NewConversationService() *ConversationService {
	return &ConversationService{}
}

func (s *ConversationService) List(agentName string, userID, limit, offset int) (ConversationPage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

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

	where := ""
	args := make([]any, 0, 3)
	if userID > 0 {
		where = " WHERE user_id = ?"
		args = append(args, userID)
	}

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
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
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

func (s *ConversationService) Get(agentName, id string, limit, offset int) (ConversationDetail, error) {
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
