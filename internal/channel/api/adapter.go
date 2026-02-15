package api

import (
	"encoding/json"
	"fmt"
	"myaaw/internal/channel"
)

// APIMeta holds API-specific metadata (currently minimal).
type APIMeta struct {
	// Could hold API key, session ID, etc. in the future
}

// APIAdapter implements channel.Adapter for REST API.
type APIAdapter struct{}

// NewAPIAdapter creates a new API channel adapter.
func NewAPIAdapter() *APIAdapter {
	return &APIAdapter{}
}

func (a *APIAdapter) Name() string {
	return "api"
}

// ChatRequest is the JSON request body for REST API chat.
type ChatRequest struct {
	UserID int      `json:"user_id"`
	Text   string   `json:"text"`
	Images []string `json:"images,omitempty"`
}

// ParseIncoming converts a raw API JSON payload into a generic IncomingMessage.
func (a *APIAdapter) ParseIncoming(payload json.RawMessage) (*channel.IncomingMessage, error) {
	var req ChatRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to parse API payload: %w", err)
	}

	if req.UserID == 0 {
		return nil, fmt.Errorf("user_id is required")
	}

	if req.Text == "" {
		return nil, fmt.Errorf("text is required")
	}

	return &channel.IncomingMessage{
		UserID:  req.UserID,
		Text:    req.Text,
		Images:  req.Images,
		Channel: "api",
		RawMeta: APIMeta{},
	}, nil
}

// Send is a no-op for API channel — the handler returns JSON directly.
func (a *APIAdapter) Send(msg *channel.IncomingMessage, out *channel.OutgoingMessage) error {
	return nil
}

// SendStream is a no-op for API channel — the handler handles SSE directly.
func (a *APIAdapter) SendStream(msg *channel.IncomingMessage, streamFn func(onChunk func(chunk channel.StreamChunk)) error) (*channel.OutgoingMessage, error) {
	return nil, nil
}

// SendError is a no-op for API channel — the handler returns error JSON directly.
func (a *APIAdapter) SendError(msg *channel.IncomingMessage, errText string) error {
	return nil
}
