package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"myaaw/internal/channel"
)

func discordEnvelope(chatID, text string) []byte {
	return []byte(`{"channel":"discord","payload":{"discord_author":{"id":"7","username":"tester"},` +
		`"message":{"chat":{"id":"` + chatID + `","type":"guild"},"message_id":"m-` + text + `",` +
		`"text":"` + text + `"},"original_source":"discord"}}`)
}

func waitForJob(t *testing.T, jobs <-chan []byte) channel.QueueEnvelope {
	t.Helper()

	select {
	case job := <-jobs:
		var envelope channel.QueueEnvelope
		if err := json.Unmarshal(job, &envelope); err != nil {
			t.Fatalf("decode dispatched job: %v", err)
		}
		return envelope
	case <-time.After(2 * time.Second):
		t.Fatal("no job was dispatched")
		return channel.QueueEnvelope{}
	}
}

func payloadTexts(t *testing.T, envelope channel.QueueEnvelope) []string {
	t.Helper()

	texts := make([]string, 0, len(envelope.Payloads))
	for _, payload := range envelope.Payloads {
		var item struct {
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
		}
		if err := json.Unmarshal(payload, &item); err != nil {
			t.Fatalf("decode batched payload: %v", err)
		}
		texts = append(texts, item.Message.Text)
	}
	return texts
}

// TestRouterBatchesABurstIntoOneTurn is the regression guard for the wiring
// between the quiet window and the dispatcher: the batch must leave the consumer
// as soon as the conversation is idle, and it must carry the channel payload of
// every message rather than the queue envelope around it.
func TestRouterBatchesABurstIntoOneTurn(t *testing.T) {
	jobs := make(chan []byte, 8)
	dispatch := newKeyedDispatcher(2, func(job []byte) { jobs <- job })
	router := newMessageRouter(dispatch, 30*time.Millisecond)

	cancelled := make(chan string, 8)
	router.cancel = func(key string) bool {
		cancelled <- key
		return false
	}

	router.route(discordEnvelope("111", "hi"))
	router.route(discordEnvelope("111", "there"))
	router.route(discordEnvelope("222", "other chat"))

	// Nothing may leave before the quiet window elapses.
	if ready := router.batcher.due(nil); len(ready) != 0 {
		t.Fatalf("flushed %d batches before the window elapsed", len(ready))
	}

	time.Sleep(120 * time.Millisecond)
	router.flush()

	batches := make([][]string, 0, 2)
	for i := 0; i < 2; i++ {
		envelope := waitForJob(t, jobs)
		if len(envelope.Payload) > 0 {
			t.Fatalf("batched job carried a single payload: %s", envelope.Payload)
		}
		texts := payloadTexts(t, envelope)
		if len(texts) == 0 {
			t.Fatal("the batch carried the queue envelope instead of the channel payload")
		}
		batches = append(batches, texts)
	}

	burstFound, otherFound := false, false
	for _, texts := range batches {
		if len(texts) == 2 && texts[0] == "hi" && texts[1] == "there" {
			burstFound = true
		}
		if len(texts) == 1 && texts[0] == "other chat" {
			otherFound = true
		}
	}
	if !burstFound || !otherFound {
		t.Fatalf("batches = %v, want one chat with both messages and another with its own", batches)
	}

	// Every message of the burst cancels the running turn, so the agent answers
	// the burst once instead of leaving the old answer streaming.
	close(cancelled)
	cancels := 0
	for key := range cancelled {
		cancels++
		if !strings.HasPrefix(key, "target:discord:") {
			t.Fatalf("cancel key = %q", key)
		}
	}
	if cancels != 3 {
		t.Fatalf("cancel calls = %d, want one per plain message", cancels)
	}
}

func TestRouterStopSkipsTheBatchAndClearsIt(t *testing.T) {
	jobs := make(chan []byte, 8)
	dispatch := newKeyedDispatcher(1, func(job []byte) { jobs <- job })
	router := newMessageRouter(dispatch, time.Minute)
	router.cancel = func(string) bool { return false }

	router.route(discordEnvelope("111", "hi"))
	router.route(discordEnvelope("111", "!stop"))

	envelope := waitForJob(t, jobs)
	if len(envelope.Payload) == 0 || !strings.Contains(string(envelope.Payload), "!stop") {
		t.Fatalf("stop job = %s", envelope.Payload)
	}
	if ready := router.batcher.due(nil); len(ready) != 0 {
		t.Fatal("a stop command left the pending batch in place")
	}
}

func TestRouterSendsSlashCommandsStraightThrough(t *testing.T) {
	jobs := make(chan []byte, 8)
	dispatch := newKeyedDispatcher(1, func(job []byte) { jobs <- job })
	router := newMessageRouter(dispatch, time.Minute)
	router.cancel = func(string) bool { return false }

	router.route(discordEnvelope("111", "/new"))

	envelope := waitForJob(t, jobs)
	if len(envelope.Payloads) != 0 {
		t.Fatalf("a slash command was batched: %+v", envelope)
	}
	if !strings.Contains(string(envelope.Payload), "/new") {
		t.Fatalf("command payload = %s", envelope.Payload)
	}
	if ready := router.batcher.due(nil); len(ready) != 0 {
		t.Fatal("a slash command entered the batch")
	}
}
