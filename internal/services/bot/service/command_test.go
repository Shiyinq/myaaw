package service

import (
	"context"
	"errors"
	"testing"

	"myaaw/internal/agent"
	"myaaw/internal/channel"
	"myaaw/internal/common"
	"myaaw/internal/services/bot/model"
)

func TestStopCommandCancelsTheRunningTurn(t *testing.T) {
	svc := &BotServiceImpl{runs: &runRegistry{}}
	user := &model.User{UserId: 1}
	msg := &channel.IncomingMessage{Channel: "telegram", ChatID: "42"}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	key := conversationLockKey(msg)
	turn := svc.turnRegistry().register(key, cancel)
	if turn == nil {
		t.Fatal("the turn was not registered")
	}
	defer svc.turnRegistry().unregister(key, turn)

	handled, text, err := svc.command(user, &channel.IncomingMessage{
		Channel: "telegram",
		ChatID:  "42",
		Text:    "!stop",
	})
	if err != nil || !handled {
		t.Fatalf("command(!stop) = %v, %v", handled, err)
	}
	if text != common.CommandStopped() {
		t.Fatalf("reply = %q, want the stop acknowledgement", text)
	}
	if cause := context.Cause(ctx); !errors.Is(cause, agent.ErrRunStopped) {
		t.Fatalf("cancel cause = %v, want ErrRunStopped", cause)
	}
}

func TestStopCommandWithoutARunningTurn(t *testing.T) {
	svc := &BotServiceImpl{runs: &runRegistry{}}
	handled, text, err := svc.command(&model.User{UserId: 1}, &channel.IncomingMessage{
		Channel: "telegram",
		ChatID:  "42",
		Text:    "/stop",
	})
	if err != nil || !handled {
		t.Fatalf("command(/stop) = %v, %v", handled, err)
	}
	if text != common.CommandStopNothing() {
		t.Fatalf("reply = %q, want the nothing-running message", text)
	}
}

func TestBangPrefixOnlyClaimsKnownCommands(t *testing.T) {
	svc := &BotServiceImpl{runs: &runRegistry{}}
	user := &model.User{UserId: 1}

	// A normal message that starts with "!" must still reach the model.
	handled, _, err := svc.command(user, &channel.IncomingMessage{
		Channel: "telegram",
		ChatID:  "42",
		Text:    "!important detail here",
	})
	if err != nil || handled {
		t.Fatalf("command(!important) = %v, %v; want it treated as text", handled, err)
	}

	// "/" keeps reporting unknown commands, as before.
	handled, text, err := svc.command(user, &channel.IncomingMessage{
		Channel: "telegram",
		ChatID:  "42",
		Text:    "/notacommand",
	})
	if err != nil || !handled {
		t.Fatalf("command(/notacommand) = %v, %v", handled, err)
	}
	if text != common.CommandNotFound() {
		t.Fatalf("reply = %q, want the not-found message", text)
	}
}
