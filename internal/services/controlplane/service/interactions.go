package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"myaaw/internal/provider"
)

var (
	ErrInteractionAlreadySubmitted = errors.New("interaction already submitted")
	ErrInteractionNotFound         = errors.New("interaction block not found")
	ErrInteractionInvalid          = errors.New("invalid interaction request")
)

// InteractionRequest is sent by a client when the user clicks an interactive
// button/quick-reply or submits a dynamic form/checklist from extra_messages.
type InteractionRequest struct {
	MessageID   string         `json:"message_id,omitempty"`
	BlockID     string         `json:"block_id"`
	Kind        string         `json:"kind,omitempty"` // button | quick_reply | form | checklist
	ActionID    string         `json:"action_id,omitempty"`
	CardID      string         `json:"card_id,omitempty"`
	Label       string         `json:"label,omitempty"`
	Value       string         `json:"value,omitempty"`
	Values      map[string]any `json:"values,omitempty"`
	DisplayText string         `json:"display_text,omitempty"`
}

// InteractionResponse is returned by POST /agents/:name/conversations/:id/interactions.
type InteractionResponse struct {
	OK              bool                      `json:"ok"`
	BlockID         string                    `json:"block_id"`
	State           provider.InteractionState `json:"state"`
	InteractionText string                    `json:"interaction_text"`
}

// GroupInteractionResponse is returned by POST /groups/:id/interactions.
type GroupInteractionResponse struct {
	OK       bool                      `json:"ok"`
	BlockID  string                    `json:"block_id"`
	State    provider.InteractionState `json:"state"`
	Message  GroupMessage              `json:"message"`
	RunState GroupRunState             `json:"run_state"`
}

var conversationInteractionMu sync.Mutex

// RespondInteraction atomically records a user's interaction against an
// extra_messages block in a DM conversation and returns the formatted
// <interaction> message text to send as the follow-up turn.
func (s *ConversationService) RespondInteraction(agentName, id string, userID int, req InteractionRequest) (InteractionResponse, error) {
	req.BlockID = strings.TrimSpace(req.BlockID)
	if req.BlockID == "" {
		return InteractionResponse{}, fmt.Errorf("%w: block_id is required", ErrInteractionInvalid)
	}

	conversationInteractionMu.Lock()
	defer conversationInteractionMu.Unlock()

	db, err := openAgentDB(agentName)
	if err != nil {
		return InteractionResponse{}, err
	}
	if db == nil {
		return InteractionResponse{}, ErrConversationNotFound
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return InteractionResponse{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		owner        int
		messagesJSON string
	)
	err = tx.QueryRow(
		`SELECT user_id, messages FROM conversations WHERE id = ? LIMIT 1`, id,
	).Scan(&owner, &messagesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return InteractionResponse{}, ErrConversationNotFound
	}
	if err != nil {
		return InteractionResponse{}, err
	}
	if owner != userID {
		return InteractionResponse{}, ErrConversationForbidden
	}

	var messages []provider.Message
	if err := json.Unmarshal([]byte(messagesJSON), &messages); err != nil {
		return InteractionResponse{}, err
	}

	var (
		foundState      provider.InteractionState
		interactionText string
		matched         bool
	)

	// Search newest-first so we find the target block quickly.
	for i := len(messages) - 1; i >= 0; i-- {
		if req.MessageID != "" && messages[i].ID != "" && messages[i].ID != req.MessageID {
			continue
		}
		if len(messages[i].ExtraMessages) == 0 {
			continue
		}
		st, text, ok, applyErr := applyBlockInteraction(messages[i].ExtraMessages, req, "web")
		if applyErr != nil {
			return InteractionResponse{BlockID: req.BlockID, State: st}, applyErr
		}
		if ok {
			foundState = st
			interactionText = text
			matched = true
			break
		}
	}
	if !matched {
		return InteractionResponse{}, ErrInteractionNotFound
	}

	encoded, err := json.Marshal(messages)
	if err != nil {
		return InteractionResponse{}, err
	}
	if _, err := tx.Exec(
		`UPDATE conversations SET messages = ? WHERE id = ? AND user_id = ?`,
		string(encoded), id, userID,
	); err != nil {
		return InteractionResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return InteractionResponse{}, err
	}

	return InteractionResponse{
		OK:              true,
		BlockID:         req.BlockID,
		State:           foundState,
		InteractionText: interactionText,
	}, nil
}

// RespondInteraction atomically locks an extra_messages block on a group
// message, broadcasts the updated state to all open tabs, and dispatches a
// follow-up user turn replying directly to the agent that sent the block.
func (s *GroupService) RespondInteraction(groupID string, userID int, req InteractionRequest) (GroupInteractionResponse, error) {
	groupID = strings.Clone(strings.TrimSpace(groupID))
	req.MessageID = strings.Clone(strings.TrimSpace(req.MessageID))
	req.BlockID = strings.Clone(strings.TrimSpace(req.BlockID))
	if req.MessageID == "" || req.BlockID == "" {
		return GroupInteractionResponse{}, fmt.Errorf("%w: message_id and block_id are required", ErrInteractionInvalid)
	}

	state, interactionText, err := s.store.ApplyExtraBlockInteraction(groupID, req, "web")
	if err != nil {
		return GroupInteractionResponse{BlockID: req.BlockID, State: state}, err
	}

	// Broadcast the updated block state to every open client immediately so
	// concurrent tabs lock the button/form in real time.
	if sg, getErr := s.store.Get(groupID); getErr == nil {
		rt := s.runtimeFor(groupID, sg.Group.MaxTurns)
		rt.broadcast(GroupSSEEvent{
			Name: "extra_block_updated",
			Data: map[string]any{
				"group_id":   groupID,
				"message_id": req.MessageID,
				"block_id":   req.BlockID,
				"state":      state,
			},
		})
	}

	userMsg, runState, err := s.SendMessage(groupID, userID, SendGroupMessageRequest{
		Text:      interactionText,
		ReplyToID: req.MessageID,
	})
	if err != nil {
		return GroupInteractionResponse{
			OK:      true,
			BlockID: req.BlockID,
			State:   state,
		}, err
	}

	return GroupInteractionResponse{
		OK:       true,
		BlockID:  req.BlockID,
		State:    state,
		Message:  userMsg,
		RunState: runState,
	}, nil
}

func applyBlockInteraction(extras []provider.ExtraMessage, req InteractionRequest, actor string) (provider.InteractionState, string, bool, error) {
	for i := range extras {
		for j := range extras[i].Blocks {
			block := &extras[i].Blocks[j]
			if block.ID != req.BlockID {
				continue
			}
			if !block.Interactive {
				return provider.InteractionState{}, "", true, fmt.Errorf("%w: block %q is display-only", ErrInteractionInvalid, req.BlockID)
			}
			if block.IsSingleUse() && block.State != nil && block.State.Status == "submitted" {
				return *block.State, "", true, ErrInteractionAlreadySubmitted
			}

			kind := inferInteractionKind(block.Type, req.Kind)
			summary, payload := buildInteractionSummaryAndPayload(block, kind, req)
			now := time.Now().UTC().Format(time.RFC3339)
			state := provider.InteractionState{
				Status:     "submitted",
				AnsweredAt: now,
				Actor:      actor,
				Summary:    summary,
				Payload:    payload,
			}
			block.State = &state
			interactionText := provider.InteractionBlock(kind, payload, summary)
			return state, interactionText, true, nil
		}
	}
	return provider.InteractionState{}, "", false, nil
}

func inferInteractionKind(blockType, requested string) string {
	if provider.IsInteractionKind(requested) {
		return strings.ToLower(strings.TrimSpace(requested))
	}
	switch strings.ToLower(strings.TrimSpace(blockType)) {
	case "quick_replies":
		return provider.InteractionQuickReply
	case "form":
		return provider.InteractionForm
	case "checklist":
		return provider.InteractionChecklist
	default:
		return provider.InteractionButton
	}
}

func buildInteractionSummaryAndPayload(block *provider.ExtraBlock, kind string, req InteractionRequest) (string, map[string]any) {
	payload := map[string]any{
		"block_id":   block.ID,
		"block_type": block.Type,
	}
	if req.CardID != "" {
		payload["card_id"] = req.CardID
	}
	if req.ActionID != "" {
		payload["action_id"] = req.ActionID
	}
	if req.Label != "" {
		payload["label"] = req.Label
	}
	if req.Value != "" {
		payload["value"] = req.Value
	}
	if len(req.Values) > 0 {
		payload["values"] = req.Values
	}

	if custom := strings.TrimSpace(req.DisplayText); custom != "" {
		return custom, payload
	}

	switch kind {
	case provider.InteractionForm:
		title := extractStringField(block.Data, "title")
		if title != "" {
			payload["title"] = title
		}
		var b strings.Builder
		if title != "" {
			fmt.Fprintf(&b, "Submitted \"%s\"", title)
		} else {
			b.WriteString("Submitted form")
		}
		if len(req.Values) > 0 {
			keys := make([]string, 0, len(req.Values))
			for k := range req.Values {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "\n- %s: %v", k, formatInteractionValue(req.Values[k]))
			}
		}
		return b.String(), payload
	case provider.InteractionChecklist:
		title := extractStringField(block.Data, "title")
		var b strings.Builder
		if title != "" {
			fmt.Fprintf(&b, "Updated \"%s\"", title)
		} else {
			b.WriteString("Updated checklist")
		}
		if checked, ok := req.Values["checked"]; ok {
			fmt.Fprintf(&b, ": %v", formatInteractionValue(checked))
		}
		return b.String(), payload
	default:
		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = strings.TrimSpace(req.Value)
		}
		if label == "" {
			label = strings.TrimSpace(req.ActionID)
		}
		return label, payload
	}
}

func extractStringField(raw json.RawMessage, field string) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if s, ok := m[field].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func formatInteractionValue(v any) string {
	switch val := v.(type) {
	case []any:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ", ")
	case []string:
		return strings.Join(val, ", ")
	case bool:
		if val {
			return "Yes"
		}
		return "No"
	default:
		return fmt.Sprint(val)
	}
}
