package service

import (
	"errors"
	"strings"
	"testing"
)

func TestChatFolderLifecycle(t *testing.T) {
	setupConversationAgent(t)
	conversations := NewConversationService()

	folders, err := conversations.ListFolders("myfriend", 7)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	if len(folders) != 0 {
		t.Fatalf("expected no folders, got %d", len(folders))
	}

	folder, err := conversations.CreateFolder("myfriend", 7, "  Work  ")
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if folder.ID == "" || folder.Name != "Work" || folder.Count != 0 {
		t.Fatalf("unexpected folder: %+v", folder)
	}

	if _, err := conversations.CreateFolder("myfriend", 7, "work"); !errors.Is(err, ErrFolderNameTaken) {
		t.Fatalf("duplicate name: want ErrFolderNameTaken, got %v", err)
	}
	for _, name := range []string{"", "   ", strings.Repeat("x", 65)} {
		if _, err := conversations.CreateFolder("myfriend", 7, name); !errors.Is(err, ErrInvalidFolderName) {
			t.Fatalf("name %q: want ErrInvalidFolderName, got %v", name, err)
		}
	}

	renamed, err := conversations.RenameFolder("myfriend", folder.ID, 7, "Projects")
	if err != nil {
		t.Fatalf("rename folder: %v", err)
	}
	if renamed.Name != "Projects" {
		t.Fatalf("rename kept %q", renamed.Name)
	}
	if _, err := conversations.RenameFolder("myfriend", folder.ID, 8, "Other"); !errors.Is(err, ErrFolderNotFound) {
		t.Fatalf("rename foreign folder: want ErrFolderNotFound, got %v", err)
	}

	// Move two chats in and check the count.
	for _, id := range []string{"conv-1", "conv-2"} {
		if err := conversations.SetFolder("myfriend", id, 7, &folder.ID); err != nil {
			t.Fatalf("move %s: %v", id, err)
		}
	}
	folders, err = conversations.ListFolders("myfriend", 7)
	if err != nil || len(folders) != 1 || folders[0].Count != 2 {
		t.Fatalf("counts after move: %+v, err=%v", folders, err)
	}

	// Moving out returns the chat to Recents.
	if err := conversations.SetFolder("myfriend", "conv-1", 7, nil); err != nil {
		t.Fatalf("move out: %v", err)
	}
	recents, err := conversations.List("myfriend", 7, 10, 0, "", ConversationFilter{Unfiled: true})
	if err != nil {
		t.Fatalf("list recents: %v", err)
	}
	if len(recents.Conversations) != 1 || recents.Conversations[0].ID != "conv-1" {
		t.Fatalf("unexpected recents: %+v", recents.Conversations)
	}

	// Deleting the folder deletes the chats inside it.
	result, err := conversations.DeleteFolder("myfriend", folder.ID, 7)
	if err != nil {
		t.Fatalf("delete folder: %v", err)
	}
	if result.DeletedChats != 1 || len(result.DeletedIDs) != 1 || result.DeletedIDs[0] != "conv-2" {
		t.Fatalf("unexpected delete result: %+v", result)
	}
	if _, err := conversations.Get("myfriend", "conv-2", 7, 10, 0); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("deleted chat still readable: %v", err)
	}
	if _, err := conversations.Get("myfriend", "conv-1", 7, 10, 0); err != nil {
		t.Fatalf("chat moved out should survive: %v", err)
	}

	// A chat can only move into a folder owned by the same user.
	other, err := conversations.CreateFolder("myfriend", 8, "Other")
	if err != nil {
		t.Fatalf("create folder for other user: %v", err)
	}
	if err := conversations.SetFolder("myfriend", "conv-1", 7, &other.ID); !errors.Is(err, ErrFolderNotFound) {
		t.Fatalf("foreign folder: want ErrFolderNotFound, got %v", err)
	}
	if err := conversations.SetFolder("myfriend", "conv-3", 7, nil); !errors.Is(err, ErrConversationForbidden) {
		t.Fatalf("foreign chat: want ErrConversationForbidden, got %v", err)
	}
}

func TestConversationPinAndFilters(t *testing.T) {
	setupConversationAgent(t)
	conversations := NewConversationService()

	if err := conversations.SetPinned("myfriend", "conv-1", 7, true); err != nil {
		t.Fatalf("pin: %v", err)
	}
	pinned, err := conversations.List("myfriend", 7, 10, 0, "", ConversationFilter{Pinned: true})
	if err != nil {
		t.Fatalf("list pinned: %v", err)
	}
	if len(pinned.Conversations) != 1 || pinned.Conversations[0].ID != "conv-1" {
		t.Fatalf("unexpected pinned: %+v", pinned.Conversations)
	}
	if !pinned.Conversations[0].Pinned || pinned.Conversations[0].PinnedAt == nil {
		t.Fatalf("pinned flags missing: %+v", pinned.Conversations[0])
	}

	recents, err := conversations.List("myfriend", 7, 10, 0, "", ConversationFilter{Unfiled: true})
	if err != nil {
		t.Fatalf("list recents: %v", err)
	}
	if len(recents.Conversations) != 1 || recents.Conversations[0].ID != "conv-2" {
		t.Fatalf("unexpected recents: %+v", recents.Conversations)
	}

	// A pinned chat inside a folder appears in both sections.
	folder, err := conversations.CreateFolder("myfriend", 7, "Work")
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if err := conversations.SetFolder("myfriend", "conv-1", 7, &folder.ID); err != nil {
		t.Fatalf("move pinned chat: %v", err)
	}
	inFolder, err := conversations.List("myfriend", 7, 10, 0, "", ConversationFilter{FolderID: folder.ID})
	if err != nil {
		t.Fatalf("list folder: %v", err)
	}
	if len(inFolder.Conversations) != 1 || inFolder.Conversations[0].ID != "conv-1" {
		t.Fatalf("unexpected folder list: %+v", inFolder.Conversations)
	}
	if inFolder.Conversations[0].FolderID == nil || *inFolder.Conversations[0].FolderID != folder.ID {
		t.Fatalf("folder id missing: %+v", inFolder.Conversations[0])
	}

	if err := conversations.SetPinned("myfriend", "conv-1", 7, false); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	pinned, err = conversations.List("myfriend", 7, 10, 0, "", ConversationFilter{Pinned: true})
	if err != nil || len(pinned.Conversations) != 0 {
		t.Fatalf("unpin left rows: %+v, err=%v", pinned.Conversations, err)
	}

	// Another user cannot pin or move a foreign chat.
	if err := conversations.SetPinned("myfriend", "conv-3", 7, true); !errors.Is(err, ErrConversationForbidden) {
		t.Fatalf("foreign pin: want ErrConversationForbidden, got %v", err)
	}
	if err := conversations.SetPinned("myfriend", "missing", 7, true); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("missing pin: want ErrConversationNotFound, got %v", err)
	}
}

func TestConversationDetailCarriesOrganization(t *testing.T) {
	setupConversationAgent(t)
	conversations := NewConversationService()

	folder, err := conversations.CreateFolder("myfriend", 7, "Work")
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if err := conversations.SetFolder("myfriend", "conv-1", 7, &folder.ID); err != nil {
		t.Fatalf("move: %v", err)
	}
	if err := conversations.SetPinned("myfriend", "conv-1", 7, true); err != nil {
		t.Fatalf("pin: %v", err)
	}

	detail, err := conversations.Get("myfriend", "conv-1", 7, 10, 0)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !detail.Pinned || detail.PinnedAt == nil {
		t.Fatalf("detail missing pin: %+v", detail)
	}
	if detail.FolderID == nil || *detail.FolderID != folder.ID {
		t.Fatalf("detail missing folder: %+v", detail)
	}
}
