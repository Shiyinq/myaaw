package telegram

import (
	"testing"

	"myaaw/internal/agent/approvals"
)

func TestParseApprovalCallback(t *testing.T) {
	cases := map[string]approvals.Decision{
		"apv:abc:y": approvals.AllowOnce,
		"apv:abc:s": approvals.AllowSession,
		"apv:abc:a": approvals.AllowAlways,
		"apv:abc:n": approvals.Deny,
	}
	for data, want := range cases {
		id, got, ok := parseApprovalCallback(data)
		if !ok || id != "abc" || got != want {
			t.Errorf("%q => id=%q decision=%q ok=%v, want %q", data, id, got, ok, want)
		}
	}

	for _, data := range []string{"", "apv:", "apv:abc", "apv::y", "other:abc:y", "apv:abc:x"} {
		if _, _, ok := parseApprovalCallback(data); ok {
			t.Errorf("%q should not parse", data)
		}
	}
}

func TestHandleApprovalCallbackResolves(t *testing.T) {
	const id = "callback-test"
	ch := ApprovalRegistry.Register(id)
	defer ApprovalRegistry.Remove(id)

	if !HandleApprovalCallback([]byte(`{"callback_query":{"id":"cb","data":"apv:` + id + `:a"}}`)) {
		t.Fatal("callback query not recognized")
	}
	if got := <-ch; got != approvals.AllowAlways {
		t.Fatalf("decision = %q", got)
	}
}

func TestHandleApprovalIgnoresMessages(t *testing.T) {
	if HandleApprovalCallback([]byte(`{"message":{"text":"hi"}}`)) {
		t.Fatal("plain message should not be treated as a callback")
	}
}
