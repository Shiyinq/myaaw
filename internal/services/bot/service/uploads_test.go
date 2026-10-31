package service

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/channel"
)

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
