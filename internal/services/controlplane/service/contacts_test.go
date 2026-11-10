package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/agentdir"
)

func TestContactServiceCRUD(t *testing.T) {
	setupConfigAgent(t, nil)
	svc := NewContactService()

	list, err := svc.List("myfriend")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("list = %+v", list)
	}

	view, err := svc.Create("myfriend", ContactInput{
		Name:        "remote",
		URL:         "https://example.com/a2a",
		Token:       "secret-token",
		Description: "Remote agent",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if view.Name != "remote" || !view.Token.Configured || view.Token.Masked == "" || view.Token.Masked == "secret-token" {
		t.Fatalf("view = %+v", view)
	}

	if _, err := svc.Create("myfriend", ContactInput{Name: "REMOTE", URL: "https://other.example/a2a"}); !errors.Is(err, ErrContactNameTaken) {
		t.Fatalf("err = %v, want ErrContactNameTaken", err)
	}
	if _, err := svc.Create("myfriend", ContactInput{Name: "bad", URL: "not-a-url"}); !errors.Is(err, ErrInvalidContact) {
		t.Fatalf("err = %v, want ErrInvalidContact", err)
	}

	path, err := agentdir.ContactsPath("myfriend")
	if err != nil {
		t.Fatalf("ContactsPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contacts: %v", err)
	}
	var stored []ContactEntry
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("unmarshal contacts: %v", err)
	}
	if len(stored) != 1 || stored[0].Token != "secret-token" {
		t.Fatalf("stored = %+v", stored)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat contacts: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, want 600", info.Mode().Perm())
	}

	list, err = svc.List("myfriend")
	if err != nil {
		t.Fatalf("List after create: %v", err)
	}
	if len(list) != 1 || list[0].Name != "remote" {
		t.Fatalf("list = %+v", list)
	}

	if err := svc.Delete("myfriend", "remote"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := svc.Delete("myfriend", "remote"); !errors.Is(err, ErrContactNotFound) {
		t.Fatalf("err = %v, want ErrContactNotFound", err)
	}
}

func TestContactServiceUpdate(t *testing.T) {
	setupConfigAgent(t, nil)
	svc := NewContactService()

	if _, err := svc.Create("myfriend", ContactInput{
		Name:        "remote",
		URL:         "https://example.com/a2a",
		Token:       "secret-token",
		Description: "Remote agent",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Only the fields in the body change; the token stays configured.
	newURL := "https://other.example/a2a"
	description := "Updated agent"
	view, err := svc.Update("myfriend", "remote", ContactUpdate{URL: &newURL, Description: &description})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if view.URL != newURL || view.Description != description {
		t.Fatalf("view = %+v", view)
	}
	if !view.Token.Configured {
		t.Fatal("update without a token must keep the stored one")
	}

	// A non-empty token replaces it, an empty one leaves it alone.
	replacement := "new-token"
	empty := ""
	if _, err := svc.Update("myfriend", "remote", ContactUpdate{Token: &empty}); err != nil {
		t.Fatalf("Update empty token: %v", err)
	}
	stored := readContactsFile(t)
	if len(stored) != 1 || stored[0].Token != "secret-token" {
		t.Fatalf("stored after empty token = %+v", stored)
	}
	if _, err := svc.Update("myfriend", "remote", ContactUpdate{Token: &replacement}); err != nil {
		t.Fatalf("Update token: %v", err)
	}
	if stored := readContactsFile(t); stored[0].Token != "new-token" {
		t.Fatalf("stored = %+v", stored)
	}

	// clear_token removes it.
	if _, err := svc.Update("myfriend", "remote", ContactUpdate{ClearToken: true}); err != nil {
		t.Fatalf("Update clear token: %v", err)
	}
	if stored := readContactsFile(t); stored[0].Token != "" {
		t.Fatalf("stored after clear = %+v", stored)
	}

	// Renaming is allowed, and colliding with another contact is not.
	renamed := "remote-renamed"
	if _, err := svc.Update("myfriend", "remote", ContactUpdate{Name: &renamed}); err != nil {
		t.Fatalf("Update name: %v", err)
	}
	if view, err := svc.Update("myfriend", "remote-renamed", ContactUpdate{Name: &renamed}); err != nil || view.Name != "remote-renamed" {
		t.Fatalf("case-only rename = %+v, %v", view, err)
	}
	if _, err := svc.Create("myfriend", ContactInput{Name: "other", URL: "https://other.example/a2a"}); err != nil {
		t.Fatalf("Create other: %v", err)
	}
	if _, err := svc.Update("myfriend", "other", ContactUpdate{Name: &renamed}); !errors.Is(err, ErrContactNameTaken) {
		t.Fatalf("err = %v, want ErrContactNameTaken", err)
	}

	// Invalid input and a missing contact are reported as such.
	badURL := "not-a-url"
	if _, err := svc.Update("myfriend", "other", ContactUpdate{URL: &badURL}); !errors.Is(err, ErrInvalidContact) {
		t.Fatalf("err = %v, want ErrInvalidContact", err)
	}
	if _, err := svc.Update("myfriend", "gone", ContactUpdate{}); !errors.Is(err, ErrContactNotFound) {
		t.Fatalf("err = %v, want ErrContactNotFound", err)
	}
}

func readContactsFile(t *testing.T) []ContactEntry {
	t.Helper()
	path, err := agentdir.ContactsPath("myfriend")
	if err != nil {
		t.Fatalf("ContactsPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contacts: %v", err)
	}
	var stored []ContactEntry
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("unmarshal contacts: %v", err)
	}
	return stored
}

func TestContactServiceInvalidFile(t *testing.T) {
	agentRoot := setupConfigAgent(t, nil)
	path := filepath.Join(agentRoot, "contacts.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write contacts: %v", err)
	}
	svc := NewContactService()
	if _, err := svc.List("myfriend"); err == nil {
		t.Fatal("invalid contacts.json should return an error")
	}
}
