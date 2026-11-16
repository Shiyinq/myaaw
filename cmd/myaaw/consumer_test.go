package main

import "testing"

func TestTurnKeyFromBody(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "telegram dm",
			body: `{"channel":"telegram","payload":{"message":{"chat":{"id":123}}}}`,
			want: "target:telegram:123:",
		},
		{
			name: "telegram group",
			body: `{"channel":"telegram","payload":{"message":{"chat":{"id":-1001111}}}}`,
			want: "target:telegram:-1001111:",
		},
		{
			name: "telegram general topic",
			body: `{"channel":"telegram","payload":{"message":{"message_thread_id":1,"chat":{"id":-1001111}}}}`,
			want: "target:telegram:-1001111:",
		},
		{
			name: "telegram forum topic",
			body: `{"channel":"telegram","payload":{"message":{"message_thread_id":77,"chat":{"id":-1001111}}}}`,
			want: "target:telegram:-1001111:77",
		},
		{
			name: "discord channel",
			body: `{"channel":"discord","payload":{"message":{"chat":{"id":"999888"}}}}`,
			want: "target:discord:999888:",
		},
		{
			name: "missing chat id",
			body: `{"channel":"telegram","payload":{}}`,
			want: "",
		},
		{
			name: "invalid json",
			body: `not-json`,
			want: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := turnKeyFromBody([]byte(c.body)); got != c.want {
				t.Fatalf("turnKeyFromBody = %q, want %q", got, c.want)
			}
		})
	}
}

func TestParseEnvelopeRoutesStopAndInterrupts(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		stop       bool
		interrupts bool
	}{
		{
			name: "telegram slash stop",
			body: `{"channel":"telegram","payload":{"message":{"text":"/stop"}}}`,
			stop: true,
		},
		{
			name: "telegram bang stop",
			body: `{"channel":"telegram","payload":{"message":{"text":"!stop"}}}`,
			stop: true,
		},
		{
			name: "telegram mention stop",
			body: `{"channel":"telegram","payload":{"message":{"text":"/stop@AnyBotName"}}}`,
			stop: true,
		},
		{
			name: "discord bang stop",
			body: `{"channel":"discord","payload":{"message":{"text":"!stop"}}}`,
			stop: true,
		},
		{
			name:       "telegram text interrupts",
			body:       `{"channel":"telegram","payload":{"message":{"text":"hello"}}}`,
			interrupts: true,
		},
		{
			name:       "discord text interrupts",
			body:       `{"channel":"discord","payload":{"message":{"text":"hello"}}}`,
			interrupts: true,
		},
		{
			name: "slash commands keep their turn",
			body: `{"channel":"telegram","payload":{"message":{"text":"/new"}}}`,
		},
		{
			name: "api messages keep their turn",
			body: `{"channel":"api","payload":{"message":{"text":"hello"}}}`,
		},
		{
			name: "invalid json",
			body: `not-json`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseEnvelope([]byte(c.body))
			if got.stop != c.stop {
				t.Fatalf("stop = %v, want %v", got.stop, c.stop)
			}
			if got.interrupts != c.interrupts {
				t.Fatalf("interrupts = %v, want %v", got.interrupts, c.interrupts)
			}
		})
	}
}
