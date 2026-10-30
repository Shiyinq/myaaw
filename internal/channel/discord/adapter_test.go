package discord

import (
	"context"
	"errors"
	"testing"
	"time"

	"myaaw/internal/channel"
)

func TestOpenGatewayRetriesThenSucceeds(t *testing.T) {
	attempts := 0
	open := func() error {
		attempts++
		if attempts < 3 {
			return errors.New("network unreachable")
		}
		return nil
	}

	var sleeps []time.Duration
	sleep := func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}

	if err := openGateway(context.Background(), open, sleep); err != nil {
		t.Fatalf("openGateway: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if len(sleeps) != 2 || sleeps[0] != time.Second || sleeps[1] != 2*time.Second {
		t.Fatalf("backoff = %v, want [1s 2s]", sleeps)
	}
}

func TestOpenGatewayStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	open := func() error {
		attempts++
		return errors.New("network unreachable")
	}
	sleep := func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}

	err := openGateway(ctx, open, sleep)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestOpenGatewayGivesUpOnAuthError(t *testing.T) {
	open := func() error {
		return errors.New("websocket: close 4004: Authentication failed")
	}
	sleep := func(context.Context, time.Duration) error {
		t.Fatal("authentication failures must not be retried")
		return nil
	}

	if err := openGateway(context.Background(), open, sleep); err == nil {
		t.Fatal("expected an authentication error")
	}
}

func TestOpenGatewayReturnsErrorWhenContextAlreadyDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	open := func() error {
		t.Fatal("open must not run after cancellation")
		return nil
	}

	if err := openGateway(ctx, open, sleepContext); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestProtectRecoversFromPanic(t *testing.T) {
	protect("test handler", func() {
		panic("boom")
	})
}

func TestSendErrorWithoutTransportReturnsError(t *testing.T) {
	adapter := &DiscordAdapter{}
	msg := &channel.IncomingMessage{RawMeta: DiscordMeta{ChannelID: "chan"}}

	if err := adapter.SendError(msg, "failed"); err == nil {
		t.Fatal("expected an error when no transport is configured")
	}
}

func TestSendErrorRejectsUnexpectedMetadata(t *testing.T) {
	adapter := &DiscordAdapter{transport: &scriptedTransport{}}
	msg := &channel.IncomingMessage{RawMeta: "not-discord"}

	if err := adapter.SendError(msg, "failed"); err == nil {
		t.Fatal("expected an error for unexpected metadata")
	}
}
