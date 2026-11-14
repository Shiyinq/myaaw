package service

import (
	"context"
	"errors"
	"testing"

	"myaaw/internal/agent"
	"myaaw/internal/channel"
)

func TestRunRegistryCancelStopsTheTurn(t *testing.T) {
	var registry runRegistry
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	turn := registry.register("target:telegram:1:", cancel)
	if turn == nil {
		t.Fatal("register returned no handle")
	}
	if !registry.cancel("target:telegram:1:") {
		t.Fatal("cancel reported no running turn")
	}
	if cause := context.Cause(ctx); !errors.Is(cause, agent.ErrRunStopped) {
		t.Fatalf("cancel cause = %v, want ErrRunStopped", cause)
	}
	if registry.cancel("target:telegram:1:") {
		t.Fatal("a second cancel re-stopped an already stopped turn")
	}
	registry.unregister("target:telegram:1:", turn)
}

func TestRunRegistryCancelWithoutTurn(t *testing.T) {
	var registry runRegistry
	if registry.cancel("target:telegram:1:") {
		t.Fatal("cancel reported a turn in an empty registry")
	}
}

func TestRunRegistryStaleUnregisterKeepsTheNewerTurn(t *testing.T) {
	var registry runRegistry

	_, cancelFirst := context.WithCancelCause(context.Background())
	defer cancelFirst(nil)
	first := registry.register("conversation:conv-1", cancelFirst)

	secondCtx, cancelSecond := context.WithCancelCause(context.Background())
	defer cancelSecond(nil)
	registry.register("conversation:conv-1", cancelSecond)

	// The older turn finishes after the newer one registered its entry.
	registry.unregister("conversation:conv-1", first)
	if _, ok := registry.runs["conversation:conv-1"]; !ok {
		t.Fatal("a stale unregister removed the newer turn")
	}
	if !registry.cancel("conversation:conv-1") {
		t.Fatal("the newer turn was not cancellable")
	}
	if cause := context.Cause(secondCtx); !errors.Is(cause, agent.ErrRunStopped) {
		t.Fatalf("cancel cause = %v, want ErrRunStopped", cause)
	}
}

func TestStoppableTurnScopesChannelTurns(t *testing.T) {
	cases := []struct {
		name string
		msg  *channel.IncomingMessage
		want bool
	}{
		{name: "telegram user", msg: &channel.IncomingMessage{Channel: "telegram"}, want: true},
		{name: "discord user", msg: &channel.IncomingMessage{Channel: "Discord"}, want: true},
		{name: "triggered turn", msg: &channel.IncomingMessage{Channel: "telegram", TriggerType: "cron"}, want: false},
		{name: "api", msg: &channel.IncomingMessage{Channel: "api"}, want: false},
		{name: "nil message", msg: nil, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stoppableTurn(tc.msg); got != tc.want {
				t.Fatalf("stoppableTurn(%+v) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}
