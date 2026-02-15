package api

import (
	"encoding/json"
	"fmt"
	"myaaw/internal/channel"
)

type APIMeta struct{}

type APIAdapter struct{}

func NewAPIAdapter() *APIAdapter {
	return &APIAdapter{}
}

func (a *APIAdapter) Name() string {
	return "api"
}

type ChatRequest struct {
	UserID int      `json:"user_id"`
	Text   string   `json:"text"`
	Images []string `json:"images,omitempty"`
}

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

func (a *APIAdapter) Send(msg *channel.IncomingMessage, out *channel.OutgoingMessage) error {
	return nil
}

func (a *APIAdapter) SendStream(msg *channel.IncomingMessage, streamFn func(onChunk func(chunk channel.StreamChunk)) error) (*channel.OutgoingMessage, error) {
	return nil, nil
}

func (a *APIAdapter) SendError(msg *channel.IncomingMessage, errText string) error {
	return nil
}
