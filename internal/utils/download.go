package utils

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
)

// DownloadFileToBase64 downloads a file from URL and returns base64 string.
func DownloadFileToBase64(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read body: %w", err)
	}

	return base64.StdEncoding.EncodeToString(body), nil
}

// IsImage checks if content type is an image.
func IsImage(contentType string) bool {
	return contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/webp" || contentType == "image/gif"
}
