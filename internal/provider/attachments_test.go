package provider

import (
	"strings"
	"testing"
)

func TestStripAttachmentBlock(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "block before text",
			in:   "<attached>\n[read them with the filesystem or bash tools]\n/tmp/a.txt\n/tmp/b.csv\n</attached>\n\nsummarize these",
			want: "summarize these",
		},
		{
			name: "block only",
			in:   "<attached>\n/tmp/a.txt\n</attached>",
			want: "",
		},
		{
			name: "no block",
			in:   "just a question",
			want: "just a question",
		},
		{
			name: "unclosed tag is left alone",
			in:   "<attached>\n/tmp/a.txt",
			want: "<attached>\n/tmp/a.txt",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripAttachmentBlock(tc.in); got != tc.want {
				t.Fatalf("StripAttachmentBlock(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripContextBlocks(t *testing.T) {
	userinfo := "<userinfo>\nPlatform: API · 2026-11-04 14:03\n</userinfo>"
	attached := "<attached>\n/tmp/a.txt\n</attached>"

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"both blocks", userinfo + "\n\n" + attached + "\n\nsummarize", "summarize"},
		{"only userinfo", userinfo + "\n\nhello", "hello"},
		{"only attached", attached + "\n\nhello", "hello"},
		{"plain text", "hello", "hello"},
		{"unclosed userinfo is left alone", "<userinfo>\nhello", "<userinfo>\nhello"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripContextBlocks(tc.in); got != tc.want {
				t.Fatalf("StripContextBlocks(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSplitAndJoinContextBlocks(t *testing.T) {
	userinfo := "<userinfo>\nPlatform: API\n</userinfo>"
	attached := "<attached>\n/tmp/a.txt\n</attached>"

	blocks, body := SplitContextBlocks(userinfo + "\n\n" + attached + "\n\nhello there")
	if body != "hello there" {
		t.Fatalf("body = %q", body)
	}
	if !strings.HasPrefix(blocks, "<userinfo>") || !strings.Contains(blocks, "<attached>") {
		t.Fatalf("blocks = %q", blocks)
	}

	// Editing replaces only the body and keeps the blocks in the same order.
	joined := JoinContextBlocks(blocks, "edited")
	if !strings.HasPrefix(joined, userinfo) || !strings.Contains(joined, attached) || !strings.HasSuffix(joined, "\n\nedited") {
		t.Fatalf("joined = %q", joined)
	}

	if got := JoinContextBlocks("", "edited"); got != "edited" {
		t.Fatalf("JoinContextBlocks without blocks = %q", got)
	}
}
