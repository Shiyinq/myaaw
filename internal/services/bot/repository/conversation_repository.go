package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"myaaw/internal/provider"
	"time"

	"myaaw/internal/services/bot/model"

	"github.com/google/uuid"
)

type ConversationRepository interface {
	GetConversationByUserId(userId int) ([]*model.Conversation, error)
	GetConversationById(id string) (*model.Conversation, error)
	CreateConversation(userId int, title string) (*model.Conversation, error)
	CreateConversationWithId(userId int, id, title string) (*model.Conversation, error)
	// CreateConversationInScope starts a new active conversation for a scope
	// (a chat target such as a Telegram topic), deactivating the previous one.
	// An empty scope keeps the legacy per-user behavior.
	CreateConversationInScope(userId int, scope, title string) (*model.Conversation, error)
	UpdateConversationById(id string, messages []provider.Message, title string) error
	UpdateConversationTitle(id string, title string) error
	DeleteConversationById(id string) error
	GetActiveConversationByUserId(userId int) (*model.Conversation, error)
	GetActiveConversationByScope(userId int, scope string) (*model.Conversation, error)
	// UpsertScopeNames records the display name of a chat target (for example a
	// Telegram group and forum topic). Empty values keep the stored name.
	UpsertScopeNames(scope, chatName, topicName string, isForum bool) error
}

type ConversationRepositoryImpl struct {
	db *sql.DB
}

func NewConversationRepository(db *sql.DB) ConversationRepository {
	return &ConversationRepositoryImpl{db: db}
}

const conversationColumns = `id, user_id, scope, title, messages, active, created_at, updated_at`

// rowScanner is implemented by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanConversation(row rowScanner) (*model.Conversation, error) {
	var (
		conv         model.Conversation
		scope        sql.NullString
		messagesJSON string
	)
	if err := row.Scan(
		&conv.Id, &conv.UserId, &scope, &conv.Title, &messagesJSON, &conv.Active, &conv.CreatedAt, &conv.UpdatedAt,
	); err != nil {
		return nil, err
	}
	conv.Scope = scope.String

	if err := json.Unmarshal([]byte(messagesJSON), &conv.Messages); err != nil {
		// fallback if it fails
		conv.Messages = []provider.Message{}
	}
	return &conv, nil
}

func (r *ConversationRepositoryImpl) GetConversationByUserId(userId int) ([]*model.Conversation, error) {
	query := `SELECT ` + conversationColumns + ` FROM conversations WHERE user_id = ? ORDER BY updated_at DESC, created_at DESC`
	rows, err := r.db.Query(query, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []*model.Conversation
	for rows.Next() {
		conv, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conv)
	}

	return conversations, rows.Err()
}

func (r *ConversationRepositoryImpl) GetConversationById(id string) (*model.Conversation, error) {
	query := `SELECT ` + conversationColumns + ` FROM conversations WHERE id = ? LIMIT 1`

	conv, err := scanConversation(r.db.QueryRow(query, id))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return conv, nil
}

func (r *ConversationRepositoryImpl) DeleteConversationById(id string) error {
	query := `DELETE FROM conversations WHERE id = ?`
	_, err := r.db.Exec(query, id)
	return err
}

func (r *ConversationRepositoryImpl) CreateConversation(userId int, title string) (*model.Conversation, error) {
	if title == "" {
		title = "New Chat"
	}

	// Deactivate other active conversations in the legacy (unscoped) scope only,
	// so a DM "new chat" never clears a group topic's active session.
	updateQuery := `UPDATE conversations SET active = 0 WHERE user_id = ? AND scope = '' AND active = 1`
	_, _ = r.db.Exec(updateQuery, userId)

	return r.insertConversation(userId, "", title)
}

// CreateConversationWithId inserts a conversation with a caller-supplied ID.
// A2A uses this to map an opaque contextId to a chat transcript.
func (r *ConversationRepositoryImpl) CreateConversationWithId(userId int, id, title string) (*model.Conversation, error) {
	if id == "" {
		return nil, errors.New("conversation id is required")
	}
	if title == "" {
		title = "New Chat"
	}

	conversation := &model.Conversation{
		Id:        id,
		UserId:    userId,
		Title:     title,
		Messages:  []provider.Message{},
		Active:    true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	messagesJSON, _ := json.Marshal(conversation.Messages)
	insertQuery := `INSERT INTO conversations (id, user_id, scope, title, messages, active, created_at, updated_at)
	                VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := r.db.Exec(insertQuery,
		conversation.Id, conversation.UserId, conversation.Scope, conversation.Title, messagesJSON, conversation.Active, conversation.CreatedAt, conversation.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return conversation, nil
}

// CreateConversationInScope rotates the active session for one scope: the
// previous active row stays as history (active = 0) and a new row becomes the
// current session. Only rows in this scope are touched.
func (r *ConversationRepositoryImpl) CreateConversationInScope(userId int, scope, title string) (*model.Conversation, error) {
	if scope == "" {
		return r.CreateConversation(userId, title)
	}
	if title == "" {
		title = "New Chat"
	}

	if _, err := r.db.Exec(
		`UPDATE conversations SET active = 0 WHERE scope = ? AND active = 1`,
		scope,
	); err != nil {
		return nil, fmt.Errorf("failed to deactivate scope: %w", err)
	}

	return r.insertConversation(userId, scope, title)
}

func (r *ConversationRepositoryImpl) insertConversation(userId int, scope, title string) (*model.Conversation, error) {
	conversation := &model.Conversation{
		Id:        uuid.New().String(),
		UserId:    userId,
		Scope:     scope,
		Title:     title,
		Messages:  []provider.Message{},
		Active:    true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	messagesJSON, _ := json.Marshal(conversation.Messages)
	insertQuery := `INSERT INTO conversations (id, user_id, scope, title, messages, active, created_at, updated_at)
	                VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := r.db.Exec(insertQuery,
		conversation.Id, conversation.UserId, conversation.Scope, conversation.Title, messagesJSON, conversation.Active, conversation.CreatedAt, conversation.UpdatedAt,
	); err != nil {
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

func (r *ConversationRepositoryImpl) UpdateConversationTitle(id string, title string) error {
	_, err := r.db.Exec(
		`UPDATE conversations SET title = ?, updated_at = ? WHERE id = ?`,
		title, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("failed to update conversation title: %w", err)
	}
	return nil
}

func (r *ConversationRepositoryImpl) GetActiveConversationByUserId(userId int) (*model.Conversation, error) {
	query := `SELECT ` + conversationColumns + ` FROM conversations WHERE user_id = ? AND scope = '' AND active = 1 ORDER BY updated_at DESC LIMIT 1`

	conv, err := scanConversation(r.db.QueryRow(query, userId))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	conv.CreatedAt = time.Time{}
	conv.UpdatedAt = time.Time{}
	return conv, nil
}

// GetActiveConversationByScope resolves the current session for a chat target.
// Scoped sessions are shared by everyone in that target, so the lookup ignores
// the user id. An empty scope falls back to the legacy per-user behavior.
func (r *ConversationRepositoryImpl) GetActiveConversationByScope(userId int, scope string) (*model.Conversation, error) {
	if scope == "" {
		return r.GetActiveConversationByUserId(userId)
	}

	query := `SELECT ` + conversationColumns + ` FROM conversations WHERE scope = ? AND active = 1 ORDER BY updated_at DESC LIMIT 1`

	conv, err := scanConversation(r.db.QueryRow(query, scope))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return conv, nil
}

// UpsertScopeNames records the display name of a chat target. Empty values keep
// whatever is already stored, so a message that carries no topic name does not
// erase a name learned earlier.
func (r *ConversationRepositoryImpl) UpsertScopeNames(scope, chatName, topicName string, isForum bool) error {
	if scope == "" {
		return nil
	}

	_, err := r.db.Exec(
		`INSERT INTO chat_scopes (scope, chat_name, topic_name, is_forum, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(scope) DO UPDATE SET
		   chat_name = CASE WHEN excluded.chat_name <> '' THEN excluded.chat_name ELSE chat_scopes.chat_name END,
		   topic_name = CASE WHEN excluded.topic_name <> '' THEN excluded.topic_name ELSE chat_scopes.topic_name END,
		   is_forum = excluded.is_forum,
		   updated_at = excluded.updated_at`,
		scope, chatName, topicName, isForum, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert scope names: %w", err)
	}
	return nil
}
