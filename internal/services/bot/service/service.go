package service

import (
	"fmt"
	"log"
	"myaaw/internal/agent"
	"myaaw/internal/config"
	"myaaw/internal/pkg"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
	"myaaw/internal/services/bot/repository"
	"strconv"
	"time"
)

type BotService interface {
	checkUser(chat *pkg.TelegramIncommingChat) (*model.User, error)
	Bot(chat *pkg.TelegramIncommingChat) (*pkg.TelegramSendMessageStatus, error)
	command(user *model.User, chat *pkg.TelegramIncommingChat) (bool, string, error)
	conversation(user *model.User, chat *pkg.TelegramIncommingChat) (*pkg.TelegramSendMessageStatus, error)
	NotifyError(chatId int, replyId int, text string, markdown bool) (*pkg.TelegramSendMessageStatus, error)
	ProcessHeartbeat(prompt, to, channel string) error
}

type BotServiceImpl struct {
	userRepo         repository.UserRepository
	conversationRepo repository.ConversationRepository
	llmProvider      provider.LLMProvider
	ttsProvider      provider.TTSProvider
	agent            agent.AgentProvider
}

func NewBotService(userRepo repository.UserRepository, conversationRepo repository.ConversationRepository) BotService {
	llmProvider, err := provider.CreateLLMProvider(config.LLMProviderName, config.LLMProviderAPIKey)
	if err != nil {
		log.Fatalf("Error create LLM provider - %s: %v", config.LLMProviderName, err)
	}

	ttsProvider, err := provider.CreateTTSProvider(config.TTSProviderName, config.TTSProviderAPIKey)
	if err != nil {
		log.Printf("Warning: Error creating TTS provider %s: %v. TTS functionality might be affected or disabled depending on message handling logic.", config.TTSProviderName, err)
	}

	ag := agent.NewAgent(llmProvider)

	return &BotServiceImpl{
		userRepo:         userRepo,
		conversationRepo: conversationRepo,
		llmProvider:      llmProvider,
		ttsProvider:      ttsProvider,
		agent:            ag,
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

func (r *BotServiceImpl) ProcessHeartbeat(prompt, to, channel string) error {
	log.Printf("Processing heartbeat request from %s (Channel: %s)...", to, channel)

	userId, err := strconv.Atoi(to)
	if err != nil {
		return fmt.Errorf("invalid user ID format: %v", err)
	}

	syntheticChat := &pkg.TelegramIncommingChat{
		UpdateId: 0,
		Message: pkg.UserMessage{
			MessageId: 0,
			Date:      time.Now().Unix(),
			Text:      prompt,
			From: pkg.From{
				Id:           userId,
				IsBot:        false,
				FirstName:    "Heartbeat Trigger",
				Username:     "heartbeat",
				LanguageCode: "en",
			},
			Chat: pkg.Chat{
				Id:        userId,
				Type:      "private",
				FirstName: "Heartbeat Trigger",
				Username:  "heartbeat",
			},
		},
	}

	user, err := r.checkUser(syntheticChat)
	if err != nil {
		return fmt.Errorf("failed to get/create user for heartbeat: %w", err)
	}

	_, err = r.conversation(user, syntheticChat)
	if err != nil {
		log.Printf("Heartbeat conversation error: %v", err)
		return err
	}

	log.Println("Heartbeat conversation processed successfully.")
	return nil
}
