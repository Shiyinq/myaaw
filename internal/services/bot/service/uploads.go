package service

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"myaaw/internal/channel"
	"myaaw/internal/provider"
)

// saveUserFiles writes one message's uploads under
// <userDir>/uploads/<conversationID>/<messageID>/ and returns their metadata.
// The client file name is never trusted as a path component.
func saveUserFiles(userDir, conversationID, messageID string, files []channel.FileUpload) ([]provider.Upload, error) {
	if len(files) == 0 {
		return nil, nil
	}

	dir := filepath.Join(userDir, "uploads", conversationID, messageID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create upload directory: %w", err)
	}

	uploads := make([]provider.Upload, 0, len(files))
	for _, file := range files {
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(file.DataBase64))
		if err != nil {
			return nil, fmt.Errorf("decode %q: %w", file.Name, err)
		}
		name := uniqueFileName(dir, safeFileName(file.Name))
		full := filepath.Join(dir, name)
		if err := os.WriteFile(full, data, 0o600); err != nil {
			return nil, fmt.Errorf("write %q: %w", name, err)
		}
		uploads = append(uploads, provider.Upload{
			Name:     name,
			Path:     full,
			MimeType: file.MimeType,
			Size:     int64(len(data)),
		})
	}
	return uploads, nil
}

func safeFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	parts := strings.Split(name, "/")
	name = parts[len(parts)-1]
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	return name
}

// uniqueFileName returns a name that does not collide inside dir, appending
// "-2", "-3", ... before the extension when needed.
func uniqueFileName(dir, name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	candidate := name
	for n := 2; ; n++ {
		if _, err := os.Lstat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d%s", stem, n, ext)
	}
}
