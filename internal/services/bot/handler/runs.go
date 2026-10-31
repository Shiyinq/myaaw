package handler

import (
	"context"
	"strings"
	"sync"

	"myaaw/internal/agent"
)

// runRegistry tracks in-flight runs by the id the client sent with them, so the
// same client can stop a run later. Fiber's request context is detached from
// the connection and is never cancelled, so chat handlers cannot rely on a
// disconnect to end a run.
type runRegistry struct {
	mu   sync.Mutex
	runs map[string]context.CancelCauseFunc
}

func newRunRegistry() *runRegistry {
	return &runRegistry{runs: make(map[string]context.CancelCauseFunc)}
}

// activeRuns holds the runs of this agent server. A run is keyed by the
// request_id of the chat request that started it.
var activeRuns = newRunRegistry()

// register stores cancel for requestID and returns the release function the
// caller must run when the run finishes.
func (r *runRegistry) register(requestID string, cancel context.CancelCauseFunc) func() {
	if requestID == "" {
		return func() {}
	}

	r.mu.Lock()
	r.runs[requestID] = cancel
	r.mu.Unlock()

	return func() {
		r.mu.Lock()
		delete(r.runs, requestID)
		r.mu.Unlock()
		cancel(nil)
	}
}

// stop cancels the run registered under requestID. It reports whether a run was
// found, so the API can tell a stale id from a stopped run.
func (r *runRegistry) stop(requestID string) bool {
	r.mu.Lock()
	cancel, ok := r.runs[requestID]
	r.mu.Unlock()
	if !ok {
		return false
	}
	cancel(agent.ErrRunStopped)
	return true
}

// runContext derives the cancelable context of one chat request. Without a
// request id there is nothing to stop, so the parent context is used as-is.
func runContext(parent context.Context, requestID string) (context.Context, context.CancelCauseFunc, func()) {
	if strings.TrimSpace(requestID) == "" {
		return parent, func(error) {}, func() {}
	}

	ctx, cancel := context.WithCancelCause(parent)
	release := activeRuns.register(requestID, cancel)
	return ctx, cancel, release
}

// stopRun is the cause a chat handler uses when the client goes away mid-stream.
func stopRun(cancel context.CancelCauseFunc) {
	cancel(agent.ErrRunStopped)
}
