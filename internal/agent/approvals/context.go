package approvals

import (
	"context"
	"strings"
)

type contextKey struct{ name string }

var (
	gateKey      = contextKey{"approvals-gate"}
	requesterKey = contextKey{"approvals-requester"}
	subagentKey  = contextKey{"approvals-subagent"}
)

// WithGate attaches the run's approval gate. The engine does this before
// executing tools so nested runs (sub-agents) can inherit it.
func WithGate(ctx context.Context, gate *Gate) context.Context {
	return context.WithValue(ctx, gateKey, gate)
}

// GateFromContext returns the approval gate attached to ctx, if any.
func GateFromContext(ctx context.Context) *Gate {
	gate, _ := ctx.Value(gateKey).(*Gate)
	return gate
}

// WithRequester attaches a channel requester for the run. Channel entry points
// (CLI, Telegram, Discord, API) set this before starting a run.
func WithRequester(ctx context.Context, requester Requester) context.Context {
	return context.WithValue(ctx, requesterKey, requester)
}

// RequesterFromContext returns the channel requester attached to ctx.
func RequesterFromContext(ctx context.Context) Requester {
	requester, _ := ctx.Value(requesterKey).(Requester)
	return requester
}

// WithSubagent marks ctx as a sub-agent run without naming it.
func WithSubagent(ctx context.Context) context.Context {
	return WithSubagentNamed(ctx, "")
}

// WithSubagentNamed marks ctx as a sub-agent run and records the task name so
// approval prompts can show where the request came from.
func WithSubagentNamed(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, subagentKey, strings.TrimSpace(name))
}

// IsSubagent reports whether ctx belongs to a sub-agent run.
func IsSubagent(ctx context.Context) bool {
	return ctx.Value(subagentKey) != nil
}

// SubagentName returns the sub-agent task name attached to ctx, if any.
func SubagentName(ctx context.Context) string {
	name, _ := ctx.Value(subagentKey).(string)
	return name
}
