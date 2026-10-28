package channel

import (
	"encoding/json"
	"myaaw/internal/provider"
)

type IncomingMessage struct {
	UserID         int      // unified user identifier
	SenderName     string   // optional display name for new users (e.g. "a2a:alice")
	Text           string   // text content (or transcribed voice)
	Images         []string // base64 encoded images
	Voice          []byte   // raw audio bytes
	Channel        string   // "telegram", "api", etc.
	ReplyTo        string   // quoted/replied text context
	RawMeta        any      // channel-specific metadata (e.g. chat ID, message ID)
	TriggerType    string   // "cron", "heartbeat", or empty
	ConversationID string   // specific conversation/session ID (optional)
}

type OutgoingMessage struct {
	Text           string
	Trace          []provider.ReactStep
	Usage          provider.Usage
	Thought        string
	ConversationID string
}

type StreamChunk struct {
	// Kind distinguishes progress events: "step", "text", "thought", "tool",
	// "tool_result", "trace", or "usage". Empty means plain text for
	// backwards compatibility.
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
}

type QueueEnvelope struct {
	Channel string          `json:"channel"`
	Payload json.RawMessage `json:"payload"`
}
