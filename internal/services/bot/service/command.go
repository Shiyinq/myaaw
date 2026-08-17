package service

import (
	"myaaw/internal/channel"
	"myaaw/internal/common"
	"myaaw/internal/services/bot/model"
	"myaaw/internal/utils"
)

type CommandFactory interface {
	HandleCommand(user *model.User, args string) (bool, string, error)
}

type StartCommand struct {
	r *BotServiceImpl
}

func NewStartCommand(r *BotServiceImpl) CommandFactory {
	return &StartCommand{r: r}
}

func (c *StartCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	return true, common.CommandStart(), nil
}

type AboutCommand struct {
	r *BotServiceImpl
}

func NewAboutCommand(r *BotServiceImpl) CommandFactory {
	return &AboutCommand{r: r}
}

func (c *AboutCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	return true, common.CommandAbout(), nil
}

type NewCommand struct {
	r *BotServiceImpl
}

func NewNewCommand(r *BotServiceImpl) CommandFactory {
	return &NewCommand{r: r}
}

func (c *NewCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	_, err := c.r.conversationRepo.CreateConversation(user.UserId, "")
	if err != nil {
		return true, common.CommandNewFailed(), nil
	}
	return true, common.CommandNew(), nil
}

type MeCommand struct {
	r *BotServiceImpl
}

func NewMeCommand(r *BotServiceImpl) CommandFactory {
	return &MeCommand{r: r}
}

func (c *MeCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	return true, utils.CommandMe(user), nil
}

type NotFoundCommand struct {
	r *BotServiceImpl
}

func NewNotFoundCommand(r *BotServiceImpl) CommandFactory {
	return &NotFoundCommand{r: r}
}

func (c *NotFoundCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	return true, common.CommandNotFound(), nil
}

type CommandExecutor struct {
	commandMap map[string]CommandFactory
}

func NewCommandExecutor(r *BotServiceImpl) *CommandExecutor {
	return &CommandExecutor{
		commandMap: map[string]CommandFactory{
			"start":  NewStartCommand(r),
			"menu":   NewStartCommand(r),
			"help":   NewStartCommand(r),
			"about":  NewAboutCommand(r),
			"new":    NewNewCommand(r),

			"me": NewMeCommand(r),
		},
	}
}

func (e *CommandExecutor) ExecuteCommand(command string, user *model.User, args string) (bool, string, error) {
	cmd, exists := e.commandMap[command]
	if !exists {
		cmd = NewNotFoundCommand(nil)
	}
	return cmd.HandleCommand(user, args)
}

func (r *BotServiceImpl) command(user *model.User, msg *channel.IncomingMessage) (bool, string, error) {
	isCommand, command, args := utils.ParseCommand(msg.Text)
	if !isCommand {
		return false, "", nil
	}

	executor := NewCommandExecutor(r)
	return executor.ExecuteCommand(command, user, args)
}
