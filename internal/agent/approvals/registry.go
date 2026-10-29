package approvals

import "sync"

// Registry tracks pending approval requests by token. Channels register a
// channel for a token, deliver the prompt (inline keyboard, SSE event, TUI
// prompt), and resolve it when the user answers.
type Registry struct {
	mu      sync.Mutex
	pending map[string]chan Decision
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{pending: map[string]chan Decision{}}
}

// Register creates a pending request and returns the decision channel.
func (r *Registry) Register(id string) <-chan Decision {
	if r == nil {
		closed := make(chan Decision)
		close(closed)
		return closed
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan Decision, 1)
	r.pending[id] = ch
	return ch
}

// Resolve delivers a decision to the waiting run. It reports false when the
// token is unknown or already resolved.
func (r *Registry) Resolve(id string, decision Decision) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	ch, ok := r.pending[id]
	if ok {
		delete(r.pending, id)
	}
	r.mu.Unlock()
	if !ok {
		return false
	}
	ch <- decision
	close(ch)
	return true
}

// Remove drops a pending request without resolving it (used on timeout).
func (r *Registry) Remove(id string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pending, id)
}
