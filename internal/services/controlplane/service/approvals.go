package service

import (
	"errors"
	"sort"
	"time"

	"myaaw/internal/agent/approvals"
	"myaaw/internal/agentdir"
	"myaaw/internal/config"
)

// ErrGrantNotFound is returned when revoking a grant that does not exist.
var ErrGrantNotFound = errors.New("approval grant not found")

// ScopeView identifies the actor a grant belongs to.
type ScopeView struct {
	Channel string `json:"channel"`
	UserID  string `json:"user_id"`
}

// SessionGrantView is one conversation-scoped grant.
type SessionGrantView struct {
	Channel   string    `json:"channel"`
	UserID    string    `json:"user_id"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

// GrantView is one approval grant entry.
type GrantView struct {
	Key      string             `json:"key"`
	Always   []ScopeView        `json:"always,omitempty"`
	Sessions []SessionGrantView `json:"sessions,omitempty"`
}

// ApprovalsView is the read-only approvals surface: effective policy + grants.
type ApprovalsView struct {
	Config *config.ApprovalsConfig `json:"config"`
	Grants []GrantView             `json:"grants"`
}

// ApprovalsService manages the approval policy view and the runtime grant
// store. Policy writes go through ConfigService.Patch; this service only reads
// the policy and mutates grants.
type ApprovalsService struct{}

func NewApprovalsService() *ApprovalsService {
	return &ApprovalsService{}
}

// View returns the effective approval policy and the current grants.
func (s *ApprovalsService) View(agentName string) (ApprovalsView, error) {
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return ApprovalsView{}, err
	}
	path, err := agentdir.ApprovalsPath(agentName)
	if err != nil {
		return ApprovalsView{}, err
	}
	store := approvals.NewStore(path)
	return ApprovalsView{
		Config: mergedApprovals(cfg.Approvals),
		Grants: grantViews(store.Snapshot()),
	}, nil
}

// RevokeKey removes every grant for one tool key.
func (s *ApprovalsService) RevokeKey(agentName, key string) error {
	path, err := agentdir.ApprovalsPath(agentName)
	if err != nil {
		return err
	}
	store := approvals.NewStore(path)
	if _, ok := store.Snapshot()[key]; !ok {
		return ErrGrantNotFound
	}
	return store.Revoke(key)
}

// RevokeScope removes the always grants and sessions that belong to one
// channel/user. Empty fields act as wildcards.
func (s *ApprovalsService) RevokeScope(agentName, key, channel, userID string) error {
	path, err := agentdir.ApprovalsPath(agentName)
	if err != nil {
		return err
	}
	removed, err := approvals.NewStore(path).RevokeScope(key, approvals.Scope{Channel: channel, UserID: userID})
	if err != nil {
		return err
	}
	if !removed {
		return ErrGrantNotFound
	}
	return nil
}

// RevokeSession removes one conversation grant for a tool key.
func (s *ApprovalsService) RevokeSession(agentName, key, conversationID, channel, userID string) error {
	path, err := agentdir.ApprovalsPath(agentName)
	if err != nil {
		return err
	}
	scope := approvals.SessionScope{
		Scope:          approvals.Scope{Channel: channel, UserID: userID},
		ConversationID: conversationID,
	}
	removed, err := approvals.NewStore(path).RevokeSession(key, scope)
	if err != nil {
		return err
	}
	if !removed {
		return ErrGrantNotFound
	}
	return nil
}

// Clear removes every grant for the agent.
func (s *ApprovalsService) Clear(agentName string) error {
	path, err := agentdir.ApprovalsPath(agentName)
	if err != nil {
		return err
	}
	return approvals.NewStore(path).Clear()
}

func grantViews(grants map[string]approvals.Grant) []GrantView {
	views := make([]GrantView, 0, len(grants))
	for key, grant := range grants {
		view := GrantView{Key: key}
		for _, scope := range grant.Always {
			view.Always = append(view.Always, ScopeView{Channel: scope.Channel, UserID: scope.UserID})
		}
		for _, entry := range grant.Sessions {
			view.Sessions = append(view.Sessions, SessionGrantView{
				Channel:   entry.Channel,
				UserID:    entry.UserID,
				ID:        entry.ConversationID,
				CreatedAt: entry.CreatedAt,
			})
		}
		sort.Slice(view.Sessions, func(i, j int) bool {
			return view.Sessions[i].CreatedAt.Before(view.Sessions[j].CreatedAt)
		})
		views = append(views, view)
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Key < views[j].Key })
	return views
}
