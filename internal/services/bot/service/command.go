package service

import (
	"myaaw/internal/channel"
	"myaaw/internal/common"
	"myaaw/internal/services/bot/model"
	"myaaw/internal/utils"
)

// CommandContext carries what a command needs: the user record, the incoming
// message, and the conversation scope (empty for DM/CLI/API).
type CommandContext struct {
	User  *model.User
	Msg   *channel.IncomingMessage
	Scope string
}

type CommandFactory interface {
	HandleCommand(ctx CommandContext, args string) (bool, string, error)
}

type StartCommand struct {
	r *BotServiceImpl
}

func NewStartCommand(r *BotServiceImpl) CommandFactory {
	return &StartCommand{r: r}
}

func (c *StartCommand) HandleCommand(ctx CommandContext, args string) (bool, string, error) {
	return true, common.CommandStart(), nil
}

type AboutCommand struct {
	r *BotServiceImpl
}

func NewAboutCommand(r *BotServiceImpl) CommandFactory {
	return &AboutCommand{r: r}
}

func (c *AboutCommand) HandleCommand(ctx CommandContext, args string) (bool, string, error) {
	return true, common.CommandAbout(), nil
}

type StopCommand struct {
	r *BotServiceImpl
}

func NewStopCommand(r *BotServiceImpl) CommandFactory {
	return &StopCommand{r: r}
}

// HandleCommand cancels the turn currently answering the same conversation.
// The partial transcript is saved by runConversation, so stopping never loses
// what the model already produced.
func (c *StopCommand) HandleCommand(ctx CommandContext, args string) (bool, string, error) {
	if ctx.Msg == nil || !c.r.turnRegistry().cancel(conversationLockKey(ctx.Msg)) {
		return true, common.CommandStopNothing(), nil
	}
	return true, common.CommandStopped(), nil
}

type NewCommand struct {
	r *BotServiceImpl
}

func NewNewCommand(r *BotServiceImpl) CommandFactory {
	return &NewCommand{r: r}
}

func (c *NewCommand) HandleCommand(ctx CommandContext, args string) (bool, string, error) {
	// Scoped channels (Telegram topics) rotate the session for that scope only.
	// An empty scope keeps the legacy per-user behavior.
	channelName := ""
	if ctx.Msg != nil {
		channelName = ctx.Msg.Channel
	}
	_, err := c.r.conversationRepo.CreateConversationInScope(ctx.User.UserId, ctx.Scope, channelName, "")
	if err != nil {
		return true, common.CommandNewFailed(), nil
	}
	if ctx.Scope != "" {
		return true, common.CommandNewTopic(), nil
	}
	return true, common.CommandNew(), nil
}

type MeCommand struct {
	r *BotServiceImpl
}

func NewMeCommand(r *BotServiceImpl) CommandFactory {
	return &MeCommand{r: r}
}

func (c *MeCommand) HandleCommand(ctx CommandContext, args string) (bool, string, error) {
	return true, utils.CommandMe(ctx.User), nil
}

type NotFoundCommand struct {
	r *BotServiceImpl
}

func NewNotFoundCommand(r *BotServiceImpl) CommandFactory {
	return &NotFoundCommand{r: r}
}

func (c *NotFoundCommand) HandleCommand(ctx CommandContext, args string) (bool, string, error) {
	return true, common.CommandNotFound(), nil
}

type CommandExecutor struct {
	commandMap map[string]CommandFactory
}

func NewCommandExecutor(r *BotServiceImpl) *CommandExecutor {
	return &CommandExecutor{
		commandMap: map[string]CommandFactory{
			"start": NewStartCommand(r),
			"menu":  NewStartCommand(r),
			"help":  NewStartCommand(r),
			"about": NewAboutCommand(r),
			"new":   NewNewCommand(r),
			"stop":  NewStopCommand(r),

			"me": NewMeCommand(r),
		},
	}
}

// Known reports whether name is a command this executor handles.
func (e *CommandExecutor) Known(name string) bool {
	_, ok := e.commandMap[name]
	return ok
}

func (e *CommandExecutor) ExecuteCommand(command string, ctx CommandContext, args string) (bool, string, error) {
	cmd, exists := e.commandMap[command]
	if !exists {
		cmd = NewNotFoundCommand(nil)
	}
	return cmd.HandleCommand(ctx, args)
}

func (r *BotServiceImpl) command(user *model.User, msg *channel.IncomingMessage) (bool, string, error) {
	isCommand, command, args := utils.ParseCommand(msg.Text)
	if !isCommand {
		return false, "", nil
	}

	executor := NewCommandExecutor(r)
	// "/" always means a command, including the "not found" reply. "!" is a
	// convenience prefix from chat platforms, so it only counts when it names a
	// command we know; a normal message that starts with "!" reaches the model.
	if utils.CommandPrefix(msg.Text) == '!' && !executor.Known(command) {
		return false, "", nil
	}

	return executor.ExecuteCommand(command, CommandContext{User: user, Msg: msg, Scope: msg.Scope}, args)
}
