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
	ttsProvider provider.TTSProvider // New field
}

func NewBotService(userRepo repository.UserRepository) BotService {
	llmProvider, err := provider.CreateProvider(config.LLMProviderName, config.LLMProviderAPIKey)
	if err != nil {
		log.Fatalf("Error create LLM provider - %s: %v", config.LLMProviderName, err)
	}

	var ttsProvider provider.TTSProvider // Declare ttsProvider
	ttsProvider, err = provider.CreateTTSProvider(config.TTSProviderName, config.TTSProviderAPIKey, "") // Empty for default model
	if err != nil {
		// Log a warning and proceed with ttsProvider as nil.
		// The message handling logic (e.g. in NewMessage factory within message.go)
		// should already be capable of checking for a nil provider or handling errors from CreateTTSProvider.
		log.Printf("Warning: Error creating TTS provider %s: %v. TTS functionality might be affected or disabled depending on message handling logic.", config.TTSProviderName, err)
		// ttsProvider will be nil if an error occurred and wasn't fatal.
		// If CreateTTSProvider returns a non-nil error, ttsProvider's value is undefined by that call alone,
		// so explicitly ensuring it's nil or handling as per CreateTTSProvider's contract is important.
		// Assuming CreateTTSProvider returns (nil, error) on failure for our purposes here.
	}

	return &BotServiceImpl{
		userRepo:    userRepo,
		llmProvider: llmProvider,
		ttsProvider: ttsProvider, // Assign initialized provider
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
