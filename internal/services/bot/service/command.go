package service

import (
	"strconv"
	"teo/internal/common"
	"teo/internal/config"
	"teo/internal/pkg"
	"teo/internal/provider"
	"teo/internal/services/bot/model"
	"teo/internal/utils"
)

type CommandFactory interface {
	HandleCommand(user *model.User, args string) (bool, string, error)
}

type StartCommand struct {
	r *BotServiceImpl
}

func (c *StartCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	return true, common.CommandStart(), nil
}

type AboutCommand struct {
	r *BotServiceImpl
}

func (c *AboutCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	return true, common.CommandAbout(), nil
}

type SystemCommand struct {
	r *BotServiceImpl
}

func (c *SystemCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	if args == "" {
		return true, common.CommandSystemNeedArgs(), nil
	}
	err := c.r.userRepo.UpdateSystem(user.UserId, args)
	if err != nil {
		return true, common.CommandSystemFailed(), nil
	}
	return true, common.CommandSystem(), nil
}

type ResetCommand struct {
	r *BotServiceImpl
}

func (c *ResetCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	err := c.r.userRepo.UpdateMessages(user.UserId, &[]provider.Message{})
	if err != nil {
		return true, common.CommandResetFailed(), nil
	}
	return true, common.CommandReset(), nil
}

type ModelsCommand struct {
	r *BotServiceImpl
}

func (c *ModelsCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	var models []string
	provider := c.r.llmProvider.ProviderName()
	modelCache, err := pkg.GetModelNamesFromRedis(config.RedisClient, provider)
	if err != nil {
		return true, common.CommandModelsFailed(), nil
	}

	if modelCache != nil {
		models = modelCache
	} else {
		models, err = c.r.llmProvider.Models()
		if err != nil {
			return true, common.CommandModelsFailed(), nil
		}
		pkg.SaveModelNamesToRedis(config.RedisClient, provider, models)
	}

	if args == "" {
		return true, utils.ListModels(*user, models), nil
	}

	idModel, err := strconv.Atoi(args)
	if err != nil || idModel < 0 || idModel >= len(models) {
		return true, common.CommandModelsArgsNotInt(), nil
	}

	err = c.r.userRepo.UpdateModel(user.UserId, models[idModel])
	if err != nil {
		return true, common.CommandModelsUpdateFailed(), nil
	}

	return true, common.CommandModels(), nil
}

type AgentCommand struct {
	r *BotServiceImpl
}

func (c *AgentCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	list, detailAgents := utils.Agents()

	if args == "" {
		return true, list, nil
	}

	idAgent, err := strconv.Atoi(args)
	if err != nil {
		return true, common.CommandAgentArgsNotInt(), nil
	}

	if idAgent < 0 || idAgent >= len(detailAgents) {
		return true, common.CommandAgentNotFound(), nil
	}

	if prompt, ok := detailAgents[idAgent]["prompt"].(string); ok {
		var cf CommandFactory

		cf = &ResetCommand{r: c.r}
		cf.HandleCommand(user, args)

		cf = &SystemCommand{r: c.r}
		return cf.HandleCommand(user, prompt)
	}

	return true, "", nil
}

type MeCommand struct {
	r *BotServiceImpl
}

func (c *MeCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	return true, utils.CommandMe(user), nil
}

type NotFoundCommand struct {
	r *BotServiceImpl
}

func (c *NotFoundCommand) HandleCommand(user *model.User, args string) (bool, string, error) {
	return true, common.CommandNotFound(), nil
}

func (r *BotServiceImpl) command(user *model.User, chat *model.TelegramIncommingChat) (bool, string, error) {
	isCommand, command, commandArgs := utils.ParseCommand(chat.Message.Text)
	if !isCommand {
		return false, "", nil
	}

	commandMap := map[string]CommandFactory{
		"start":  &StartCommand{r: r},
		"about":  &AboutCommand{r: r},
		"system": &SystemCommand{r: r},
		"reset":  &ResetCommand{r: r},
		"models": &ModelsCommand{r: r},
		"agents": &AgentCommand{r: r},
		"me":     &MeCommand{r: r},
	}

	commandMessage, exists := commandMap[command]
	if !exists {
		commandMessage = &NotFoundCommand{r: r}
	}
	return commandMessage.HandleCommand(user, commandArgs)
}
