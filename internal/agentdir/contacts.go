package agentdir

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// contactEntry mirrors the A2A contacts.json shape shared by the `contact`
// tool and the control-plane API. Unknown fields are preserved on back-fill
// through the map-based helpers below.
type contactEntry struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Token       string `json:"token,omitempty"`
	Description string `json:"description,omitempty"`
}

// NewA2AToken returns a random 32-byte hex token.
func NewA2AToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// readEnvValue reads a KEY=value from an agent's .env file.
func readEnvValue(name, key string) string {
	envPath, err := EnvPath(name)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// agentBaseURL returns the HTTP base URL for an agent, or "" when it cannot be
// resolved from MYAAW_BASE_URL or PORT.
func agentBaseURL(name string) string {
	if base := strings.TrimRight(readEnvValue(name, "MYAAW_BASE_URL"), "/"); base != "" {
		return base
	}
	port := readPort(name)
	if port <= 0 {
		return ""
	}
	return fmt.Sprintf("http://localhost:%d", port)
}

// SeedA2AContacts wires a newly created agent into the A2A mesh: it generates
// an A2A_TOKEN for the new agent, adds every other initialized agent (and its
// A2A_TOKEN) to the new agent's contacts.json, and adds the new agent to every
// other agent's contacts.json.
func SeedA2AContacts(newAgent string) error {
	token, err := NewA2AToken()
	if err != nil {
		return fmt.Errorf("generate A2A token: %w", err)
	}
	envPath, err := EnvPath(newAgent)
	if err != nil {
		return err
	}
	if err := UpsertEnvValue(envPath, "A2A_TOKEN", token); err != nil {
		return fmt.Errorf("write A2A_TOKEN: %w", err)
	}

	names, err := ListAgents()
	if err != nil {
		return err
	}

	newBase := agentBaseURL(newAgent)
	if newBase == "" {
		return fmt.Errorf("cannot resolve base URL for agent %q", newAgent)
	}

	newContacts := make([]contactEntry, 0, len(names))
	for _, other := range names {
		if sameAgentName(other, newAgent) {
			continue
		}
		base := agentBaseURL(other)
		if base == "" {
			continue
		}
		newContacts = append(newContacts, contactEntry{
			Name:        other,
			URL:         base + "/a2a",
			Token:       readEnvValue(other, "A2A_TOKEN"),
			Description: "Myaaw agent " + other,
		})
	}
	if err := writeContacts(newAgent, newContacts); err != nil {
		return err
	}

	for _, other := range names {
		if sameAgentName(other, newAgent) {
			continue
		}
		if agentBaseURL(other) == "" {
			continue
		}
		if err := upsertContact(other, contactEntry{
			Name:        newAgent,
			URL:         newBase + "/a2a",
			Token:       token,
			Description: "Myaaw agent " + newAgent,
		}); err != nil {
			return err
		}
	}
	return nil
}

// EnsureA2AContacts wires an agent into the A2A mesh without rotating an
// existing token: it creates A2A_TOKEN when missing, adds every other
// initialized agent to this agent's contacts, and adds this agent to theirs.
// The agent itself is never written as its own contact. Safe to call repeatedly.
func EnsureA2AContacts(name string) error {
	envPath, err := EnvPath(name)
	if err != nil {
		return err
	}

	token := readEnvValue(name, "A2A_TOKEN")
	if token == "" {
		token, err = NewA2AToken()
		if err != nil {
			return fmt.Errorf("generate A2A token: %w", err)
		}
		if err := UpsertEnvValue(envPath, "A2A_TOKEN", token); err != nil {
			return fmt.Errorf("write A2A_TOKEN: %w", err)
		}
	}

	base := agentBaseURL(name)
	if base == "" {
		return fmt.Errorf("cannot resolve base URL for agent %q", name)
	}

	names, err := ListAgents()
	if err != nil {
		return err
	}

	// Make sure contacts.json exists even when this is the only agent, and drop
	// any self entry left by an older version or a manual edit.
	if err := ensureContactsFile(name); err != nil {
		return err
	}
	if err := removeSelfContact(name); err != nil {
		return err
	}

	for _, other := range names {
		if sameAgentName(other, name) {
			continue
		}
		otherBase := agentBaseURL(other)
		if otherBase == "" {
			continue
		}
		if err := upsertContact(name, contactEntry{
			Name:        other,
			URL:         otherBase + "/a2a",
			Token:       readEnvValue(other, "A2A_TOKEN"),
			Description: "Myaaw agent " + other,
		}); err != nil {
			return err
		}
		if err := upsertContact(other, contactEntry{
			Name:        name,
			URL:         base + "/a2a",
			Token:       token,
			Description: "Myaaw agent " + name,
		}); err != nil {
			return err
		}
	}
	return nil
}

// sameAgentName reports whether two names refer to the same agent. Comparison
// is case-insensitive because macOS and Windows filesystems are, and because
// contact entries are already matched with strings.EqualFold.
func sameAgentName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// removeSelfContact drops entries whose name matches the agent itself.
func removeSelfContact(name string) error {
	path, err := ContactsPath(name)
	if err != nil {
		return err
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil
		}
		return readErr
	}

	var entries []map[string]any
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	kept := make([]map[string]any, 0, len(entries))
	removed := false
	for _, entry := range entries {
		entryName, _ := entry["name"].(string)
		if sameAgentName(entryName, name) {
			removed = true
			continue
		}
		kept = append(kept, entry)
	}
	if !removed {
		return nil
	}

	out, err := json.MarshalIndent(kept, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(out, '\n'))
}

func ensureContactsFile(name string) error {
	path, err := ContactsPath(name)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(path); statErr == nil {
		return nil
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	return writeContacts(name, nil)
}

func writeContacts(name string, entries []contactEntry) error {
	if entries == nil {
		entries = []contactEntry{}
	}
	path, err := ContactsPath(name)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

// upsertContact adds or replaces a contact entry by name, preserving existing
// entries and any unknown fields they carry.
func upsertContact(name string, entry contactEntry) error {
	// Never store the agent itself as a contact.
	if sameAgentName(entry.Name, name) {
		return nil
	}

	path, err := ContactsPath(name)
	if err != nil {
		return err
	}

	entries := make([]map[string]any, 0)
	if data, readErr := os.ReadFile(path); readErr == nil {
		if unmarshalErr := json.Unmarshal(data, &entries); unmarshalErr != nil {
			return unmarshalErr
		}
	}

	found := false
	for i, existing := range entries {
		if n, _ := existing["name"].(string); strings.EqualFold(n, entry.Name) {
			entries[i] = contactMap(entry)
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, contactMap(entry))
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

func contactMap(entry contactEntry) map[string]any {
	m := map[string]any{"name": entry.Name, "url": entry.URL}
	if entry.Token != "" {
		m["token"] = entry.Token
	}
	if entry.Description != "" {
		m["description"] = entry.Description
	}
	return m
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
