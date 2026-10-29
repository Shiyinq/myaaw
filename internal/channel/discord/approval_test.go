package discord

import (
	"testing"

	"myaaw/internal/agent/approvals"
)

func TestParseApprovalCustomID(t *testing.T) {
	cases := map[string]approvals.Decision{
		"apv:abc:y": approvals.AllowOnce,
		"apv:abc:s": approvals.AllowSession,
		"apv:abc:a": approvals.AllowAlways,
		"apv:abc:n": approvals.Deny,
	}
	for data, want := range cases {
		id, got, ok := parseApprovalCustomID(data)
		if !ok || id != "abc" || got != want {
			t.Errorf("%q => id=%q decision=%q ok=%v, want %q", data, id, got, ok, want)
		}
	}

	for _, data := range []string{"", "apv:", "apv:abc", "apv::y", "other:abc:y", "apv:abc:x"} {
		if _, _, ok := parseApprovalCustomID(data); ok {
			t.Errorf("%q should not parse", data)
		}
	}
}
