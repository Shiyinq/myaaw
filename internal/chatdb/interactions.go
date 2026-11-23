package chatdb

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

// InteractionCallback stores the target metadata behind a short platform
// callback key (e.g. Telegram `em:<key>` or Discord `em:<key>`).
type InteractionCallback struct {
	Key            string
	ConversationID string
	UserID         int
	Channel        string
	BlockID        string
	Kind           string
	ActionID       string
	CardID         string
	Label          string
	Value          string
	MetaJSON       string
	CreatedAt      time.Time
}

// BlockInteractionState mirrors provider.InteractionState without introducing
// an import cycle between chatdb (used by config) and provider.
type BlockInteractionState struct {
	Status     string         `json:"status"`
	AnsweredAt string         `json:"answered_at,omitempty"`
	Actor      string         `json:"actor,omitempty"`
	Summary    string         `json:"summary,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
}

type storedExtraBlock struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Interactive bool                   `json:"interactive,omitempty"`
	SingleUse   *bool                  `json:"single_use,omitempty"`
	Data        json.RawMessage        `json:"data"`
	State       *BlockInteractionState `json:"state,omitempty"`
}

func (b storedExtraBlock) isSingleUse() bool {
	if b.SingleUse == nil {
		return b.Interactive
	}
	return *b.SingleUse
}

type storedExtraMessage struct {
	ID        string             `json:"id"`
	Blocks    []storedExtraBlock `json:"blocks"`
	CreatedAt string             `json:"created_at,omitempty"`
}

var interactionStateMu sync.Mutex

func ensureInteractionCallbacksTable(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS interaction_callbacks (
		key TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL DEFAULT '',
		user_id INTEGER NOT NULL DEFAULT 0,
		channel TEXT NOT NULL DEFAULT '',
		block_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		action_id TEXT NOT NULL DEFAULT '',
		card_id TEXT NOT NULL DEFAULT '',
		label TEXT NOT NULL DEFAULT '',
		value TEXT NOT NULL DEFAULT '',
		meta_json TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL
	)`); err != nil {
		return err
	}
	_, err := db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_interaction_callbacks_created ON interaction_callbacks(created_at)`,
	)
	return err
}

// SaveInteractionCallback inserts a callback record with a generated 8-char hex
// key (or reuses cb.Key when supplied) and returns the key.
func SaveInteractionCallback(db *sql.DB, cb InteractionCallback) (string, error) {
	if db == nil {
		return "", errors.New("database is nil")
	}
	if err := ensureInteractionCallbacksTable(db); err != nil {
		return "", err
	}
	key := strings.TrimSpace(cb.Key)
	if key == "" {
		var buf [4]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return "", err
		}
		key = hex.EncodeToString(buf[:])
	}
	now := time.Now().UTC()
	if cb.CreatedAt.IsZero() {
		cb.CreatedAt = now
	}

	_, err := db.Exec(`INSERT OR REPLACE INTO interaction_callbacks
		(key, conversation_id, user_id, channel, block_id, kind, action_id, card_id, label, value, meta_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key,
		strings.TrimSpace(cb.ConversationID),
		cb.UserID,
		strings.TrimSpace(cb.Channel),
		strings.TrimSpace(cb.BlockID),
		strings.TrimSpace(cb.Kind),
		strings.TrimSpace(cb.ActionID),
		strings.TrimSpace(cb.CardID),
		cb.Label,
		cb.Value,
		cb.MetaJSON,
		cb.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return "", err
	}

	// Best-effort prune of callbacks older than 30 days.
	cutoff := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339Nano)
	_, _ = db.Exec(`DELETE FROM interaction_callbacks WHERE created_at < ?`, cutoff)
	return key, nil
}

// GetInteractionCallback retrieves a callback record by its short key.
func GetInteractionCallback(db *sql.DB, key string) (InteractionCallback, bool, error) {
	if db == nil {
		return InteractionCallback{}, false, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return InteractionCallback{}, false, nil
	}
	if err := ensureInteractionCallbacksTable(db); err != nil {
		return InteractionCallback{}, false, err
	}

	var (
		cb         InteractionCallback
		createdRaw string
	)
	err := db.QueryRow(`SELECT key, conversation_id, user_id, channel, block_id, kind, action_id, card_id, label, value, meta_json, created_at
		FROM interaction_callbacks WHERE key = ? LIMIT 1`, key,
	).Scan(
		&cb.Key,
		&cb.ConversationID,
		&cb.UserID,
		&cb.Channel,
		&cb.BlockID,
		&cb.Kind,
		&cb.ActionID,
		&cb.CardID,
		&cb.Label,
		&cb.Value,
		&cb.MetaJSON,
		&createdRaw,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return InteractionCallback{}, false, nil
	}
	if err != nil {
		return InteractionCallback{}, false, err
	}
	if t, err := time.Parse(time.RFC3339Nano, createdRaw); err == nil {
		cb.CreatedAt = t
	}
	return cb, true, nil
}

// MarkBlockSubmitted atomically updates an interactive extra_messages block's
// state in conversations.db. If the block is single-use and already submitted,
// alreadySubmitted is true and the database is left unchanged.
//
// Because all channels share conversations.db, marking a block submitted from
// Telegram or Discord immediately synchronizes the locked state with the WebUI.
func MarkBlockSubmitted(
	db *sql.DB,
	conversationID string,
	blockID string,
	actor string,
	summary string,
	payload map[string]any,
) (matchedConvID string, alreadySubmitted bool, state BlockInteractionState, err error) {
	if db == nil || strings.TrimSpace(blockID) == "" {
		return "", false, BlockInteractionState{}, nil
	}

	interactionStateMu.Lock()
	defer interactionStateMu.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return "", false, BlockInteractionState{}, err
	}
	defer func() { _ = tx.Rollback() }()

	type convRow struct {
		id           string
		messagesJSON string
	}
	var candidates []convRow

	if id := strings.TrimSpace(conversationID); id != "" {
		var raw string
		err := tx.QueryRow(`SELECT messages FROM conversations WHERE id = ? LIMIT 1`, id).Scan(&raw)
		if err == nil {
			candidates = append(candidates, convRow{id: id, messagesJSON: raw})
		} else if !errors.Is(err, sql.ErrNoRows) {
			return "", false, BlockInteractionState{}, err
		}
	}

	if len(candidates) == 0 {
		rows, qErr := tx.Query(`SELECT id, messages FROM conversations ORDER BY updated_at DESC LIMIT 25`)
		if qErr != nil {
			return "", false, BlockInteractionState{}, qErr
		}
		for rows.Next() {
			var r convRow
			if sErr := rows.Scan(&r.id, &r.messagesJSON); sErr == nil {
				if strings.Contains(r.messagesJSON, blockID) {
					candidates = append(candidates, r)
				}
			}
		}
		_ = rows.Close()
	}

	for _, cand := range candidates {
		// Decode each message as a raw field map so we preserve 100% of all
		// provider.Message fields untouched while updating only extra_messages.
		var rawMessages []map[string]json.RawMessage
		if uErr := json.Unmarshal([]byte(cand.messagesJSON), &rawMessages); uErr != nil {
			continue
		}

		matched := false
		for i := len(rawMessages) - 1; i >= 0 && !matched; i-- {
			rawExtras, ok := rawMessages[i]["extra_messages"]
			if !ok || len(rawExtras) == 0 {
				continue
			}
			var extras []storedExtraMessage
			if json.Unmarshal(rawExtras, &extras) != nil {
				continue
			}
			for j := range extras {
				for k := range extras[j].Blocks {
					blk := &extras[j].Blocks[k]
					if blk.ID != blockID {
						continue
					}
					matched = true
					if blk.isSingleUse() && blk.State != nil && blk.State.Status == "submitted" {
						return cand.id, true, *blk.State, nil
					}
					nextState := BlockInteractionState{
						Status:     "submitted",
						AnsweredAt: time.Now().UTC().Format(time.RFC3339),
						Actor:      actor,
						Summary:    strings.TrimSpace(summary),
						Payload:    payload,
					}
					blk.State = &nextState
					state = nextState
					break
				}
			}
			if matched {
				encodedExtras, mErr := json.Marshal(extras)
				if mErr != nil {
					return "", false, BlockInteractionState{}, mErr
				}
				rawMessages[i]["extra_messages"] = encodedExtras
			}
		}
		if !matched {
			continue
		}

		encoded, mErr := json.Marshal(rawMessages)
		if mErr != nil {
			return "", false, BlockInteractionState{}, mErr
		}
		if _, eErr := tx.Exec(`UPDATE conversations SET messages = ? WHERE id = ?`, string(encoded), cand.id); eErr != nil {
			return "", false, BlockInteractionState{}, eErr
		}
		if cErr := tx.Commit(); cErr != nil {
			return "", false, BlockInteractionState{}, cErr
		}
		return cand.id, false, state, nil
	}

	return "", false, BlockInteractionState{}, nil
}
