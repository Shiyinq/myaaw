package channel

import (
	"encoding/json"
	"myaaw/internal/provider"
)

type IncomingMessage struct {
	UserID     int      // unified user identifier
	SenderName string   // optional display name for new users (e.g. "a2a:alice")
	Text       string   // text content (or transcribed voice)
	Images     []string // base64 encoded images
	Voice      []byte   // raw audio bytes
	// Files are user-uploaded documents. They are persisted to disk and the
	// model receives their paths rather than the raw bytes.
	Files          []FileUpload
	Channel        string // "telegram", "api", etc.
	ReplyTo        string // quoted/replied text context
	RawMeta        any    // channel-specific metadata (e.g. chat ID, message ID)
	TriggerType    string // "cron", "heartbeat", or empty
	ConversationID string // specific conversation/session ID (optional)
}

type OutgoingMessage struct {
	Text           string
	Trace          []provider.ReactStep
	Usage          provider.Usage
	Thought        string
	ConversationID string
	// Attachments are files produced by the run (for example screenshots
	// requested with deliver=true). Channels send them after the text.
	Attachments []Attachment
	// RunID identifies the recorded trace for this message. It is empty when
	// observability is disabled.
	RunID string
	// Stopped reports that the client stopped the run. The text carries
	// whatever the model managed to produce before the stop.
	Stopped bool
}

// FileUpload is one uploaded file carried by a chat message.
type FileUpload struct {
	Name       string `json:"name"`
	MimeType   string `json:"mime_type,omitempty"`
	DataBase64 string `json:"data_base64"`
}

// Attachment is one file produced by a tool and delivered to the user.
type Attachment struct {
	FileName string
	MimeType string
	Data     []byte
}

type StreamChunk struct {
	// Kind distinguishes progress events: "step", "text", "thought", "tool",
	// "tool_result", "trace", "usage", "conversation", or "attachment".
	// Empty means plain text for backwards compatibility.
	Kind string
	// StepIndex is the 1-based loop iteration for Kind "step".
	StepIndex      int
	Text           string
	ToolCalls      []provider.ToolCall
	ToolCallID     string
	ToolName       string
	ToolArguments  string
	ToolResultText string
	IsError        bool
	Trace          []provider.ReactStep
	Usage          provider.Usage
	Thought        string
	// Attempt is set on Kind "retry".
	Attempt int
	// ConversationID is set on Kind "conversation" so a client knows the id
	// before the run finishes (a stopped run never sends "done").
	ConversationID string
	// Approval fields are set on Kind "approval_required" and
	// "approval_resolved".
	ApprovalID  string
	ToolAction  string
	ArgsSummary string
	// Args is the full raw argument JSON for approval requests.
	Args     string
	Decision string
	Reason   string
	// SubAgent names the sub-agent that raised the approval request, empty for
	// calls made by the main agent.
	SubAgent string
	// Attachments are files emitted by the run (Kind "attachment").
	Attachments []Attachment
	// Compaction numbers are set on Kind "compaction".
	TokensBefore    int
	TokensAfter     int
	DroppedMessages int
}

type QueueEnvelope struct {
	Channel string          `json:"channel"`
	Payload json.RawMessage `json:"payload"`
}
