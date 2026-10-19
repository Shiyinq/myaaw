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
	"myaaw/internal/services/bot/model"

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

// ConversationService reads conversations from an agent's SQLite database.
type ConversationService struct{}

func NewConversationService() *ConversationService {
	return &ConversationService{}
}

func (s *ConversationService) List(agentName string, userID, limit int) ([]ConversationSummary, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	db, err := openAgentDB(agentName)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return []ConversationSummary{}, nil
	}
	defer db.Close()

	query := `SELECT id, user_id, title, active, created_at, updated_at, messages FROM conversations`
	args := make([]any, 0, 2)
	if userID > 0 {
		query += ` WHERE user_id = ?`
		args = append(args, userID)
	}
	query += ` ORDER BY updated_at DESC, created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return []ConversationSummary{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	summaries := make([]ConversationSummary, 0)
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
			return nil, err
		}
		var parsed []provider.Message
		if json.Unmarshal([]byte(messages), &parsed) == nil {
			summary.MessageCount = len(parsed)
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

func (s *ConversationService) Get(agentName, id string) (*model.Conversation, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return nil, err
	}

	db, err := openAgentDB(agentName)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, ErrConversationNotFound
	}
	defer db.Close()

	const query = `SELECT id, user_id, title, messages, active, created_at, updated_at FROM conversations WHERE id = ? LIMIT 1`
	var (
		conv         model.Conversation
		messagesJSON string
	)
	err = db.QueryRow(query, id).Scan(
		&conv.Id,
		&conv.UserId,
		&conv.Title,
		&messagesJSON,
		&conv.Active,
		&conv.CreatedAt,
		&conv.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConversationNotFound
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(messagesJSON), &conv.Messages); err != nil {
		return nil, err
	}
	conv.Messages = agent.MigrateLegacyTranscript(conv.Messages)
	return &conv, nil
}

func openAgentDB(agentName string) (*sql.DB, error) {
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
