package service

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/channel"
)

func TestSaveUserImages(t *testing.T) {
	userDir := t.TempDir()
	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n-fake-png"))
	jpeg := base64.StdEncoding.EncodeToString([]byte("\xff\xd8\xff\xe0-fake-jpeg"))

	uploads, err := saveUserImages(userDir, "conv-1", "msg-1", []string{png, jpeg})
	if err != nil {
		t.Fatalf("saveUserImages: %v", err)
	}
	if len(uploads) != 2 || uploads[0].Name != "image-1.png" || uploads[1].Name != "image-2.jpg" {
		t.Fatalf("uploads = %+v", uploads)
	}
	if uploads[0].MimeType != "image/png" || uploads[1].MimeType != "image/jpeg" {
		t.Fatalf("mime types = %q/%q", uploads[0].MimeType, uploads[1].MimeType)
	}

	dir := filepath.Join(userDir, "uploads", "conv-1", "msg-1")
	for _, want := range []string{"image-1.png", "image-2.jpg"} {
		// The extension follows the base64 signature: the dashboard re-encodes to
		// JPEG, an API client may well send PNG.
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Fatalf("%s missing: %v", want, err)
		}
	}

	encodedPNG := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n"))
	if got := imageExtension(encodedPNG); got != ".png" {
		t.Fatalf("png extension = %q", got)
	}
	if got := imageExtension(base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0xe0})); got != ".jpg" {
		t.Fatalf("jpeg extension = %q", got)
	}

	// No images means no directory either.
	if empty, err := saveUserImages(userDir, "conv-2", "msg-2", nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty save: %v (uploads=%+v)", err, empty)
	}
	if _, err := os.Stat(filepath.Join(userDir, "uploads", "conv-2")); !os.IsNotExist(err) {
		t.Fatal("empty images should not create a directory")
	}
}

func TestSaveUserFiles(t *testing.T) {
	userDir := t.TempDir()
	data := base64.StdEncoding.EncodeToString([]byte("hello,world\n"))

	files := []channel.FileUpload{
		{Name: "../report.csv", MimeType: "text/csv", DataBase64: data},
		{Name: "notes.txt", DataBase64: data},
		{Name: "notes.txt", DataBase64: data}, // duplicate name
	}

	uploads, err := saveUserFiles(userDir, "conv-1", "msg-1", files)
	if err != nil {
		t.Fatalf("saveUserFiles: %v", err)
	}
	if len(uploads) != 3 {
		t.Fatalf("uploads = %d, want 3", len(uploads))
	}

	// Path traversal is stripped and duplicates get a suffix.
	if uploads[0].Name != "report.csv" {
		t.Fatalf("first name = %q", uploads[0].Name)
	}
	if uploads[1].Name != "notes.txt" || uploads[2].Name != "notes-2.txt" {
		t.Fatalf("dedupe names = %q, %q", uploads[1].Name, uploads[2].Name)
	}

	base := filepath.Join(userDir, "uploads", "conv-1", "msg-1")
	for _, upload := range uploads {
		if !strings.HasPrefix(upload.Path, base) {
			t.Fatalf("path %q is outside %q", upload.Path, base)
		}
		if _, err := os.Stat(upload.Path); err != nil {
			t.Fatalf("stored file missing: %v", err)
		}
	}

	// No files means no directory and no metadata.
	empty, err := saveUserFiles(userDir, "conv-2", "msg-2", nil)
	if err != nil || empty != nil {
		t.Fatalf("empty upload = %v, %v", empty, err)
	}
	if _, err := os.Stat(filepath.Join(userDir, "uploads", "conv-2")); !os.IsNotExist(err) {
		t.Fatal("empty upload should not create a directory")
	}
}
