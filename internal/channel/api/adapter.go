package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"myaaw/internal/channel"
)

// maxImages caps one API chat request. The dashboard enforces the same limit;
// this keeps direct API callers honest too.
const maxImages = 6

type APIMeta struct{}

type APIAdapter struct{}

func NewAPIAdapter() *APIAdapter {
	return &APIAdapter{}
}

func (a *APIAdapter) Name() string {
	return "api"
}

type ChatRequest struct {
	UserID         int      `json:"user_id"`
	Text           string   `json:"text"`
	Images         []string `json:"images,omitempty"`
	ConversationID string   `json:"conversation_id,omitempty"`
}

func (a *APIAdapter) ParseIncoming(payload json.RawMessage) (*channel.IncomingMessage, error) {
	var req ChatRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to parse API payload: %w", err)
	}

	// user_id is optional. Direct agent API calls default to 0; the control
	// plane injects its configured user ID before proxying chat requests.

	if req.Text == "" && len(req.Images) == 0 {
		return nil, fmt.Errorf("text or images are required")
	}
	if len(req.Images) > maxImages {
		return nil, fmt.Errorf("at most %d images are allowed", maxImages)
	}
	// Images arrive as raw base64. Reject anything that does not decode to an
	// image so a video or arbitrary blob can never reach the provider.
	for _, image := range req.Images {
		data, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(image))
		if decodeErr != nil || len(data) == 0 {
			return nil, fmt.Errorf("images must be base64-encoded")
		}
		if !strings.HasPrefix(http.DetectContentType(data), "image/") {
			return nil, fmt.Errorf("images must be image data")
		}
	}

	conversationID := req.ConversationID
	if conversationID == "" {
		// API clients must opt in to an existing session; without an ID each
		// request starts a new conversation.
		conversationID = "NEW"
	}

	return &channel.IncomingMessage{
		UserID:         req.UserID,
		Text:           req.Text,
		Images:         req.Images,
		Channel:        "api",
		ConversationID: conversationID,
		RawMeta:        APIMeta{},
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
