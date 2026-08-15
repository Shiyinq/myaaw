package channel

import (
	"encoding/json"
	"myaaw/internal/provider"
)

type IncomingMessage struct {
	UserID         int      // unified user identifier
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
	Text    string
	Trace   []provider.ReactStep
	Usage   provider.Usage
	Thought string
}

type StreamChunk struct {
	Text      string
	ToolCalls []provider.ToolCall
	Trace     []provider.ReactStep
	Usage     provider.Usage
	Thought   string
}

type QueueEnvelope struct {
	Channel string          `json:"channel"`
	Payload json.RawMessage `json:"payload"`
}
