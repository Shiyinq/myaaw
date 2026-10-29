// Package approvals implements the tool-call approval gate shared by every
// channel. The gate is channel-agnostic: interactive channels provide a
// Requester, while runs without one (cron, heartbeat, subagent notifications,
// headless, inbound A2A) fall back to the configured unattended decision.
package approvals

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"myaaw/internal/config"
)

// UserIDHeader carries the acting user id on HTTP approval decisions. The
// control plane sets it from the authenticated session.
const UserIDHeader = "X-Myaaw-User-ID"

// Decision is the user's answer to an approval request.
type Decision string

const (
	AllowOnce    Decision = "allow_once"
	AllowSession Decision = "allow_session"
	AllowAlways  Decision = "allow_always"
	Deny         Decision = "deny"
)

// Request describes one approval request shown to a user.
type Request struct {
	ID          string
	RunID       string
	ToolName    string
	Action      string
	ArgsSummary string
	// Args is the full raw argument JSON. Approval UIs may expand it; the
	// summary above stays the one-line preview.
	Args           string
	Channel        string
	ConversationID string
	UserID         string
	// SubAgent is the task name of the sub-agent that made the call, empty for
	// calls made by the main agent. Prompts use it to show where the request
	// came from.
	SubAgent string
}

// Requester delivers an approval request to a user and waits for the decision.
// Implementations must honor ctx cancellation and the deadline.
type Requester interface {
	Request(ctx context.Context, req Request) (Decision, error)
}

// Session identifies the run and conversation an approval belongs to.
type Session struct {
	RunID          string
	Channel        string
	ConversationID string
	UserID         string
}

// Policy is the static approval configuration.
type Policy struct {
	Mode       string // manual | off
	Timeout    time.Duration
	Allow      []string // whitelist: always allowed, never prompts
	Deny       []string // hard refusal, wins over everything
	Unattended Decision // decision when no Requester is attached
	Subagents  string   // inherit | allow | deny
}

// PolicyFromConfig builds a Policy from the process configuration.
func PolicyFromConfig() Policy {
	timeout := time.Duration(config.ApprovalsTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	unattended := AllowOnce
	if strings.EqualFold(config.ApprovalsUnattended, "deny") {
		unattended = Deny
	}
	return Policy{
		Mode:       strings.ToLower(strings.TrimSpace(config.ApprovalsMode)),
		Timeout:    timeout,
		Allow:      append([]string(nil), config.ApprovalsAllow...),
		Deny:       append([]string(nil), config.ApprovalsDeny...),
		Unattended: unattended,
		Subagents:  strings.ToLower(strings.TrimSpace(config.ApprovalsSubagents)),
	}
}

// Outcome is the result of checking one tool call.
type Outcome struct {
	Allowed  bool
	Reason   string
	Decision Decision
	Key      string
	Action   string
}

// Gate evaluates tool calls for one run.
type Gate struct {
	Policy    Policy
	Store     *Store
	Session   Session
	Requester Requester
	// Auto is used when Requester is nil. Empty behaves like Deny.
	Auto Decision
	// Audit records every decision with the full arguments. Optional.
	Audit *AuditLog
}

// Check resolves the approval policy for one tool call.
func (g *Gate) Check(ctx context.Context, toolName string, args json.RawMessage) (out Outcome) {
	key, action := Key(toolName, args)
	out = Outcome{Allowed: true, Key: key, Action: action, Decision: AllowOnce}
	if g == nil {
		return out
	}

	scope := SessionScope{
		Scope:          Scope{Channel: g.Session.Channel, UserID: g.Session.UserID},
		ConversationID: g.Session.ConversationID,
	}
	source := "policy"
	requestID := ""
	var waited time.Duration
	start := time.Now()

	defer func() {
		if g.Audit == nil {
			return
		}
		g.Audit.Record(AuditEntry{
			RequestID:      requestID,
			RunID:          g.Session.RunID,
			Channel:        g.Session.Channel,
			UserID:         g.Session.UserID,
			ConversationID: g.Session.ConversationID,
			SubAgent:       SubagentName(ctx),
			Tool:           toolName,
			Action:         action,
			Key:            key,
			Args:           auditArgs(args),
			Decision:       string(out.Decision),
			Source:         source,
			Allowed:        out.Allowed,
			Reason:         out.Reason,
			WaitedMS:       waited.Milliseconds(),
		})
	}()

	p := g.Policy
	if matches(p.Deny, key, toolName) {
		source = "deny_list"
		return Outcome{
			Allowed:  false,
			Reason:   fmt.Sprintf("tool %q is denied by approval policy", key),
			Decision: Deny,
			Key:      key,
			Action:   action,
		}
	}
	if matches(p.Allow, key, toolName) {
		source = "allow_list"
		return out
	}
	if strings.EqualFold(p.Mode, "off") {
		source = "mode_off"
		return out
	}

	if g.Store != nil {
		if decision, ok := g.Store.Grant(key, scope); ok {
			source = "grant"
			out.Decision = decision
			return out
		}
	}

	if g.Requester == nil {
		decision := g.Auto
		if decision == "" {
			decision = Deny
		}
		if decision == Deny {
			source = "no_requester"
			return Outcome{
				Allowed:  false,
				Reason:   fmt.Sprintf("tool %q requires approval but no interactive channel is attached", key),
				Decision: Deny,
				Key:      key,
				Action:   action,
			}
		}
		source = "unattended"
		out.Decision = decision
		return out
	}

	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	requestID = NewToken()
	decision, err := g.Requester.Request(reqCtx, Request{
		ID:             requestID,
		RunID:          g.Session.RunID,
		ToolName:       toolName,
		Action:         action,
		ArgsSummary:    SummarizeArgs(args),
		Args:           string(args),
		Channel:        g.Session.Channel,
		ConversationID: g.Session.ConversationID,
		UserID:         g.Session.UserID,
		SubAgent:       SubagentName(ctx),
	})
	waited = time.Since(start)
	if err != nil {
		source = "error"
		if errors.Is(err, context.DeadlineExceeded) {
			source = "timeout"
		}
		return Outcome{
			Allowed:  false,
			Reason:   fmt.Sprintf("approval for %q failed: %v", key, err),
			Decision: Deny,
			Key:      key,
			Action:   action,
		}
	}

	switch decision {
	case AllowOnce:
		source = "user"
	case AllowSession:
		source = "user"
		if g.Store != nil {
			_ = g.Store.GrantSession(key, scope)
		}
	case AllowAlways:
		source = "user"
		if g.Store != nil {
			_ = g.Store.GrantAlways(key, Scope{Channel: g.Session.Channel, UserID: g.Session.UserID})
		}
	case Deny:
		source = "user_deny"
		return Outcome{
			Allowed:  false,
			Reason:   fmt.Sprintf("approval denied for %q", key),
			Decision: Deny,
			Key:      key,
			Action:   action,
		}
	default:
		source = "user"
		return Outcome{
			Allowed:  false,
			Reason:   fmt.Sprintf("unknown approval decision %q for %q", decision, key),
			Decision: Deny,
			Key:      key,
			Action:   action,
		}
	}
	out.Decision = decision
	return out
}

// DenyAll returns a gate that refuses every call. It backs the
// `approvals.subagents: deny` policy.
func DenyAll() *Gate {
	return &Gate{
		Policy: Policy{Mode: "manual", Deny: []string{"*"}},
		Auto:   Deny,
	}
}

// Key returns the canonical approval key for a call: "tool:action" when the
// arguments carry a string "action" field (e.g. the computer tool), otherwise
// the bare tool name.
func Key(toolName string, args json.RawMessage) (key, action string) {
	action = actionFromArgs(args)
	if action != "" {
		return toolName + ":" + action, action
	}
	return toolName, ""
}

func actionFromArgs(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var payload struct {
		Action   string `json:"action"`
		ToolName string `json:"tool_name"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return ""
	}
	if action := strings.TrimSpace(payload.Action); action != "" {
		return action
	}
	return strings.TrimSpace(payload.ToolName)
}

// matches reports whether any rule matches the call. A bare tool rule matches
// every action of that tool; "*" matches everything.
func matches(rules []string, key, toolName string) bool {
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if rule == "*" || rule == key || rule == toolName {
			return true
		}
	}
	return false
}

// SummarizeArgs renders a truncated, single-line argument preview for prompts.
func SummarizeArgs(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, args); err != nil {
		return truncate(string(args), 200)
	}
	return truncate(compact.String(), 200)
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "…"
}

// NewToken returns an unguessable approval token.
func NewToken() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand should never fail; fall back to a timestamp-derived token
		// so the gate still works (it is not used for authorization alone).
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
