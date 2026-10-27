package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"myaaw/internal/agentdir"
)

var (
	ErrContactNotFound  = errors.New("contact not found")
	ErrContactNameTaken = errors.New("contact name already exists")
	ErrInvalidContact   = errors.New("invalid contact")
)

// ContactEntry is stored in ~/.myaaw/<agent>/contacts.json.
type ContactEntry struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Token       string `json:"token,omitempty"`
	Description string `json:"description,omitempty"`
}

// ContactInput creates a contact.
type ContactInput struct {
	Name        string
	URL         string
	Token       string
	Description string
}

// ContactView masks the token on output.
type ContactView struct {
	Name        string      `json:"name"`
	URL         string      `json:"url"`
	Description string      `json:"description,omitempty"`
	Token       SecretState `json:"token"`
}

type ContactService struct{}

func NewContactService() *ContactService {
	return &ContactService{}
}

func (s *ContactService) List(agentName string) ([]ContactView, error) {
	entries, err := loadContactsFile(agentName)
	if err != nil {
		return nil, err
	}
	views := make([]ContactView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, contactView(entry))
	}
	return views, nil
}

func (s *ContactService) Create(agentName string, in ContactInput) (ContactView, error) {
	name := strings.TrimSpace(in.Name)
	rawURL := strings.TrimSpace(in.URL)
	if err := validateContact(name, rawURL); err != nil {
		return ContactView{}, err
	}

	entries, err := loadContactsFile(agentName)
	if err != nil {
		return ContactView{}, err
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name, name) {
			return ContactView{}, fmt.Errorf("%w: %s", ErrContactNameTaken, name)
		}
	}

	entry := ContactEntry{
		Name:        name,
		URL:         rawURL,
		Token:       strings.TrimSpace(in.Token),
		Description: strings.TrimSpace(in.Description),
	}
	entries = append(entries, entry)
	if err := saveContactsFile(agentName, entries); err != nil {
		return ContactView{}, err
	}
	return contactView(entry), nil
}

func (s *ContactService) Delete(agentName, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("%w: invalid contact name", ErrInvalidContact)
	}
	entries, err := loadContactsFile(agentName)
	if err != nil {
		return err
	}
	idx := -1
	for i, entry := range entries {
		if strings.EqualFold(entry.Name, name) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrContactNotFound
	}
	entries = append(entries[:idx], entries[idx+1:]...)
	return saveContactsFile(agentName, entries)
}

func contactView(entry ContactEntry) ContactView {
	view := ContactView{
		Name:        entry.Name,
		URL:         entry.URL,
		Description: entry.Description,
	}
	if entry.Token != "" {
		view.Token = SecretState{Configured: true, Masked: "********"}
	}
	return view
}

func validateContact(name, rawURL string) error {
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidContact)
	}
	if strings.ContainsAny(name, "/\\\x00") || name == "." || name == ".." {
		return fmt.Errorf("%w: invalid name %q", ErrInvalidContact, name)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: url must be an absolute http(s) URL", ErrInvalidContact)
	}
	return nil
}

func loadContactsFile(agentName string) ([]ContactEntry, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return nil, err
	}
	path, err := agentdir.ContactsPath(agentName)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ContactEntry{}, nil
		}
		return nil, err
	}
	var entries []ContactEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return entries, nil
}

func saveContactsFile(agentName string, entries []ContactEntry) error {
	if err := agentdirValidateAgent(agentName); err != nil {
		return err
	}
	path, err := agentdir.ContactsPath(agentName)
	if err != nil {
		return err
	}
	if entries == nil {
		entries = []ContactEntry{}
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
