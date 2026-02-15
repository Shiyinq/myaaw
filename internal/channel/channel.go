package channel

import "encoding/json"

type Adapter interface {
	Name() string
	ParseIncoming(payload json.RawMessage) (*IncomingMessage, error)
	Send(msg *IncomingMessage, out *OutgoingMessage) error
	SendStream(msg *IncomingMessage, streamFn func(onChunk func(chunk StreamChunk)) error) (*OutgoingMessage, error)
	SendError(msg *IncomingMessage, errText string) error
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

func (r *Registry) Register(adapter Adapter) {
	r.adapters[adapter.Name()] = adapter
}

func (r *Registry) Get(name string) Adapter {
	return r.adapters[name]
}
