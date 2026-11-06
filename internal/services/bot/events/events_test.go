package events

import (
	"testing"
	"time"
)

func TestBrokerFansOutAndCancels(t *testing.T) {
	broker := newBroker()
	first, cancelFirst := broker.Subscribe()
	defer cancelFirst()
	second, cancelSecond := broker.Subscribe()
	defer cancelSecond()

	broker.Publish(TurnEvent{ConversationID: "conv-1", Trigger: "cron", At: "2026-11-06T18:00:00Z"})

	for name, ch := range map[string]<-chan TurnEvent{"first": first, "second": second} {
		select {
		case event := <-ch:
			if event.ConversationID != "conv-1" || event.Trigger != "cron" {
				t.Fatalf("%s event = %+v", name, event)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s did not receive the event", name)
		}
	}

	// An empty conversation id is not re-readable, so it is dropped.
	broker.Publish(TurnEvent{Trigger: "cron"})
	select {
	case event := <-first:
		t.Fatalf("empty event was published: %+v", event)
	case <-time.After(50 * time.Millisecond):
	}

	// Cancelling closes that subscriber and stops delivery to it.
	cancelSecond()
	broker.Publish(TurnEvent{ConversationID: "conv-2", Trigger: "heartbeat"})
	select {
	case event := <-first:
		if event.ConversationID != "conv-2" {
			t.Fatalf("first event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("first subscriber stopped receiving events")
	}
	select {
	case _, ok := <-second:
		if ok {
			t.Fatal("cancelled subscriber still received an event")
		}
	case <-time.After(50 * time.Millisecond):
	}
}
