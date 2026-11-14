package channel

import (
	"encoding/json"
	"strings"
)

// interactiveChannels lists the chat platforms where a user can send another
// message while the bot is answering. Everything else (API, CLI, A2A, cron) has
// its own stop path, so its turns are never cancelled by a newer message.
//
// A new chat channel opts in by adding its adapter name here; the batching and
// stop pipeline in cmd/myaaw and the bot service is channel-agnostic, so no other
// change is needed for the same behaviour.
var interactiveChannels = map[string]struct{}{
	"telegram": {},
	"discord":  {},
}

// Interactive reports whether messages from this channel come from a chat
// platform where a user can send another message while the bot is answering,
// and can therefore stop or interrupt the running turn. Scheduled turns are
// excluded by their trigger type, not by the channel.
func Interactive(name string) bool {
	_, ok := interactiveChannels[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

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
