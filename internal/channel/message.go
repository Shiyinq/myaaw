package channel

import (
	"encoding/json"
	"myaaw/internal/provider"
)

// IncomingMessage is a channel-agnostic representation of an incoming user message.
type IncomingMessage struct {
	UserID  int      // unified user identifier
	Text    string   // text content (or transcribed voice)
	Images  []string // base64 encoded images
	Voice   []byte   // raw audio bytes
	Channel string   // "telegram", "api", etc.
	ReplyTo string   // quoted/replied text context
	RawMeta any      // channel-specific metadata (e.g. chat ID, message ID)
}

// OutgoingMessage is a channel-agnostic representation of a bot response.
type OutgoingMessage struct {
	Text  string
	Trace []provider.ReactStep
	Usage provider.Usage
}

// StreamChunk represents a partial streaming response from the bot.
type StreamChunk struct {
	Text      string
	ToolCalls []provider.ToolCall
	Trace     []provider.ReactStep
	Usage     provider.Usage
}

// QueueEnvelope wraps a channel-specific payload with routing info for RabbitMQ.
type QueueEnvelope struct {
	Channel string          `json:"channel"`
	Payload json.RawMessage `json:"payload"`
}
