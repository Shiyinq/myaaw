package provider

import "testing"

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
