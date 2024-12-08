package service

import (
	"log"
	"teo/internal/config"
	"teo/internal/pkg"
	"teo/internal/provider"
	"teo/internal/services/bot/model"
	"teo/internal/services/bot/repository"
)

type BotService interface {
	checkUser(chat *pkg.TelegramIncommingChat) (*model.User, error)
	Bot(chat *pkg.TelegramIncommingChat) (*pkg.TelegramSendMessageStatus, error)
	command(user *model.User, chat *pkg.TelegramIncommingChat) (bool, string, error)
	conversation(user *model.User, chat *pkg.TelegramIncommingChat) (*pkg.TelegramSendMessageStatus, error)
	NotifyError(chatId int, replyId int, text string, markdown bool) (*pkg.TelegramSendMessageStatus, error)
}

type BotServiceImpl struct {
	userRepo    repository.UserRepository
	llmProvider provider.LLMProvider
}

func NewBotService(userRepo repository.UserRepository) BotService {
	llmProvider, err := provider.CreateProvider(config.LLMProviderName, config.LLMProviderAPIKey)
	if err != nil {
		log.Fatalf("Error create provider - %s: %v", config.LLMProviderName, err)
	}

	return &BotServiceImpl{
		userRepo:    userRepo,
		llmProvider: llmProvider,
	}
}

func (r *BotServiceImpl) checkUser(chat *pkg.TelegramIncommingChat) (*model.User, error) {
	var user *model.User
	var err error
	user, err = r.userRepo.GetUserById(chat.Message.From.Id)
	if err != nil {
		return nil, err
	}

	if user == nil {
		newUser := model.User{
			UserId:   chat.Message.From.Id,
			Name:     chat.Message.Chat.FirstName,
			Provider: r.llmProvider.ProviderName(),
			Model:    r.llmProvider.DefaultModel(""),
		}
		user, err = r.userRepo.CreateUser(&newUser)

		if err != nil {
			return nil, err
		}
	}

	return user, nil
}

func (r *BotServiceImpl) changeProviderAndModel(user *model.User) (*model.User, error) {
	systemProvider := r.llmProvider.ProviderName()
	systemModel := r.llmProvider.DefaultModel("")

	log.Printf("Provider mismatch!")
	log.Printf("Automatically updating user %v configurations", user.UserId)

	err := r.userRepo.UpdateModel(user.UserId, systemModel)
	if err != nil {
		return nil, err
	}

	err = r.userRepo.UpdateProvider(user.UserId, systemProvider)
	if err != nil {
		return nil, err
	}

	user, err = r.userRepo.GetUserById(user.UserId)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (r *BotServiceImpl) Bot(chat *pkg.TelegramIncommingChat) (*pkg.TelegramSendMessageStatus, error) {
	var command bool
	var response string

	user, err := r.checkUser(chat)
	if err != nil {
		return nil, err
	}

	if user.Provider != r.llmProvider.ProviderName() {
		user, err = r.changeProviderAndModel(user)
		if err != nil {
			return nil, err
		}
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

		return conv, nil
	}

	if command {
		send, err := pkg.SendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, response, true)
		if err != nil || !send.Ok {
			return nil, err
		}
	}

	return nil, nil
}

func (r *BotServiceImpl) NotifyError(chatId int, replyId int, text string, markdown bool) (*pkg.TelegramSendMessageStatus, error) {
	return pkg.SendTelegramMessage(chatId, replyId, text, markdown)
}
