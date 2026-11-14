package main

import (
	"encoding/json"
	"testing"
	"time"

	"myaaw/internal/channel"
)

func TestMessageBatcherGroupsAndFlushesAfterTheWindow(t *testing.T) {
	clock := time.Unix(1000, 0)
	b := newMessageBatcher(time.Second)
	b.now = func() time.Time { return clock }

	b.add("target:telegram:42:", "telegram", json.RawMessage(`{"message":{"text":"one"}}`))
	clock = clock.Add(400 * time.Millisecond)
	b.add("target:telegram:42:", "telegram", json.RawMessage(`{"message":{"text":"two"}}`))
	b.add("target:discord:chan:", "discord", json.RawMessage(`{"message":{"text":"other"}}`))

	clock = clock.Add(900 * time.Millisecond)
	if ready := b.due(nil); len(ready) != 0 {
		t.Fatalf("batches flushed before the quiet window elapsed: %d", len(ready))
	}

	clock = clock.Add(200 * time.Millisecond)
	ready := b.due(nil)
	if len(ready) != 2 {
		t.Fatalf("flushed batches = %d, want 2", len(ready))
	}

	byKey := map[string][]string{}
	for _, batch := range ready {
		var envelope channel.QueueEnvelope
		if err := json.Unmarshal(batch.envelope, &envelope); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		if len(envelope.Payloads) == 0 {
			t.Fatalf("batch %s has no payloads", batch.key)
		}
		for _, payload := range envelope.Payloads {
			var item struct {
				Message struct {
					Text string `json:"text"`
				} `json:"message"`
			}
			if err := json.Unmarshal(payload, &item); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			byKey[batch.key] = append(byKey[batch.key], item.Message.Text)
		}
	}

	if got := byKey["target:telegram:42:"]; len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("telegram batch = %v, want one then two", got)
	}
	if got := byKey["target:discord:chan:"]; len(got) != 1 || got[0] != "other" {
		t.Fatalf("discord batch = %v", got)
	}
}

func TestMessageBatcherQuietWindowRestartsAfterEachMessage(t *testing.T) {
	clock := time.Unix(2000, 0)
	b := newMessageBatcher(time.Second)
	b.now = func() time.Time { return clock }

	b.add("user:1", "telegram", json.RawMessage(`{"n":1}`))
	clock = clock.Add(900 * time.Millisecond)
	b.add("user:1", "telegram", json.RawMessage(`{"n":2}`))

	clock = clock.Add(900 * time.Millisecond)
	if ready := b.due(nil); len(ready) != 0 {
		t.Fatal("the second message did not move the window forward")
	}

	clock = clock.Add(200 * time.Millisecond)
	ready := b.due(nil)
	if len(ready) != 1 {
		t.Fatalf("flushed batches = %d, want 1", len(ready))
	}
	var envelope channel.QueueEnvelope
	if err := json.Unmarshal(ready[0].envelope, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if len(envelope.Payloads) != 2 {
		t.Fatalf("payloads = %d, want both messages", len(envelope.Payloads))
	}
}

func TestMessageBatcherHoldsABatchWhileTheConversationIsBusy(t *testing.T) {
	clock := time.Unix(4000, 0)
	b := newMessageBatcher(time.Second)
	b.now = func() time.Time { return clock }

	b.add("user:1", "telegram", json.RawMessage(`{"n":1}`))
	clock = clock.Add(2 * time.Second)

	busy := true
	if ready := b.due(func(string) bool { return busy }); len(ready) != 0 {
		t.Fatal("a busy conversation flushed a second answer")
	}

	busy = false
	ready := b.due(func(string) bool { return busy })
	if len(ready) != 1 {
		t.Fatalf("flushed batches = %d, want 1 once the conversation is idle", len(ready))
	}
}

// TestMessageBatcherFlushesIntoARealDispatcher guards the wiring between the
// quiet window and the dispatcher: the predicate must hold a batch while the key
// is busy and release it as soon as the key is idle.
func TestMessageBatcherFlushesIntoARealDispatcher(t *testing.T) {
	started := make(chan string, 2)
	d := newKeyedDispatcher(2, func(job []byte) { started <- string(job) })

	clock := time.Unix(5000, 0)
	b := newMessageBatcher(time.Second)
	b.now = func() time.Time { return clock }

	b.add("target:discord:1:", "discord", json.RawMessage(`{"n":1}`))
	clock = clock.Add(2 * time.Second)

	ready := b.due(d.Busy)
	if len(ready) != 1 {
		t.Fatalf("flushed batches = %d, want 1 for an idle dispatcher", len(ready))
	}
	d.Submit(ready[0].key, ready[0].envelope)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("the flushed batch never reached the dispatcher")
	}
}

func TestMessageBatcherClearDropsPendingMessages(t *testing.T) {
	clock := time.Unix(3000, 0)
	b := newMessageBatcher(time.Second)
	b.now = func() time.Time { return clock }

	b.add("user:1", "telegram", json.RawMessage(`{"n":1}`))
	b.clear("user:1")
	clock = clock.Add(2 * time.Second)

	if ready := b.due(nil); len(ready) != 0 {
		t.Fatal("a cleared batch was still flushed")
	}
}
