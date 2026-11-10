package api

import (
	"encoding/base64"
	"strings"
	"testing"

	"myaaw/internal/channel"
)

func TestParseIncomingFiles(t *testing.T) {
	adapter := NewAPIAdapter()

	data := base64.StdEncoding.EncodeToString([]byte("a,b,c\n1,2,3\n"))

	msg, err := adapter.ParseIncoming([]byte(
		`{"files":[{"name":"report.csv","mime_type":"text/csv","data_base64":"` + data + `"}]}`,
	))
	if err != nil {
		t.Fatalf("ParseIncoming files: %v", err)
	}
	if len(msg.Files) != 1 || msg.Files[0].Name != "report.csv" {
		t.Fatalf("files = %+v", msg.Files)
	}

	// Files alone are valid, like images.
	if _, err := adapter.ParseIncoming([]byte(`{}`)); err == nil {
		t.Fatal("empty message should be rejected")
	}

	// Path traversal in a client name is stripped to the base name.
	msg, err = adapter.ParseIncoming([]byte(
		`{"files":[{"name":"../../secret.txt","data_base64":"` + data + `"}]}`,
	))
	if err != nil {
		t.Fatalf("traversal name should be sanitized, not rejected: %v", err)
	}
	if msg.Files[0].Name != "secret.txt" {
		t.Fatalf("sanitized name = %q", msg.Files[0].Name)
	}

	if _, err := adapter.ParseIncoming([]byte(`{"files":[{"name":"a.bin","data_base64":"AAAA"}]}`)); err == nil {
		t.Fatal("an unsupported extension should be rejected")
	}
	if _, err := adapter.ParseIncoming([]byte(`{"files":[{"name":"x.txt","data_base64":"not-base64"}]}`)); err == nil {
		t.Fatal("bad base64 should be rejected")
	}
	if _, err := adapter.ParseIncoming([]byte(`{"files":[{"name":"","data_base64":"` + data + `"}]}`)); err == nil {
		t.Fatal("an empty name should be rejected")
	}
}

func TestValidateFilesLimits(t *testing.T) {
	one := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))

	// Too many files.
	many := make([]channel.FileUpload, maxFiles+1)
	for i := range many {
		many[i] = channel.FileUpload{Name: "x.txt", DataBase64: one}
	}
	if err := validateFiles(many); err == nil {
		t.Fatal("too many files should be rejected")
	}

	// A file over the per-file limit is rejected without writing anything.
	big := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", maxFileBytes+1)))
	if err := validateFiles([]channel.FileUpload{{Name: "big.txt", DataBase64: big}}); err == nil {
		t.Fatal("oversized file should be rejected")
	}
}
