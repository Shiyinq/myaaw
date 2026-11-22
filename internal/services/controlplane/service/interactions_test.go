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
