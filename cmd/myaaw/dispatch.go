package main

import (
	"sync"
)

// keyedDispatcher runs jobs concurrently up to a limit while serializing jobs
// that share a key. Inbound chats use the user ID as the key, so one
// conversation keeps its order but different users make progress in parallel.
type keyedDispatcher struct {
	limit int
	run   func(job []byte)

	mu      sync.Mutex
	active  int
	running map[string]bool
	queues  map[string][][]byte
}

func newKeyedDispatcher(limit int, run func(job []byte)) *keyedDispatcher {
	if limit < 1 {
		limit = 1
	}
	return &keyedDispatcher{
		limit:   limit,
		run:     run,
		running: make(map[string]bool),
		queues:  make(map[string][][]byte),
	}
}

// Submit queues job behind any work already running or pending for key.
func (d *keyedDispatcher) Submit(key string, job []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running[key] && len(d.queues[key]) == 0 && d.active < d.limit {
		d.startLocked(key, job)
		return
	}
	d.queues[key] = append(d.queues[key], job)
}

func (d *keyedDispatcher) startLocked(key string, job []byte) {
	d.active++
	d.running[key] = true
	go func() {
		d.run(job)
		d.finish(key)
	}()
}

func (d *keyedDispatcher) finish(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.active--
	delete(d.running, key)

	// Continue the same conversation before starting anyone else, so per-key
	// ordering never depends on goroutine scheduling.
	if next, ok := d.popLocked(key); ok {
		d.startLocked(key, next)
		return
	}

	// Free capacity: pick up work from another key that was waiting for a slot.
	for otherKey := range d.queues {
		if d.running[otherKey] {
			continue
		}
		next, ok := d.popLocked(otherKey)
		if !ok {
			continue
		}
		d.startLocked(otherKey, next)
		return
	}
}

func (d *keyedDispatcher) popLocked(key string) ([]byte, bool) {
	queued, ok := d.queues[key]
	if !ok || len(queued) == 0 {
		delete(d.queues, key)
		return nil, false
	}
	next := queued[0]
	if len(queued) == 1 {
		delete(d.queues, key)
	} else {
		d.queues[key] = queued[1:]
	}
	return next, true
}
