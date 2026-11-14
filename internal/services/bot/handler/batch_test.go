package handler

import (
	"encoding/json"
	"testing"

	"myaaw/internal/channel"
	"myaaw/internal/channel/api"
	"myaaw/internal/channel/discord"
)

func TestParseEnvelopeMessagesKeepsEveryBatchedPayload(t *testing.T) {
	adapter := api.NewAPIAdapter()

	envelope := channel.QueueEnvelope{
		Channel: "api",
		Payloads: []json.RawMessage{
			json.RawMessage(`{"user_id":7,"text":"one"}`),
			json.RawMessage(`{"user_id":7,"text":"two"}`),
			json.RawMessage(`{"user_id":7,"text":"three"}`),
		},
	}

	msg, err := parseEnvelopeMessages(adapter, envelope)
	if err != nil {
		t.Fatalf("parseEnvelopeMessages: %v", err)
	}
	if msg.Text != "one" {
		t.Fatalf("primary text = %q, want the oldest message", msg.Text)
	}
	if len(msg.Batch) != 2 {
		t.Fatalf("batch = %d, want 2 extra messages", len(msg.Batch))
	}
	if msg.Batch[0].Text != "two" || msg.Batch[1].Text != "three" {
		t.Fatalf("batch order = %q, %q", msg.Batch[0].Text, msg.Batch[1].Text)
	}
}

func TestParseEnvelopeMessagesSkipsOnlyTheUnreadablePayload(t *testing.T) {
	adapter := api.NewAPIAdapter()

	msg, err := parseEnvelopeMessages(adapter, channel.QueueEnvelope{
		Payloads: []json.RawMessage{
			json.RawMessage(`not-json`),
			json.RawMessage(`{"user_id":7,"text":"kept"}`),
		},
	})
	if err != nil {
		t.Fatalf("parseEnvelopeMessages: %v", err)
	}
	if msg.Text != "kept" {
		t.Fatalf("primary text = %q", msg.Text)
	}
	if len(msg.Batch) != 0 {
		t.Fatalf("batch = %d, want none", len(msg.Batch))
	}

	if _, err := parseEnvelopeMessages(adapter, channel.QueueEnvelope{
		Payloads: []json.RawMessage{json.RawMessage(`not-json`)},
	}); err == nil {
		t.Fatal("a batch without any readable message should fail")
	}
}

func TestParseEnvelopeMessagesReadsARealDiscordBurst(t *testing.T) {
	payload := func(id, text string) json.RawMessage {
		return json.RawMessage(`{"discord_author":{"id":"7","username":"tester"},` +
			`"message":{"chat":{"id":"431","type":"guild"},"message_id":"` + id + `",` +
			`"text":"` + text + `"},"original_source":"discord"}`)
	}

	msg, err := parseEnvelopeMessages(&discord.DiscordAdapter{}, channel.QueueEnvelope{
		Channel:  "discord",
		Payloads: []json.RawMessage{payload("1", "first"), payload("2", "second")},
	})
	if err != nil {
		t.Fatalf("parseEnvelopeMessages: %v", err)
	}
	if msg.Text != "first" || msg.ChatID != "431" || msg.UserID != 7 {
		t.Fatalf("primary = %+v", msg)
	}
	if len(msg.Batch) != 1 || msg.Batch[0].Text != "second" {
		t.Fatalf("batch = %+v", msg.Batch)
	}
}

func TestParseEnvelopeMessagesKeepsTheSinglePayloadPath(t *testing.T) {
	adapter := api.NewAPIAdapter()

	msg, err := parseEnvelopeMessages(adapter, channel.QueueEnvelope{
		Channel: "api",
		Payload: json.RawMessage(`{"user_id":7,"text":"alone"}`),
	})
	if err != nil {
		t.Fatalf("parseEnvelopeMessages: %v", err)
	}
	if msg.Text != "alone" || len(msg.Batch) != 0 {
		t.Fatalf("msg = %+v", msg)
	}
}
