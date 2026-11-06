// Package events is a small in-process fan-out for conversation updates.
//
// The agent server publishes a turn after it is persisted; SSE clients
// subscribed through the control plane receive a "conversation.updated" event
// and re-read the authoritative transcript. The broker is process-wide on
// purpose: one agent process serves one agent, so no routing table is needed.
package events

import "sync"

// TurnEvent says one conversation gained a persisted turn.
type TurnEvent struct {
	ConversationID string `json:"conversation_id"`
	Trigger        string `json:"trigger"`
	At             string `json:"at"`
}

// Broker fans one event out to every subscriber without blocking the publisher.
type Broker struct {
	mu   sync.Mutex
	next int
	subs map[int]chan TurnEvent
}

func newBroker() *Broker {
	return &Broker{subs: make(map[int]chan TurnEvent)}
}

// Subscribe returns a channel of events and a cancel function. A subscriber
// that falls behind loses events instead of stalling a turn; it re-syncs by
// re-reading the transcript when it reconnects.
func (b *Broker) Subscribe() (<-chan TurnEvent, func()) {
	b.mu.Lock()
	id := b.next
	b.next++
	ch := make(chan TurnEvent, 16)
	b.subs[id] = ch
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if existing, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(existing)
		}
		b.mu.Unlock()
	}
	return ch, cancel
}

// Publish delivers the event to every subscriber. An empty conversation id is
// ignored: nothing could be re-read from it.
func (b *Broker) Publish(event TurnEvent) {
	if event.ConversationID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

var defaultBroker = newBroker()

// Default is the process-wide broker shared by the bot service (publisher) and
// the SSE handler (subscriber).
func Default() *Broker { return defaultBroker }
