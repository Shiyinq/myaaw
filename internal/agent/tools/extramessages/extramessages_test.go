package extramessages

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/agent/tools"
	"myaaw/internal/attachment"
)

func TestExtraMessagesMultiMediaAndInteractive(t *testing.T) {
	home := t.TempDir()
	imgPath := filepath.Join(home, "chart.png")
	if err := os.WriteFile(imgPath, []byte("fake-png-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	tool := NewWithRoots(attachment.Roots{Home: home})
	args := map[string]any{
		"blocks": []any{
			map[string]any{
				"type": "media",
				"data": map[string]any{
					"layout": "grid",
					"items": []any{
						map[string]any{"path": imgPath, "caption": "Local chart"},
						map[string]any{"url": "https://example.com/photo.jpg", "caption": "Online photo"},
					},
				},
			},
			map[string]any{
				"type": "buttons",
				"data": map[string]any{
					"prompt": "Pick an action",
					"buttons": []any{
						map[string]any{"id": "dl", "label": "Download", "type": "action", "value": "mp4", "style": "primary"},
						map[string]any{"label": "Docs", "type": "link", "url": "https://example.com/docs"},
					},
				},
			},
			map[string]any{
				"type": "form",
				"data": map[string]any{
					"title": "Feedback",
					"fields": []any{
						map[string]any{"name": "topic", "label": "Topic", "type": "text", "required": true},
						map[string]any{"name": "rating", "label": "Rating", "type": "select", "options": []any{
							map[string]any{"label": "Good", "value": "good"},
						}},
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(args)

	res := tool.CallToolStructured(context.Background(), string(raw), &tools.ToolsContext{Channel: "api"})
	if strings.HasPrefix(res.Text, "Error:") {
		t.Fatalf("unexpected error: %s", res.Text)
	}
	if len(res.ExtraMessages) != 1 {
		t.Fatalf("got %d extra messages, want 1", len(res.ExtraMessages))
	}
	blocks := res.ExtraMessages[0].Blocks
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(blocks))
	}
	if blocks[0].Interactive {
		t.Fatalf("media block must not be interactive")
	}
	if !blocks[1].Interactive || blocks[1].State == nil || blocks[1].State.Status != "pending" {
		t.Fatalf("buttons block with action button must be interactive & pending")
	}
	if !blocks[2].Interactive || blocks[2].State == nil || blocks[2].State.Status != "pending" {
		t.Fatalf("form block must be interactive & pending")
	}
}

func TestExtraMessagesSecurityRejections(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	_ = os.WriteFile(outside, []byte("secret"), 0o600)

	tool := NewWithRoots(attachment.Roots{Home: home})

	// Reject local path outside allowed roots
	rawOutside, _ := json.Marshal(map[string]any{
		"blocks": []any{
			map[string]any{
				"type": "media",
				"data": map[string]any{"items": []any{map[string]any{"path": outside}}},
			},
		},
	})
	if res := tool.CallToolStructured(context.Background(), string(rawOutside), nil); !strings.HasPrefix(res.Text, "Error:") {
		t.Fatalf("expected outside path error, got: %s", res.Text)
	}

	// Reject javascript: URL
	rawJS, _ := json.Marshal(map[string]any{
		"blocks": []any{
			map[string]any{
				"type": "buttons",
				"data": map[string]any{"buttons": []any{map[string]any{"label": "Bad", "type": "link", "url": "javascript:alert(1)"}}},
			},
		},
	})
	if res := tool.CallToolStructured(context.Background(), string(rawJS), nil); !strings.HasPrefix(res.Text, "Error:") {
		t.Fatalf("expected javascript URL rejection, got: %s", res.Text)
	}
}
