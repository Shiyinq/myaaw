package service

import (
	"context"
	"strings"
	"sync"

	"myaaw/internal/agent"
	"myaaw/internal/channel"
)

// processRuns is the registry shared by the bot service and the queue consumer.
// The binary runs one agent, so a single registry is enough; tests inject their
// own to stay isolated.
var processRuns = &runRegistry{}

// CancelTurn stops the turn currently answering key. The queue consumer calls it
// when an interrupting chat message arrives, before that message is dispatched.
func CancelTurn(key string) bool {
	return processRuns.cancel(key)
}

// TurnKey is the conversation identity shared by the conversation lock, the run
// registry, and the queue consumer.
func TurnKey(channelName, conversationID, chatID, topicID string) string {
	if id := strings.TrimSpace(conversationID); id != "" {
		return "conversation:" + id
	}
	return "target:" + strings.ToLower(strings.TrimSpace(channelName)) + ":" + chatID + ":" + topicID
}

// activeTurn is one in-flight agent turn that another goroutine may stop.
type activeTurn struct {
	cancel context.CancelCauseFunc
}

// runRegistry maps a conversation key to the turn currently answering it.
//
// A channel turn runs inside the webhook request that delivered it, while a
// stop command arrives in a different request. The cancel func is what lets the
// second request reach the first goroutine, the same way the web API's
// request-id run broker does.
//
// The zero value is ready to use.
type runRegistry struct {
	mu   sync.Mutex
	runs map[string]*activeTurn
}

// register records cancel as the turn answering key. The returned handle must
// be passed to unregister, so a turn that finishes after a newer one started
// cannot remove the newer turn's entry.
func (r *runRegistry) register(key string, cancel context.CancelCauseFunc) *activeTurn {
	if key == "" || cancel == nil {
		return nil
	}

	turn := &activeTurn{cancel: cancel}
	r.mu.Lock()
	if r.runs == nil {
		r.runs = make(map[string]*activeTurn)
	}
	r.runs[key] = turn
	r.mu.Unlock()
	return turn
}

// unregister removes turn, but only while it is still the entry for key.
func (r *runRegistry) unregister(key string, turn *activeTurn) {
	if key == "" || turn == nil {
		return
	}

	r.mu.Lock()
	if r.runs[key] == turn {
		delete(r.runs, key)
	}
	r.mu.Unlock()
}

// cancel stops the turn currently answering key and reports whether one was
// running. The entry is removed first so two stops cannot cancel the same turn.
func (r *runRegistry) cancel(key string) bool {
	if key == "" {
		return false
	}

	r.mu.Lock()
	turn := r.runs[key]
	if turn != nil {
		delete(r.runs, key)
	}
	r.mu.Unlock()

	if turn == nil {
		return false
	}
	turn.cancel(agent.ErrRunStopped)
	return true
}

// turnRegistry is the registry this service registers turns in: its own when a
// caller injected one (tests), otherwise the process-wide registry the queue
// consumer cancels through.
func (r *BotServiceImpl) turnRegistry() *runRegistry {
	if r.runs != nil {
		return r.runs
	}
	return processRuns
}

// stoppableTurn reports whether msg starts a user turn that may be stopped or
// interrupted from the channel. Scheduled turns (cron, heartbeat) are delivered
// by the same channel but must run to completion.
func stoppableTurn(msg *channel.IncomingMessage) bool {
	return msg != nil && msg.TriggerType == "" && channel.Interactive(msg.Channel)
}
