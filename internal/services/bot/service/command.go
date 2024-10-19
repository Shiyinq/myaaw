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

func (r *BotServiceImpl) handleSystemCommand(user *model.User, args string) (bool, string, error) {
	if args == "" {
		return true, common.CommandSystemNeedArgs(), nil
	}
	err := r.userRepo.UpdateSystem(user.UserId, args)
	if err != nil {
		return true, common.CommandSystemFailed(), nil
	}
	return true, common.CommandSystem(), nil
}

func (r *BotServiceImpl) handleResetCommand(user *model.User) (bool, string, error) {
	err := r.userRepo.UpdateMessages(user.UserId, &[]provider.Message{})
	if err != nil {
		return true, common.CommandResetFailed(), nil
	}
	return true, common.CommandReset(), nil
}

func (r *BotServiceImpl) handleModelsCommand(user *model.User, args string) (bool, string, error) {
	var models []string
	provider := r.llmProvider.ProviderName()
	modelCache, err := pkg.GetModelNamesFromRedis(config.RedisClient, provider)
	if err != nil {
		return true, common.CommandModelsFailed(), nil
	}

	if modelCache != nil {
		models = modelCache
	} else {
		models, err = r.llmProvider.Models()
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

	err = r.userRepo.UpdateModel(user.UserId, models[idModel])
	if err != nil {
		return true, common.CommandModelsUpdateFailed(), nil
	}

	return true, common.CommandModels(), nil
}

func (r *BotServiceImpl) handleAgentCommand(user *model.User, args string) (bool, string, error) {
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
		r.handleResetCommand(user)
		return r.handleSystemCommand(user, prompt)
	}

	return true, "", nil
}

func (r *BotServiceImpl) command(user *model.User, chat *model.TelegramIncommingChat) (bool, string, error) {
	isCommand, command, commandArgs := utils.ParseCommand(chat.Message.Text)
	if !isCommand {
		return false, "", nil
	}

	switch command {
	case "start":
		return true, common.CommandStart(), nil
	case "about":
		return true, common.CommandAbout(), nil
	case "system":
		return r.handleSystemCommand(user, commandArgs)
	case "reset":
		return r.handleResetCommand(user)
	case "models":
		return r.handleModelsCommand(user, commandArgs)
	case "me":
		return true, utils.CommandMe(user), nil
	case "agents":
		return r.handleAgentCommand(user, commandArgs)
	default:
		return true, common.CommandNotFound(command), nil
	}
}
