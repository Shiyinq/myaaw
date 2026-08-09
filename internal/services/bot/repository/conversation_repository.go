package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"myaaw/internal/provider"
	"time"

	"myaaw/internal/services/bot/model"

	"github.com/google/uuid"
)

type ConversationRepository interface {
	GetConversationByUserId(userId int) ([]*model.Conversation, error)
	CreateConversation(userId int, title string) (*model.Conversation, error)
	UpdateConversationById(id string, messages []provider.Message, title string) error
	GetActiveConversationByUserId(userId int) (*model.Conversation, error)
}

type ConversationRepositoryImpl struct {
	db *sql.DB
}

func NewConversationRepository(db *sql.DB) ConversationRepository {
	return &ConversationRepositoryImpl{db: db}
}

func (r *ConversationRepositoryImpl) GetConversationByUserId(userId int) ([]*model.Conversation, error) {
	query := `SELECT id, user_id, title, messages, active, created_at, updated_at FROM conversations WHERE user_id = ?`
	rows, err := r.db.Query(query, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []*model.Conversation
	for rows.Next() {
		var conv model.Conversation
		var messagesJSON string
		err := rows.Scan(&conv.Id, &conv.UserId, &conv.Title, &messagesJSON, &conv.Active, &conv.CreatedAt, &conv.UpdatedAt)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(messagesJSON), &conv.Messages); err != nil {
			// fallback if it fails
			conv.Messages = []provider.Message{}
		}

		conversations = append(conversations, &conv)
	}

	return conversations, nil
}

func (r *ConversationRepositoryImpl) CreateConversation(userId int, title string) (*model.Conversation, error) {
	if title == "" {
		title = "New Chat"
	}

	// Deactivate other active conversations
	updateQuery := `UPDATE conversations SET active = 0 WHERE user_id = ? AND active = 1`
	_, _ = r.db.Exec(updateQuery, userId)

	conversation := &model.Conversation{
		Id:        uuid.New().String(),
		UserId:    userId,
		Title:     title,
		Messages:  []provider.Message{},
		Active:    true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	messagesJSON, _ := json.Marshal(conversation.Messages)
	insertQuery := `INSERT INTO conversations (id, user_id, title, messages, active, created_at, updated_at) 
	                VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.Exec(insertQuery,
		conversation.Id, conversation.UserId, conversation.Title, messagesJSON, conversation.Active, conversation.CreatedAt, conversation.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return conversation, nil
}

func (r *ConversationRepositoryImpl) UpdateConversationById(id string, messages []provider.Message, title string) error {
	messagesJSON, _ := json.Marshal(messages)
	updatedAt := time.Now()

	var query string
	var args []interface{}

	if title != "" {
		query = `UPDATE conversations SET messages = ?, title = ?, updated_at = ? WHERE id = ?`
		args = []interface{}{messagesJSON, title, updatedAt, id}
	} else {
		query = `UPDATE conversations SET messages = ?, updated_at = ? WHERE id = ?`
		args = []interface{}{messagesJSON, updatedAt, id}
	}

	_, err := r.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}

	return nil
}

func (r *ConversationRepositoryImpl) GetActiveConversationByUserId(userId int) (*model.Conversation, error) {
	query := `SELECT id, user_id, title, messages, active, created_at, updated_at FROM conversations WHERE user_id = ? AND active = 1 LIMIT 1`

	var conv model.Conversation
	var messagesJSON string

	err := r.db.QueryRow(query, userId).Scan(
		&conv.Id, &conv.UserId, &conv.Title, &messagesJSON, &conv.Active, &conv.CreatedAt, &conv.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if err := json.Unmarshal([]byte(messagesJSON), &conv.Messages); err != nil {
		conv.Messages = []provider.Message{}
	}

	conv.CreatedAt = time.Time{}
	conv.UpdatedAt = time.Time{}
	return &conv, nil
}
