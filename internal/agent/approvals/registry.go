// Package approvals implements the tool-call approval gate shared by every
// channel. The gate is channel-agnostic: interactive channels provide a
// Requester, while runs without one (cron, heartbeat, subagent notifications,
// headless, inbound A2A) fall back to the configured unattended decision.
package approvals

import "sync"

// Actor identifies who may answer a pending approval. Fields that are empty in
// the registered expectation are not checked.
type Actor struct {
	Channel        string
	UserID         string
	ConversationID string
}

func (a Actor) matches(want Actor) bool {
	if want.Channel != "" && a.Channel != want.Channel {
		return false
	}
	if want.UserID != "" && a.UserID != want.UserID {
		return false
	}
	if want.ConversationID != "" && a.ConversationID != want.ConversationID {
		return false
	}
	return true
}

type pending struct {
	ch    chan Decision
	actor Actor
}

// Registry tracks pending approval requests by token. Channels register a
// channel for a token, deliver the prompt (inline keyboard, SSE event, TUI
// prompt), and resolve it when the user answers.
type Registry struct {
	mu      sync.Mutex
	pending map[string]pending
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{pending: map[string]pending{}}
}

// Register creates a pending request for the given actor and returns the
// decision channel. An empty Actor field is treated as a wildcard.
func (r *Registry) Register(id string, actor Actor) <-chan Decision {
	if r == nil {
		closed := make(chan Decision)
		close(closed)
		return closed
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan Decision, 1)
	r.pending[id] = pending{ch: ch, actor: actor}
	return ch
}

// Check reports whether the token exists and whether actor is allowed to
// resolve it. It never mutates the registry.
func (r *Registry) Check(id string, actor Actor) (found, allowed bool) {
	if r == nil {
		return false, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pending[id]
	if !ok {
		return false, false
	}
	return true, actor.matches(p.actor)
}

// Resolve delivers a decision to the waiting run. It reports resolved=false
// when actor does not match (the request stays pending) and found=false when
// the token is unknown or already resolved.
func (r *Registry) Resolve(id string, decision Decision, actor Actor) (resolved, found bool) {
	if r == nil {
		return false, false
	}
	r.mu.Lock()
	p, ok := r.pending[id]
	if !ok {
		r.mu.Unlock()
		return false, false
	}
	if !actor.matches(p.actor) {
		r.mu.Unlock()
		return false, true
	}
	delete(r.pending, id)
	r.mu.Unlock()
	p.ch <- decision
	close(p.ch)
	return true, true
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
