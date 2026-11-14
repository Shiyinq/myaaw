package main

import (
	"encoding/json"
	"sync"
	"time"

	"myaaw/internal/channel"
)

// messageBatcher collects the plain chat messages of one conversation for a
// short quiet window. A spam burst becomes one agent turn that still contains
// every message, so nothing is lost and the agent answers once.
type messageBatcher struct {
	window time.Duration
	now    func() time.Time

	mu      sync.Mutex
	pending map[string]*pendingBatch
}

type pendingBatch struct {
	channel  string
	payloads []json.RawMessage
	lastAt   time.Time
}

// readyBatch is one flushed conversation: the dispatcher key and the envelope
// carrying every message collected for it.
type readyBatch struct {
	key      string
	envelope []byte
}

func newMessageBatcher(window time.Duration) *messageBatcher {
	if window <= 0 {
		window = time.Second
	}
	return &messageBatcher{
		window:  window,
		now:     time.Now,
		pending: make(map[string]*pendingBatch),
	}
}

// add appends one message to its conversation's batch and moves the quiet
// window forward.
func (b *messageBatcher) add(key, channelName string, payload json.RawMessage) {
	if key == "" || len(payload) == 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	batch := b.pending[key]
	if batch == nil {
		batch = &pendingBatch{channel: channelName}
		b.pending[key] = batch
	}
	batch.payloads = append(batch.payloads, payload)
	batch.lastAt = b.now()
}

// clear drops a conversation's pending messages, for an explicit stop.
func (b *messageBatcher) clear(key string) {
	if key == "" {
		return
	}

	b.mu.Lock()
	delete(b.pending, key)
	b.mu.Unlock()
}

// due removes and returns the conversation batches whose quiet window elapsed.
// busy reports whether the conversation still has a turn running or queued; a
// busy conversation keeps its batch pending, so a flush can never queue a second
// answer behind a run that is already streaming.
func (b *messageBatcher) due(busy func(key string) bool) []readyBatch {
	now := b.now()

	b.mu.Lock()
	defer b.mu.Unlock()

	var ready []readyBatch
	for key, batch := range b.pending {
		if now.Sub(batch.lastAt) < b.window {
			continue
		}
		if busy != nil && busy(key) {
			continue
		}
		delete(b.pending, key)

		envelope, err := json.Marshal(channel.QueueEnvelope{
			Channel:  batch.channel,
			Payloads: batch.payloads,
		})
		if err != nil {
			continue
		}
		ready = append(ready, readyBatch{key: key, envelope: envelope})
	}
	return ready
}
