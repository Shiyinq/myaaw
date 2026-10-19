package service

import (
	"encoding/json"
	"testing"
)

func TestInjectUserID(t *testing.T) {
	got := injectUserID([]byte(`{"text":"hi"}`), 7)
	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal: %v, body = %s", err, got)
	}
	if payload["user_id"] != float64(7) || payload["text"] != "hi" {
		t.Fatalf("payload = %+v", payload)
	}

	got = injectUserID([]byte(`{"user_id":0,"text":"hi"}`), 7)
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal zero user: %v, body = %s", err, got)
	}
	if payload["user_id"] != float64(7) {
		t.Fatalf("zero user_id should be replaced: %+v", payload)
	}

	explicit := `{"user_id":9,"text":"hi"}`
	got = injectUserID([]byte(explicit), 7)
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal explicit user: %v, body = %s", err, got)
	}
	if payload["user_id"] != float64(7) {
		t.Fatalf("body user_id must be overridden by the control-plane user: %+v", payload)
	}

	invalid := `not json`
	got = injectUserID([]byte(invalid), 7)
	if string(got) != invalid {
		t.Fatalf("invalid JSON should be forwarded unchanged, got %s", got)
	}
}
