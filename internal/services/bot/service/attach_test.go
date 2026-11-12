package service

import (
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/attachment"
	"myaaw/internal/provider"
)

func attachRoots(t *testing.T) attachment.Roots {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	return attachment.Roots{
		Home:   home,
		User:   filepath.Join(root, "user"),
		Skills: filepath.Join(root, "skills"),
		Jobs:   filepath.Join(root, "jobs"),
	}
}

func attachFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDeliverAttachmentsAPIUsesVirtualPath(t *testing.T) {
	roots := attachRoots(t)
	video := attachFile(t, filepath.Join(roots.Home, "workspace", "clip.mp4"), "MP4")

	cleaned, attachments := attachmentsForChannel("api", "Here is the clip.\n@attach("+video+")\n", roots)

	// The directive stays in the text; the WebUI renders it as its path.
	if cleaned != "Here is the clip.\n@attach("+video+")\n" {
		t.Fatalf("cleaned = %q", cleaned)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %+v", attachments)
	}
	item := attachments[0]
	if item.Path != "/home/workspace/clip.mp4" {
		t.Errorf("path = %q", item.Path)
	}
	if item.Kind != "video" {
		t.Errorf("kind = %q", item.Kind)
	}
	if item.Size != 3 {
		t.Errorf("size = %d", item.Size)
	}
}

func TestDeliverAttachmentsAPIAutoPromotesBarePath(t *testing.T) {
	roots := attachRoots(t)
	video := attachFile(t, filepath.Join(roots.Home, "workspace", "clip.mp4"), "MP4")

	cleaned, attachments := attachmentsForChannel("api", "Ini videonya: "+video, roots)

	if cleaned != "Ini videonya: "+video {
		t.Errorf("cleaned = %q", cleaned)
	}
	if len(attachments) != 1 || attachments[0].Path != "/home/workspace/clip.mp4" {
		t.Fatalf("attachments = %+v", attachments)
	}
}

func TestDeliverAttachmentsAPICarriesNoBytes(t *testing.T) {
	roots := attachRoots(t)
	video := attachFile(t, filepath.Join(roots.Home, "workspace", "clip.mp4"), "MP4")

	_, attachments := attachmentsForChannel("api", "@attach("+video+")", roots)

	if len(attachments) != 1 {
		t.Fatalf("attachments = %+v", attachments)
	}
	if attachments[0].Path != "/home/workspace/clip.mp4" {
		t.Errorf("path = %q", attachments[0].Path)
	}
	if len(attachments[0].Data) != 0 {
		t.Errorf("API attachments must not carry bytes, got %d", len(attachments[0].Data))
	}
}

func TestDeliverAttachmentsMessagingReadsBytes(t *testing.T) {
	roots := attachRoots(t)
	image := attachFile(t, filepath.Join(roots.Home, "workspace", "shot.png"), "PNG")

	cleaned, attachments := attachmentsForChannel("telegram", "@attach("+image+")", roots)

	// Messaging surfaces cannot hide the directive, so it becomes the path.
	if cleaned != image {
		t.Errorf("cleaned = %q", cleaned)
	}
	if len(attachments) != 1 || string(attachments[0].Data) != "PNG" {
		t.Fatalf("attachments = %+v", attachments)
	}
	// The bytes go to the platform, while the path is persisted so the dashboard
	// can render the same file in chat history.
	if attachments[0].Path != "/home/workspace/shot.png" {
		t.Errorf("path = %q", attachments[0].Path)
	}
	if attachments[0].Kind != "image" || attachments[0].Size != 3 {
		t.Errorf("kind/size = %q/%d", attachments[0].Kind, attachments[0].Size)
	}
}

func TestDeliverAttachmentsCLISubstitutesPath(t *testing.T) {
	roots := attachRoots(t)
	image := attachFile(t, filepath.Join(roots.Home, "workspace", "shot.png"), "PNG")

	cleaned, attachments := attachmentsForChannel("cli", "See @attach("+image+")", roots)

	if cleaned != "See "+image {
		t.Errorf("cleaned = %q", cleaned)
	}
	if len(attachments) != 0 {
		t.Errorf("attachments = %+v", attachments)
	}
}

func TestDeliverAttachmentsCLILeavesUnresolved(t *testing.T) {
	roots := attachRoots(t)
	const text = "The file is @attach(/tmp/missing.png)"

	cleaned, attachments := attachmentsForChannel("cli", text, roots)

	if cleaned != text {
		t.Errorf("cleaned = %q", cleaned)
	}
	if len(attachments) != 0 {
		t.Errorf("attachments = %+v", attachments)
	}
}

func TestReplaceFinalAssistantText(t *testing.T) {
	transcript := []provider.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "old @attach(/a.png)"},
	}

	replaceFinalAssistantText(transcript, "old")

	if transcript[1].Content != "old" {
		t.Fatalf("content = %v", transcript[1].Content)
	}
}

func TestReplaceFinalAssistantTextMultimodal(t *testing.T) {
	transcript := []provider.Message{{
		Role: "assistant",
		Content: []provider.ContentItem{
			{Type: "text", Text: "old @attach(/a.png)"},
			{Type: "image_url"},
			{Type: "text", Text: "tail"},
		},
	}}

	replaceFinalAssistantText(transcript, "old")

	items, ok := transcript[0].Content.([]provider.ContentItem)
	if !ok {
		t.Fatalf("content = %T", transcript[0].Content)
	}
	if items[0].Text != "old" {
		t.Errorf("first text = %q", items[0].Text)
	}
	if items[1].Type != "image_url" {
		t.Errorf("image part was dropped: %+v", items[1])
	}
	if items[2].Text != "" {
		t.Errorf("trailing text = %q", items[2].Text)
	}
}
