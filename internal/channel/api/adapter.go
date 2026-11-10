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
	UserID int    `json:"user_id"`
	Text   string `json:"text"`
	// RequestID lets the client stop this run with POST /api/chat/stop.
	RequestID      string               `json:"request_id,omitempty"`
	Images         []string             `json:"images,omitempty"`
	Files          []channel.FileUpload `json:"files,omitempty"`
	ConversationID string               `json:"conversation_id,omitempty"`
	// RewindUserOffset rewinds the transcript before this turn. 0 keeps the
	// transcript up to the last user message (regenerate); N keeps it up to the
	// user message N turns earlier (edit that message with `text`).
	RewindUserOffset *int `json:"rewind_user_offset,omitempty"`
}

func (a *APIAdapter) ParseIncoming(payload json.RawMessage) (*channel.IncomingMessage, error) {
	var req ChatRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to parse API payload: %w", err)
	}

	// user_id is optional. Direct agent API calls default to 0; the control
	// plane injects its configured user ID before proxying chat requests.

	if req.Text == "" && len(req.Images) == 0 && len(req.Files) == 0 && req.RewindUserOffset == nil {
		return nil, fmt.Errorf("text, images, or files are required")
	}
	if req.RewindUserOffset != nil && *req.RewindUserOffset < 0 {
		return nil, fmt.Errorf("rewind_user_offset must be zero or greater")
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

	if err := validateFiles(req.Files); err != nil {
		return nil, err
	}

	conversationID := req.ConversationID
	if conversationID == "" {
		// API clients must opt in to an existing session; without an ID each
		// request starts a new conversation.
		conversationID = "NEW"
	}

	trigger := ""
	if req.RewindUserOffset != nil {
		// Label the observability run so edits and regenerations are easy to find.
		if strings.TrimSpace(req.Text) != "" {
			trigger = "edit"
		} else {
			trigger = "regenerate"
		}
	}

	return &channel.IncomingMessage{
		UserID:           req.UserID,
		Text:             req.Text,
		Images:           req.Images,
		Files:            req.Files,
		Channel:          "api",
		ConversationID:   conversationID,
		RewindUserOffset: req.RewindUserOffset,
		TriggerType:      trigger,
		RawMeta:          APIMeta{},
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

const (
	// maxFiles caps the uploads of one chat request.
	maxFiles = 8
	// maxFileBytes and maxTotalFileBytes bound a single file and a whole
	// request so a chat cannot write unbounded data into the agent folder.
	maxFileBytes      = 10 << 20
	maxTotalFileBytes = 20 << 20
)

var fileExtensions = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".csv": true, ".tsv": true,
	".json": true, ".xml": true, ".yaml": true, ".yml": true, ".log": true,
	".rtf": true, ".pdf": true, ".doc": true, ".docx": true, ".xls": true,
	".xlsx": true, ".ppt": true, ".pptx": true, ".odt": true, ".ods": true,
	".odp": true,
}

// validateFiles decodes, bounds and sanitizes uploads in place. The agent
// stores files but never sends their bytes to a model, so the checks here only
// need to prevent bad names and unbounded writes.
func validateFiles(files []channel.FileUpload) error {
	if len(files) == 0 {
		return nil
	}
	if len(files) > maxFiles {
		return fmt.Errorf("at most %d files are allowed", maxFiles)
	}

	var total int64
	for i := range files {
		file := &files[i]
		name := sanitizeFileName(file.Name)
		if name == "" {
			return fmt.Errorf("file name is required")
		}
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(file.DataBase64))
		if err != nil || len(data) == 0 {
			return fmt.Errorf("file %q must be base64-encoded", name)
		}
		if int64(len(data)) > maxFileBytes {
			return fmt.Errorf("file %q exceeds %d MB", name, maxFileBytes>>20)
		}
		total += int64(len(data))
		if total > maxTotalFileBytes {
			return fmt.Errorf("total upload size exceeds %d MB", maxTotalFileBytes>>20)
		}
		if !allowedFile(name, file.MimeType) {
			return fmt.Errorf("unsupported file type: %q", name)
		}
		file.Name = name
	}
	return nil
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	parts := strings.Split(name, "/")
	name = parts[len(parts)-1]
	if name == "" || name == "." || name == ".." {
		return ""
	}
	return name
}

func allowedFile(name, mime string) bool {
	if mime != "" {
		if strings.HasPrefix(mime, "text/") || mime == "application/json" ||
			mime == "application/xml" || mime == "application/pdf" {
			return true
		}
	}
	dot := strings.LastIndex(name, ".")
	if dot < 0 {
		return false
	}
	return fileExtensions[strings.ToLower(name[dot:])]
}
