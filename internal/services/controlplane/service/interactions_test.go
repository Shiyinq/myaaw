package service

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myaaw/internal/provider"
)

func TestApplyExtraBlockInteractionSingleUseAndPersistence(t *testing.T) {
	cpdb, err := OpenControlPlaneDBAt(filepath.Join(t.TempDir(), "controlplane.db"))
	if err != nil {
		t.Fatalf("OpenControlPlaneDBAt: %v", err)
	}
	defer cpdb.Close()

	store := NewGroupStoreWithDB(cpdb)
	now := time.Now().UTC().Format(time.RFC3339)

	groupID := "grp_test_extra"
	if err := store.Create(&storedGroup{
		Group: Group{
			ID:             groupID,
			Name:           "Test Room",
			DiscussionMode: DiscussionModeAutonomous,
			MaxTurns:       4,
			Members:        []GroupMember{{Name: "alice"}},
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		Messages: []GroupMessage{
			{
				ID:         "msg_agent_1",
				GroupID:    groupID,
				SenderType: "agent",
				SenderName: "alice",
				Content:    "Choose a format:",
				ExtraMessages: []provider.ExtraMessage{
					{
						ID: "em_1",
						Blocks: []provider.ExtraBlock{
							{
								ID:          "blk_btn_1",
								Type:        "buttons",
								Interactive: true,
								Data:        json.RawMessage(`{"buttons":[{"id":"mp4","label":"Download MP4","type":"action","value":"mp4"}]}`),
								State:       &provider.InteractionState{Status: "pending"},
							},
						},
					},
				},
				CreatedAt: now,
			},
		},
	}); err != nil {
		t.Fatalf("store.Create: %v", err)
	}

	// First submission succeeds.
	state, text, err := store.ApplyExtraBlockInteraction(groupID, InteractionRequest{
		MessageID: "msg_agent_1",
		BlockID:   "blk_btn_1",
		ActionID:  "mp4",
		Label:     "Download MP4",
		Value:     "mp4",
	}, "web")
	if err != nil {
		t.Fatalf("first ApplyExtraBlockInteraction failed: %v", err)
	}
	if state.Status != "submitted" || state.Actor != "web" {
		t.Fatalf("unexpected state: %+v", state)
	}
	if !strings.Contains(text, `<interaction kind="button">`) || !strings.Contains(text, "Download MP4") {
		t.Fatalf("unexpected interaction text: %s", text)
	}

	// Second submission is atomically rejected with ErrInteractionAlreadySubmitted.
	_, _, err = store.ApplyExtraBlockInteraction(groupID, InteractionRequest{
		MessageID: "msg_agent_1",
		BlockID:   "blk_btn_1",
		ActionID:  "mp4",
		Label:     "Download MP4",
		Value:     "mp4",
	}, "web")
	if !errors.Is(err, ErrInteractionAlreadySubmitted) {
		t.Fatalf("expected ErrInteractionAlreadySubmitted, got %v", err)
	}

	// Reload from SQLite and verify ExtraMessages + submitted state survived.
	loaded, err := store.Get(groupID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if len(loaded.Messages) != 1 || len(loaded.Messages[0].ExtraMessages) != 1 {
		t.Fatalf("extra_messages not persisted on reload: %+v", loaded.Messages)
	}
	gotBlock := loaded.Messages[0].ExtraMessages[0].Blocks[0]
	if gotBlock.State == nil || gotBlock.State.Status != "submitted" {
		t.Fatalf("block state not persisted as submitted: %+v", gotBlock.State)
	}
}

func TestExtraMessagesAppearInDMAndGroupAttachments(t *testing.T) {
	// 1. DM transcript attachment collection includes local paths from media & carousel.
	dmMessages := []provider.Message{
		{
			ID:      "m1",
			Role:    "assistant",
			Content: "here",
			ExtraMessages: []provider.ExtraMessage{
				{
					ID: "em_att",
					Blocks: []provider.ExtraBlock{
						{
							ID:   "blk_m",
							Type: "media",
							Data: json.RawMessage(`{"items":[{"path":"/home/chart.png","kind":"image","file_name":"chart.png","size":123},{"url":"https://example.com/remote.png","kind":"image"}]}`),
						},
						{
							ID:   "blk_c",
							Type: "carousel",
							Data: json.RawMessage(`{"cards":[{"id":"c1","title":"Card","image_path":"/home/card.jpg"}]}`),
						},
					},
				},
			},
		},
	}
	dmAtts := collectConversationAttachments(dmMessages)
	if len(dmAtts) != 2 {
		t.Fatalf("expected 2 local DM attachments (excluding remote URL), got %+v", dmAtts)
	}
	if dmAtts[0].Path != "/home/chart.png" || dmAtts[1].Path != "/home/card.jpg" {
		t.Fatalf("unexpected DM attachment paths: %+v", dmAtts)
	}

	// 2. Group ListAttachments includes local paths from extra_messages_json.
	cpdb, err := OpenControlPlaneDBAt(filepath.Join(t.TempDir(), "controlplane.db"))
	if err != nil {
		t.Fatalf("OpenControlPlaneDBAt: %v", err)
	}
	defer cpdb.Close()
	store := NewGroupStoreWithDB(cpdb)
	now := time.Now().UTC().Format(time.RFC3339)
	groupID := "grp_att_extra"
	if err := store.Create(&storedGroup{
		Group: Group{
			ID:             groupID,
			Name:           "Files Room",
			DiscussionMode: DiscussionModeAutonomous,
			MaxTurns:       4,
			Members:        []GroupMember{{Name: "alice"}},
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		Messages: []GroupMessage{
			{
				ID:            "msg_1",
				GroupID:       groupID,
				SenderType:    "agent",
				SenderName:    "alice",
				Content:       "files",
				ExtraMessages: dmMessages[0].ExtraMessages,
				CreatedAt:     now,
			},
		},
	}); err != nil {
		t.Fatalf("store.Create: %v", err)
	}

	page, err := store.ListAttachments(groupID, "", 50, 0)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("expected 2 group attachments from extra_messages, got %+v", page)
	}
	if page.Items[0].Agent != "alice" || page.Items[0].Path != "/home/chart.png" {
		t.Fatalf("unexpected first group attachment: %+v", page.Items[0])
	}
}
