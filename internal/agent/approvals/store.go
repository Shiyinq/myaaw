package approvals

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// maxSessionGrants bounds the per-key session grant map so a long-lived agent
// cannot grow the file without bound.
const maxSessionGrants = 100

// Grant is one persisted approval grant for a tool key.
type Grant struct {
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
	if err := json.Unmarshal(data, &grants); err != nil {
		// Keep the in-memory copy rather than wiping grants on a transient read.
		return
	}
	if grants == nil {
		grants = map[string]Grant{}
	}
	s.grants = grants
}

// Grant returns a stored decision for key, scoped to conversationID.
func (s *Store) Grant(key, conversationID string) (Decision, bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	grant, ok := s.grants[key]
	if !ok {
		return "", false
	}
	if grant.Always {
		return AllowAlways, true
	}
	if conversationID != "" {
		if _, ok := grant.Sessions[conversationID]; ok {
			return AllowSession, true
		}
	}
	return "", false
}

// GrantSession remembers key for the given conversation.
func (s *Store) GrantSession(key, conversationID string) error {
	return s.mutate(func(grants map[string]Grant) {
		grant := grants[key]
		if grant.Sessions == nil {
			grant.Sessions = map[string]time.Time{}
		}
		grant.Sessions[conversationID] = time.Now()
		pruneSessions(grant.Sessions)
		grants[key] = grant
	})
}

// GrantAlways remembers key for every conversation.
func (s *Store) GrantAlways(key string) error {
	return s.mutate(func(grants map[string]Grant) {
		grants[key] = Grant{Always: true}
	})
}

// Revoke removes every grant for key.
func (s *Store) Revoke(key string) error {
	return s.mutate(func(grants map[string]Grant) {
		delete(grants, key)
	})
}

// RevokeSession removes one conversation grant for key. It reports false when
// the key or session does not exist.
func (s *Store) RevokeSession(key, conversationID string) (bool, error) {
	removed := false
	err := s.mutate(func(grants map[string]Grant) {
		grant, ok := grants[key]
		if !ok || grant.Sessions == nil {
			return
		}
		if _, ok := grant.Sessions[conversationID]; !ok {
			return
		}
		delete(grant.Sessions, conversationID)
		if len(grant.Sessions) == 0 && !grant.Always {
			delete(grants, key)
		} else {
			grants[key] = grant
		}
		removed = true
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
		copied := Grant{Always: grant.Always}
		if grant.Sessions != nil {
			copied.Sessions = make(map[string]time.Time, len(grant.Sessions))
			for id, created := range grant.Sessions {
				copied.Sessions[id] = created
			}
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

func pruneSessions(sessions map[string]time.Time) {
	if len(sessions) <= maxSessionGrants {
		return
	}
	names := make([]string, 0, len(sessions))
	for name := range sessions {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return sessions[names[i]].Before(sessions[names[j]])
	})
	for _, name := range names[:len(names)-maxSessionGrants] {
		delete(sessions, name)
	}
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
