package approvals

import (
	"context"
	"testing"
	"time"
)

// A sub-agent approval has to be visible to any connected client and answerable
// through the shared registry, because the run no longer owns a live stream.
func TestBroadcastRequesterPublishesAndResolves(t *testing.T) {
	events, cancel := DefaultBroker().Subscribe()
	defer cancel()

	request := Request{
		ID:             "broadcast-test-1",
		RunID:          "run-1",
		ConversationID: "conv-1",
		ToolName:       "bash",
		UserID:         "7",
		SubAgent:       "researcher",
	}

	decided := make(chan Decision, 1)
	go func() {
		decision, _ := (BroadcastRequester{}).Request(context.Background(), request)
		decided <- decision
	}()

	select {
	case event := <-events:
		if event.Resolved || event.ID != request.ID || event.ConversationID != "conv-1" {
			t.Fatalf("required event = %+v", event)
		}
		if event.SubAgent != "researcher" {
			t.Fatalf("subagent = %q", event.SubAgent)
		}
	case <-time.After(time.Second):
		t.Fatal("no approval.required event")
	}

	resolved, found := DefaultRegistry().Resolve(
		request.ID, AllowOnce, Actor{Channel: "api", UserID: "7"},
	)
	if !found || !resolved {
		t.Fatalf("resolve = found:%v resolved:%v", found, resolved)
	}

	select {
	case decision := <-decided:
		if decision != AllowOnce {
			t.Fatalf("decision = %q, want allow_once", decision)
		}
	case <-time.After(time.Second):
		t.Fatal("requester did not return after the decision")
	}

	select {
	case event := <-events:
		if !event.Resolved || event.Decision != string(AllowOnce) {
			t.Fatalf("resolved event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("no approval.resolved event")
	}
}
