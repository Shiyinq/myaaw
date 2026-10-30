package approvals

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// maxSessionGrants bounds the per-key session grant list so a long-lived agent
// cannot grow the file without bound.
const maxSessionGrants = 100

// Scope identifies who a grant belongs to. "*" matches anything (legacy scope).
type Scope struct {
	Channel string `json:"channel,omitempty"`
	UserID  string `json:"user_id,omitempty"`
}

// SessionScope is a Scope plus the conversation a session grant belongs to.
type SessionScope struct {
	Scope
	ConversationID string
}

// SessionGrant is one conversation-scoped grant.
type SessionGrant struct {
	Channel        string    `json:"channel,omitempty"`
	UserID         string    `json:"user_id,omitempty"`
	ConversationID string    `json:"conversation_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// Grant is one persisted approval grant for a tool key.
type Grant struct {
	Always   []Scope        `json:"always,omitempty"`
	Sessions []SessionGrant `json:"sessions,omitempty"`
}

// grantLegacy is the pre-scope format: key -> {always, sessions}.
type grantLegacy struct {
	Always   bool                 `json:"always,omitempty"`
	Sessions map[string]time.Time `json:"sessions,omitempty"`
}

// Store persists approval grants (allow session / allow always) in the agent
// config directory. Mutations reload the file first (read-modify-write) so an
// external writer — the control plane, or another agent run — cannot lose
// grants it added. Writes are atomic.
type Store struct {
	mu     sync.Mutex
	path   string
	grants map[string]Grant
}

// NewStore loads grants from path. A missing or unreadable file yields an empty
// store; grant writes surface the error.
func NewStore(path string) *Store {
	s := &Store{path: path, grants: map[string]Grant{}}
	s.mu.Lock()
	s.reloadLocked()
	s.mu.Unlock()
	return s
}

func (s *Store) reloadLocked() {
	if s == nil || s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}

	var grants map[string]Grant
	if err := json.Unmarshal(data, &grants); err == nil {
		if grants == nil {
			grants = map[string]Grant{}
		}
		s.grants = grants
		return
	}

	// Legacy map: key -> {always, sessions}. Global always grants are dropped
	// because their author is unknown; sessions are kept with a wildcard scope
	// since a conversation ID already identifies one chat.
	var legacy map[string]grantLegacy
	if err := json.Unmarshal(data, &legacy); err != nil {
		return
	}
	grants = make(map[string]Grant, len(legacy))
	dropped := 0
	for key, old := range legacy {
		grant := Grant{}
		if old.Always {
			dropped++
		}
		for conversation, created := range old.Sessions {
			grant.Sessions = append(grant.Sessions, SessionGrant{
				Channel:        "*",
				UserID:         "*",
				ConversationID: conversation,
				CreatedAt:      created,
			})
		}
		sortSessionGrants(grant.Sessions)
		if len(grant.Sessions) > 0 {
			grants[key] = grant
		}
	}
	if dropped > 0 {
		log.Printf("approvals: dropped %d legacy global always grant(s) in %s; those tools will be asked again", dropped, s.path)
	}
	s.grants = grants
}

// Grant returns a stored decision for key scoped to the caller.
func (s *Store) Grant(key string, scope SessionScope) (Decision, bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	grant, ok := s.grants[key]
	if !ok {
		return "", false
	}
	for _, entry := range grant.Always {
		if scope.matches(Scope{Channel: entry.Channel, UserID: entry.UserID}) {
			return AllowAlways, true
		}
	}
	for _, entry := range grant.Sessions {
		if entry.ConversationID != scope.ConversationID {
			continue
		}
		if scope.matches(Scope{Channel: entry.Channel, UserID: entry.UserID}) {
			return AllowSession, true
		}
	}
	return "", false
}

// GrantSession remembers key for one conversation.
func (s *Store) GrantSession(key string, scope SessionScope) error {
	return s.mutate(func(grants map[string]Grant) {
		grant := grants[key]
		for i, entry := range grant.Sessions {
			if entry.ConversationID == scope.ConversationID &&
				entry.Channel == scope.Channel && entry.UserID == scope.UserID {
				grant.Sessions[i].CreatedAt = time.Now()
				grants[key] = grant
				return
			}
		}
		grant.Sessions = append(grant.Sessions, SessionGrant{
			Channel:        scope.Channel,
			UserID:         scope.UserID,
			ConversationID: scope.ConversationID,
			CreatedAt:      time.Now(),
		})
		sortSessionGrants(grant.Sessions)
		if len(grant.Sessions) > maxSessionGrants {
			grant.Sessions = grant.Sessions[len(grant.Sessions)-maxSessionGrants:]
		}
		grants[key] = grant
	})
}

// GrantAlways remembers key for every conversation of one actor.
func (s *Store) GrantAlways(key string, scope Scope) error {
	return s.mutate(func(grants map[string]Grant) {
		grant := grants[key]
		for _, entry := range grant.Always {
			if entry.Channel == scope.Channel && entry.UserID == scope.UserID {
				return
			}
		}
		grant.Always = append(grant.Always, Scope{Channel: scope.Channel, UserID: scope.UserID})
		sortAlways(grant.Always)
		grants[key] = grant
	})
}

// Revoke removes every grant for key.
func (s *Store) Revoke(key string) error {
	return s.mutate(func(grants map[string]Grant) {
		delete(grants, key)
	})
}

// RevokeScope removes the always grants and sessions that belong to the given
// channel/user. Empty fields act as wildcards.
func (s *Store) RevokeScope(key string, scope Scope) (bool, error) {
	removed := false
	err := s.mutate(func(grants map[string]Grant) {
		grant, ok := grants[key]
		if !ok {
			return
		}
		keptAlways := grant.Always[:0]
		for _, entry := range grant.Always {
			if filterMatches(entry.Channel, scope.Channel) && filterMatches(entry.UserID, scope.UserID) {
				removed = true
				continue
			}
			keptAlways = append(keptAlways, entry)
		}
		grant.Always = keptAlways

		keptSessions := grant.Sessions[:0]
		for _, entry := range grant.Sessions {
			if filterMatches(entry.Channel, scope.Channel) && filterMatches(entry.UserID, scope.UserID) {
				removed = true
				continue
			}
			keptSessions = append(keptSessions, entry)
		}
		grant.Sessions = keptSessions

		if len(grant.Always) == 0 && len(grant.Sessions) == 0 {
			delete(grants, key)
			return
		}
		grants[key] = grant
	})
	return removed, err
}

// RevokeSession removes one conversation grant for key. Empty channel/user act
// as wildcards. It reports false when nothing matched.
func (s *Store) RevokeSession(key string, scope SessionScope) (bool, error) {
	removed := false
	err := s.mutate(func(grants map[string]Grant) {
		grant, ok := grants[key]
		if !ok || len(grant.Sessions) == 0 {
			return
		}
		kept := grant.Sessions[:0]
		for _, entry := range grant.Sessions {
			if entry.ConversationID == scope.ConversationID &&
				filterMatches(entry.Channel, scope.Channel) &&
				filterMatches(entry.UserID, scope.UserID) {
				removed = true
				continue
			}
			kept = append(kept, entry)
		}
		grant.Sessions = kept
		if len(grant.Always) == 0 && len(grant.Sessions) == 0 {
			delete(grants, key)
			return
		}
		grants[key] = grant
	})
	return removed, err
}

// Clear removes every grant.
func (s *Store) Clear() error {
	return s.mutate(func(grants map[string]Grant) {
		for key := range grants {
			delete(grants, key)
		}
	})
}

// Snapshot returns a copy of the current grants (reloaded from disk).
func (s *Store) Snapshot() map[string]Grant {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloadLocked()
	out := make(map[string]Grant, len(s.grants))
	for key, grant := range s.grants {
		copied := Grant{
			Always: append([]Scope(nil), grant.Always...),
		}
		for _, entry := range grant.Sessions {
			copied.Sessions = append(copied.Sessions, entry)
		}
		out[key] = copied
	}
	return out
}

func (s *Store) mutate(fn func(map[string]Grant)) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Merge external writes before applying this mutation.
	s.reloadLocked()
	fn(s.grants)
	data, err := json.MarshalIndent(s.grants, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, data)
}

// matches reports whether a stored scope entry covers the caller's scope. "*"
// (and the empty string in an entry) matches anything.
func (scope SessionScope) matches(entry Scope) bool {
	return scopeMatches(entry.Channel, scope.Channel) && scopeMatches(entry.UserID, scope.UserID)
}

func scopeMatches(entry, want string) bool {
	if entry == "" || entry == "*" {
		return true
	}
	if want == "" {
		return false
	}
	return entry == want
}

// filterMatches is the revoke-side matcher: an empty filter means "every
// entry", while a non-empty filter matches exactly or a stored wildcard.
func filterMatches(entry, filter string) bool {
	if filter == "" || filter == "*" {
		return true
	}
	return entry == filter || entry == "*"
}

func sortAlways(entries []Scope) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Channel == entries[j].Channel {
			return entries[i].UserID < entries[j].UserID
		}
		return entries[i].Channel < entries[j].Channel
	})
}

func sortSessionGrants(entries []SessionGrant) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].ConversationID < entries[j].ConversationID
		}
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".approvals-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
