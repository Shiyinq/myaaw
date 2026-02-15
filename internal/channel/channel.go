package channel

import "encoding/json"

// Adapter defines the interface that each channel must implement.
type Adapter interface {
	// Name returns the channel identifier (e.g. "telegram", "api").
	Name() string

	// ParseIncoming converts channel-specific raw payload into a generic IncomingMessage.
	ParseIncoming(payload json.RawMessage) (*IncomingMessage, error)

	// Send delivers a non-streaming response back to the user.
	Send(msg *IncomingMessage, out *OutgoingMessage) error

	// SendStream delivers a streaming response to the user.
	// streamFn is called with an onChunk callback that the adapter uses to receive partial updates.
	// The adapter is responsible for implementing channel-specific streaming behavior
	// (e.g. Telegram edits messages, REST API sends SSE events).
	SendStream(msg *IncomingMessage, streamFn func(onChunk func(chunk StreamChunk)) error) (*OutgoingMessage, error)

	// SendError delivers an error message to the user.
	SendError(msg *IncomingMessage, errText string) error
}

// Registry holds all registered channel adapters.
type Registry struct {
	adapters map[string]Adapter
}

// NewRegistry creates a new empty adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

// Register adds a channel adapter to the registry.
func (r *Registry) Register(adapter Adapter) {
	r.adapters[adapter.Name()] = adapter
}

// Get returns the adapter for the given channel name, or nil if not found.
func (r *Registry) Get(name string) Adapter {
	return r.adapters[name]
}
