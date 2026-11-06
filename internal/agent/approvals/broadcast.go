package approvals

import (
	"context"
	"sync"
	"time"
)

// Event is one approval lifecycle event. It is published for the dashboard so a
// request raised by a run detached from its originating stream (a sub-agent) can
// still be seen and answered.
type Event struct {
	ID             string `json:"approval_id"`
	RunID          string `json:"run_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	ToolName       string `json:"tool_name,omitempty"`
	Action         string `json:"action,omitempty"`
	ArgsSummary    string `json:"args_summary,omitempty"`
	SubAgent       string `json:"subagent,omitempty"`
	Decision       string `json:"decision,omitempty"`
	Resolved       bool   `json:"resolved"`
	At             string `json:"at"`
}

// Broker fans approval events out to every subscriber without blocking the run.
type Broker struct {
	mu   sync.Mutex
	next int
	subs map[int]chan Event
}

func NewBroker() *Broker {
	return &Broker{subs: make(map[int]chan Event)}
}

// Subscribe returns a channel of events and a cancel function. A subscriber that
// falls behind loses events instead of stalling a run; it re-syncs by reloading
// the conversation list.
func (b *Broker) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	id := b.next
	b.next++
	ch := make(chan Event, 16)
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

// Publish delivers the event to every subscriber, dropping it for a slow one.
func (b *Broker) Publish(event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

var defaultBroker = NewBroker()

// DefaultBroker is the process-wide approval event fan-out.
func DefaultBroker() *Broker { return defaultBroker }

var defaultRegistry = NewRegistry()

// DefaultRegistry is the process-wide pending-approval registry. Every channel
// requester registers there, so a decision endpoint can resolve any of them.
func DefaultRegistry() *Registry { return defaultRegistry }

// PublishRequired announces a pending approval.
func PublishRequired(req Request) {
	DefaultBroker().Publish(Event{
		ID:             req.ID,
		RunID:          req.RunID,
		ConversationID: req.ConversationID,
		ToolName:       req.ToolName,
		Action:         req.Action,
		ArgsSummary:    req.ArgsSummary,
		SubAgent:       req.SubAgent,
		At:             time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// PublishResolved announces the answer to a pending approval.
func PublishResolved(req Request, decision string) {
	DefaultBroker().Publish(Event{
		ID:             req.ID,
		RunID:          req.RunID,
		ConversationID: req.ConversationID,
		Decision:       decision,
		Resolved:       true,
		At:             time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// BroadcastRequester is the requester used by runs that outlive their channel
// stream (sub-agents): it publishes the prompt on the process-wide broker and
// waits for a decision delivered through the shared registry, so any connected
// client can answer it.
type BroadcastRequester struct{}

func (BroadcastRequester) Request(ctx context.Context, req Request) (Decision, error) {
	decisions := DefaultRegistry().Register(req.ID, Actor{Channel: "api", UserID: req.UserID})
	defer DefaultRegistry().Remove(req.ID)

	PublishRequired(req)

	select {
	case decision := <-decisions:
		PublishResolved(req, string(decision))
		return decision, nil
	case <-ctx.Done():
		PublishResolved(req, string(Deny))
		return Deny, ctx.Err()
	}
}
