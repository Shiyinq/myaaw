package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"myaaw/internal/agent"
)

const (
	// finishedRunTTL keeps a finished run around so a browser that reloaded
	// mid-answer can still read the tail and the final result.
	finishedRunTTL = 15 * time.Minute
	// maxRunBytes bounds the replay buffer of one run. Old events are dropped
	// from the front, so a very long answer still streams to live clients.
	maxRunBytes = 16 << 20
	// runKeepAlive keeps a quiet stream from being reaped by an idle proxy.
	runKeepAlive = 20 * time.Second
)

type runEvent struct {
	Name string
	Data []byte
}

// chatRun owns one agent run. It is independent of the HTTP request that
// started it: a request only attaches to the event stream. A page reload, a
// closed tab, or a logout therefore no longer ends the answer, and the browser
// can attach again later and replay what it missed.
type chatRun struct {
	requestID string

	mu           sync.Mutex
	events       []runEvent
	base         int
	bytes        int
	notify       chan struct{}
	finished     bool
	startedAt    time.Time
	finishedAt   time.Time
	conversation string

	ctx    context.Context
	cancel context.CancelCauseFunc
}

func newChatRun(requestID string) *chatRun {
	// Deliberately not derived from the request context: the run must outlive
	// the connection, and Fiber never cancels that context anyway.
	ctx, cancel := context.WithCancelCause(context.Background())
	return &chatRun{
		requestID: requestID,
		notify:    make(chan struct{}),
		startedAt: time.Now(),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Context is the run context; tools and the approval gate read it.
func (r *chatRun) Context() context.Context { return r.ctx }

func (r *chatRun) setConversation(id string) {
	if id == "" {
		return
	}
	r.mu.Lock()
	r.conversation = id
	r.mu.Unlock()
}

func (r *chatRun) conversationID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.conversation
}

// publish appends one SSE event and wakes every attached stream.
func (r *chatRun) publish(name string, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}

	r.mu.Lock()
	r.events = append(r.events, runEvent{Name: name, Data: encoded})
	r.bytes += len(encoded)
	for r.bytes > maxRunBytes && len(r.events) > 1 {
		r.bytes -= len(r.events[0].Data)
		r.events = r.events[1:]
		r.base++
	}
	close(r.notify)
	r.notify = make(chan struct{})
	r.mu.Unlock()
}

func (r *chatRun) finish() {
	r.mu.Lock()
	r.finished = true
	r.finishedAt = time.Now()
	close(r.notify)
	r.notify = make(chan struct{})
	r.mu.Unlock()
}

func (r *chatRun) streaming() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.finished
}

func (r *chatRun) finishedAtTime() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.finishedAt
}

// slice returns the events after an absolute index, the first available index,
// the end index, a channel that closes on the next publish, and whether the run
// has finished.
func (r *chatRun) slice(after int) (events []runEvent, base, end int, wait <-chan struct{}, finished bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	base = r.base
	end = r.base + len(r.events)
	if after < base {
		after = base
	}
	if after > end {
		after = end
	}
	return r.events[after-base:], base, end, r.notify, r.finished
}

// attach streams the run to one client, replaying what it missed. A write
// failure only detaches this client; the run keeps going.
func (r *chatRun) attach(w *bufio.Writer, after int) {
	for {
		events, base, end, wait, finished := r.slice(after)
		if after < base {
			after = base
		}

		for _, event := range events {
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Name, event.Data); err != nil {
				return
			}
			after++
		}
		if len(events) > 0 {
			if err := w.Flush(); err != nil {
				return
			}
		}
		if finished && after >= end {
			return
		}

		select {
		case <-wait:
		case <-time.After(runKeepAlive):
			// Comment lines keep proxies from closing an idle stream.
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			if err := w.Flush(); err != nil {
				return
			}
		}
	}
}

// RunInfo describes one run for the client that wants to attach to it.
type RunInfo struct {
	RequestID      string `json:"request_id"`
	ConversationID string `json:"conversation_id,omitempty"`
	Streaming      bool   `json:"streaming"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at,omitempty"`
}

type runBroker struct {
	mu   sync.Mutex
	runs map[string]*chatRun
}

var activeRuns = &runBroker{runs: make(map[string]*chatRun)}

// start registers a new run. The run owns its context, so nothing about the
// request can cancel it.
func (b *runBroker) start(requestID string) *chatRun {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked()

	run := newChatRun(requestID)
	b.runs[requestID] = run
	return run
}

func (b *runBroker) get(requestID string) (*chatRun, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	run, ok := b.runs[requestID]
	return run, ok
}

// stop cancels a run at the client's request.
func (b *runBroker) stop(requestID string) bool {
	b.mu.Lock()
	run, ok := b.runs[requestID]
	b.mu.Unlock()
	if !ok {
		return false
	}
	run.cancel(agent.ErrRunStopped)
	return true
}

// list reports the runs a client may attach to, newest first. An empty
// conversation id lists every run of the agent.
func (b *runBroker) list(conversationID string) []RunInfo {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked()

	infos := make([]RunInfo, 0, len(b.runs))
	for _, run := range b.runs {
		if conversationID != "" && run.conversationID() != conversationID {
			continue
		}
		info := RunInfo{
			RequestID:      run.requestID,
			ConversationID: run.conversationID(),
			Streaming:      run.streaming(),
			StartedAt:      run.startedAt.UTC().Format(time.RFC3339),
		}
		if finishedAt := run.finishedAtTime(); !finishedAt.IsZero() {
			info.FinishedAt = finishedAt.UTC().Format(time.RFC3339)
		}
		infos = append(infos, info)
	}

	sortRunInfos(infos)
	return infos
}

// pruneLocked drops runs nobody can attach to any more. Callers hold b.mu.
func (b *runBroker) pruneLocked() {
	for id, run := range b.runs {
		finishedAt := run.finishedAtTime()
		if finishedAt.IsZero() || time.Since(finishedAt) < finishedRunTTL {
			continue
		}
		delete(b.runs, id)
	}
}

func sortRunInfos(infos []RunInfo) {
	for i := 1; i < len(infos); i++ {
		for j := i; j > 0 && infos[j].StartedAt > infos[j-1].StartedAt; j-- {
			infos[j], infos[j-1] = infos[j-1], infos[j]
		}
	}
}
