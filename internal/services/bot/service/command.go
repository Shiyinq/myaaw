package service

import (
	"strconv"
	"teo/internal/common"
	"teo/internal/provider"
	"teo/internal/services/bot/model"
	"teo/internal/utils"
)

func (r *BotServiceImpl) handleSystemCommand(chat *model.TelegramIncommingChat, args string) (bool, string, error) {
	if args == "" {
		return true, common.CommandSystemNeedArgs(), nil
	}
	err := r.userRepo.UpdateSystem(chat.Message.From.Id, args)
	if err != nil {
		return true, common.CommandSystemFailed(), nil
	}
	return true, common.CommandSystem(), nil
}

func (r *BotServiceImpl) handleResetCommand(chat *model.TelegramIncommingChat) (bool, string, error) {
	err := r.userRepo.UpdateMessages(chat.Message.From.Id, &[]provider.Message{})
	if err != nil {
		return true, common.CommandResetFailed(), nil
	}
	return true, common.CommandReset(), nil
}

func (r *BotServiceImpl) handleModelsCommand(user *model.User, chat *model.TelegramIncommingChat, args string) (bool, string, error) {
	models, err := r.llmProvider.Models()
	if err != nil {
		return true, common.CommandModelsFailed(), nil
	}

	if args == "" {
		return true, utils.ListModels(*user, models), nil
	}

	idModel, err := strconv.Atoi(args)
	if err != nil || idModel < 0 || idModel >= len(models) {
		return true, common.CommandModelsArgsNotInt(), nil
	}

	err = r.userRepo.UpdateModel(chat.Message.From.Id, models[idModel])
	if err != nil {
		return true, common.CommandModelsUpdateFailed(), nil
	}

	return true, common.CommandModels(), nil
}

func (r *BotServiceImpl) handleAgentCommand(chat *model.TelegramIncommingChat, args string) (bool, string, error) {
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
		r.handleResetCommand(chat)
		return r.handleSystemCommand(chat, prompt)
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
		return r.handleSystemCommand(chat, commandArgs)
	case "reset":
		return r.handleResetCommand(chat)
	case "models":
		return r.handleModelsCommand(user, chat, commandArgs)
	case "me":
		return true, utils.CommandMe(user), nil
	case "agents":
		return r.handleAgentCommand(chat, commandArgs)
	default:
		return true, common.CommandNotFound(command), nil
	}
}
