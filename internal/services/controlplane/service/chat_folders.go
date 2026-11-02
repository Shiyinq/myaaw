package service

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrFolderNotFound    = errors.New("folder not found")
	ErrFolderNameTaken   = errors.New("folder name already in use")
	ErrInvalidFolderName = errors.New("folder name must be 1-64 characters")
	ErrTooManyFolders    = errors.New("too many folders")
)

const (
	maxFoldersPerUser  = 100
	maxFolderNameRunes = 64
)

// ChatFolder is one user-created group of conversations.
type ChatFolder struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Count     int       `json:"count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ChatFolderList struct {
	Folders []ChatFolder `json:"folders"`
}

// ChatFolderDelete reports what a folder deletion removed, so the client can
// leave a transcript that no longer exists.
type ChatFolderDelete struct {
	OK           bool     `json:"ok"`
	DeletedChats int      `json:"deleted_chats"`
	DeletedIDs   []string `json:"deleted_ids"`
}

func normalizeFolderName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > maxFolderNameRunes {
		return "", ErrInvalidFolderName
	}
	return name, nil
}

// ListFolders returns the user's folders with the number of chats in each.
func (s *ConversationService) ListFolders(agentName string, userID int) ([]ChatFolder, error) {
	folders := []ChatFolder{}
	db, err := openAgentDB(agentName)
	if err != nil {
		return folders, err
	}
	if db == nil {
		return folders, nil
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT f.id, f.name, f.created_at, f.updated_at, COUNT(c.id)
		FROM chat_folders f
		LEFT JOIN conversations c ON c.folder_id = f.id AND c.user_id = f.user_id
		WHERE f.user_id = ?
		GROUP BY f.id, f.name, f.created_at, f.updated_at
		ORDER BY f.name COLLATE NOCASE ASC, f.created_at ASC`, userID)
	if err != nil {
		return folders, err
	}
	defer rows.Close()

	for rows.Next() {
		var folder ChatFolder
		if err := rows.Scan(
			&folder.ID,
			&folder.Name,
			&folder.CreatedAt,
			&folder.UpdatedAt,
			&folder.Count,
		); err != nil {
			return folders, err
		}
		folders = append(folders, folder)
	}
	return folders, rows.Err()
}

// CreateFolder adds a folder for userID. Names are unique per user, ignoring
// case, so two drop targets never look the same.
func (s *ConversationService) CreateFolder(agentName string, userID int, name string) (ChatFolder, error) {
	clean, err := normalizeFolderName(name)
	if err != nil {
		return ChatFolder{}, err
	}

	db, err := openAgentDB(agentName)
	if err != nil {
		return ChatFolder{}, err
	}
	if db == nil {
		return ChatFolder{}, ErrAgentNotFound
	}
	defer db.Close()

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM chat_folders WHERE user_id = ?`, userID).Scan(&total); err != nil {
		return ChatFolder{}, err
	}
	if total >= maxFoldersPerUser {
		return ChatFolder{}, ErrTooManyFolders
	}

	folder := ChatFolder{
		ID:        uuid.New().String(),
		Name:      clean,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if _, err := db.Exec(
		`INSERT INTO chat_folders (id, user_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		folder.ID, userID, folder.Name, folder.CreatedAt, folder.UpdatedAt,
	); err != nil {
		if isUniqueViolation(err) {
			return ChatFolder{}, ErrFolderNameTaken
		}
		return ChatFolder{}, err
	}
	return folder, nil
}

// RenameFolder changes one folder's name; its chats stay inside.
func (s *ConversationService) RenameFolder(agentName, id string, userID int, name string) (ChatFolder, error) {
	clean, err := normalizeFolderName(name)
	if err != nil {
		return ChatFolder{}, err
	}

	db, err := openAgentDB(agentName)
	if err != nil {
		return ChatFolder{}, err
	}
	if db == nil {
		return ChatFolder{}, ErrFolderNotFound
	}
	defer db.Close()

	owner, err := folderOwner(db, id)
	if err != nil {
		return ChatFolder{}, err
	}
	if owner != userID {
		return ChatFolder{}, ErrFolderNotFound
	}

	if _, err := db.Exec(
		`UPDATE chat_folders SET name = ?, updated_at = ? WHERE id = ?`,
		clean, time.Now().UTC(), id,
	); err != nil {
		if isUniqueViolation(err) {
			return ChatFolder{}, ErrFolderNameTaken
		}
		return ChatFolder{}, err
	}

	folder := ChatFolder{ID: id, Name: clean}
	if err := db.QueryRow(
		`SELECT created_at, updated_at FROM chat_folders WHERE id = ?`, id,
	).Scan(&folder.CreatedAt, &folder.UpdatedAt); err != nil {
		return ChatFolder{}, err
	}
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM conversations WHERE folder_id = ? AND user_id = ?`, id, userID,
	).Scan(&folder.Count); err != nil {
		return ChatFolder{}, err
	}
	return folder, nil
}

// DeleteFolder removes the folder and every conversation inside it, in one
// transaction. The deleted ids let the UI leave an open transcript.
func (s *ConversationService) DeleteFolder(agentName, id string, userID int) (ChatFolderDelete, error) {
	result := ChatFolderDelete{OK: true, DeletedIDs: []string{}}

	db, err := openAgentDB(agentName)
	if err != nil {
		return result, err
	}
	if db == nil {
		return result, ErrFolderNotFound
	}
	defer db.Close()

	owner, err := folderOwner(db, id)
	if err != nil {
		return result, err
	}
	if owner != userID {
		return result, ErrFolderNotFound
	}

	tx, err := db.Begin()
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query(
		`SELECT id FROM conversations WHERE folder_id = ? AND user_id = ?`, id, userID,
	)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var conversationID string
		if err := rows.Scan(&conversationID); err != nil {
			rows.Close()
			return result, err
		}
		result.DeletedIDs = append(result.DeletedIDs, conversationID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()

	if _, err := tx.Exec(`DELETE FROM conversations WHERE folder_id = ? AND user_id = ?`, id, userID); err != nil {
		return result, err
	}
	res, err := tx.Exec(`DELETE FROM chat_folders WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return result, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return result, err
	}
	if affected == 0 {
		return result, ErrFolderNotFound
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}

	result.DeletedChats = len(result.DeletedIDs)
	return result, nil
}

// SetPinned pins or unpins one conversation. Pinning never changes updated_at,
// so the Recents order keeps following the last message.
func (s *ConversationService) SetPinned(agentName, id string, userID int, pinned bool) error {
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

	var value any
	if pinned {
		value = time.Now().UTC()
	}
	_, err = db.Exec(`UPDATE conversations SET pinned_at = ? WHERE id = ?`, value, id)
	return err
}

// SetFolder moves a conversation into a folder, or to Recents when folderID is
// nil. The target folder must belong to the same user.
func (s *ConversationService) SetFolder(agentName, id string, userID int, folderID *string) error {
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

	var value any
	if folderID != nil {
		target := strings.TrimSpace(*folderID)
		if target == "" {
			return ErrFolderNotFound
		}
		folderUser, err := folderOwner(db, target)
		if err != nil {
			return err
		}
		if folderUser != userID {
			return ErrFolderNotFound
		}
		value = target
	}

	_, err = db.Exec(`UPDATE conversations SET folder_id = ? WHERE id = ?`, value, id)
	return err
}

func folderOwner(db *sql.DB, id string) (int, error) {
	var userID int
	err := db.QueryRow(`SELECT user_id FROM chat_folders WHERE id = ? LIMIT 1`, id).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrFolderNotFound
	}
	return userID, err
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
