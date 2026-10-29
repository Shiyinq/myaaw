package service

import (
	"context"
	"strconv"
	"strings"

	"myaaw/internal/agent/approvals"
	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
)

// approvalGate builds the per-run approval gate. Interactive channels attach a
// Requester through the context; runs without one fall back to the unattended
// policy. Channels whose interactive requester is not wired yet deny gated
// tools rather than silently approving them.
func (r *BotServiceImpl) approvalGate(ctx context.Context, msg *channel.IncomingMessage) *approvals.Gate {
	if msg == nil {
		return nil
	}
	policy := approvals.PolicyFromConfig()
	gate := &approvals.Gate{
		Policy: policy,
		Session: approvals.Session{
			Channel:        msg.Channel,
			ConversationID: msg.ConversationID,
			UserID:         strconv.Itoa(msg.UserID),
		},
		Requester: approvals.RequesterFromContext(ctx),
	}
	if path, err := agentdir.ActiveApprovalsPath(); err == nil {
		gate.Store = approvals.NewStore(path)
	}
	if gate.Requester == nil {
		if isUnattendedRun(msg) {
			gate.Auto = policy.Unattended
		} else {
			gate.Auto = approvals.Deny
		}
	}
	return gate
}

// isUnattendedRun reports whether a run has no interactive user attached:
// cron, heartbeat, subagent completion notifications, inbound A2A, and
// headless runs.
func isUnattendedRun(msg *channel.IncomingMessage) bool {
	switch strings.ToLower(strings.TrimSpace(msg.TriggerType)) {
	case "cron", "heartbeat", "subagent":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(msg.Channel)) {
	case "a2a":
		return true
	}
	return false
}
