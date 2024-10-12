package service

import (
	"teo/internal/provider"
	"teo/internal/services/bot/model"
	"teo/internal/services/bot/repository"
)

type BotService interface {
	checkUser(chat *model.TelegramIncommingChat) (*model.User, error)
	Bot(chat *model.TelegramIncommingChat) (*model.TelegramSendMessageStatus, error)
	command(user *model.User, chat *model.TelegramIncommingChat) (bool, string, error)
	conversation(user *model.User, chat *model.TelegramIncommingChat) (*provider.Message, error)
}

type BotServiceImpl struct {
	userRepo    repository.UserRepository
	llmProvider provider.LLMProvider
}

func NewBotService(userRepo repository.UserRepository) BotService {
	llmProvider, _ := provider.CreateProvider("ollama", "ollama")
	return &BotServiceImpl{
		userRepo:    userRepo,
		llmProvider: llmProvider,
	}
}

func (r *BotServiceImpl) checkUser(chat *model.TelegramIncommingChat) (*model.User, error) {
	var user *model.User
	var err error
	user, err = r.userRepo.GetUserById(chat.Message.From.Id)
	if err != nil {
		return nil, err
	}

	if user == nil {
		newUser := model.User{
			UserId: chat.Message.From.Id,
			Name:   chat.Message.Chat.FirstName,
		}
		user, err = r.userRepo.CreateUser(&newUser)

		if err != nil {
			return nil, err
		}
	}

	return user, nil
}

func (r *BotServiceImpl) Bot(chat *model.TelegramIncommingChat) (*model.TelegramSendMessageStatus, error) {
	var command bool
	var response string

	user, err := r.checkUser(chat)
	if err != nil {
		return nil, err
	}

	command, response, err = r.command(user, chat)
	if err != nil {
		return nil, err
	}

	if !command {
		conv, err := r.conversation(user, chat)
		if err != nil {
			return nil, err
		}
		response = conv.Content
	}

	send, err := sendTelegramMessage(chat.Message.From.Id, chat.Message.MessageId, response)
	if err != nil || !send.Ok {
		return nil, err
	}

	return send, nil
}
