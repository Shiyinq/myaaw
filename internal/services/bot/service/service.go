package service

import (
	"fmt"
	"log"
	"myaaw/internal/agent"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
	"myaaw/internal/services/bot/repository"
	"slices"
	"strconv"
)

type BotService interface {
	IsAllowed(userID int) bool
	checkUser(msg *channel.IncomingMessage) (*model.User, error)
	Bot(msg *channel.IncomingMessage) (*channel.OutgoingMessage, error)
	BotStream(msg *channel.IncomingMessage, onChunk func(channel.StreamChunk)) (*channel.OutgoingMessage, error)
	command(user *model.User, msg *channel.IncomingMessage) (bool, string, error)
	conversation(user *model.User, msg *channel.IncomingMessage) (*channel.OutgoingMessage, error)
	ProcessHeartbeat(prompt, to, channelName string) (*channel.IncomingMessage, *channel.OutgoingMessage, error)
}

type BotServiceImpl struct {
	userRepo         repository.UserRepository
	conversationRepo repository.ConversationRepository
	llmProvider      provider.LLMProvider
	transcriber      provider.Transcriber
	agent            agent.AgentProvider
}

func NewBotService(userRepo repository.UserRepository, conversationRepo repository.ConversationRepository) BotService {
	llmProvider, err := provider.CreateLLMProvider(config.LLMProviderName, config.LLMProviderAPIKey)
	if err != nil {
		log.Fatalf("Error create LLM provider - %s: %v", config.LLMProviderName, err)
	}

	transcriber, err := provider.CreateTranscriber(config.TranscriberProviderName, config.TranscriberAPIKey)
	if err != nil {
		log.Printf("Warning: Error creating Transcriber provider %s: %v. Transcriber functionality might be affected or disabled depending on message handling logic.", config.TranscriberProviderName, err)
	}

	ag := agent.NewAgent(llmProvider)

	return &BotServiceImpl{
		userRepo:         userRepo,
		conversationRepo: conversationRepo,
		llmProvider:      llmProvider,
		transcriber:      transcriber,
		agent:            ag,
	}
}

func (r *BotServiceImpl) IsAllowed(userID int) bool {
	if config.BotType != "private" {
		return true
	}
	return slices.Contains(config.OwnerIDs, strconv.Itoa(userID))
}

func (r *BotServiceImpl) checkUser(msg *channel.IncomingMessage) (*model.User, error) {
	var user *model.User
	var err error
	user, err = r.userRepo.GetUserById(msg.UserID)
	if err != nil {
		return nil, err
	}

	if user == nil {
		newUser := model.User{
			UserId:   msg.UserID,
			Name:     fmt.Sprintf("user-%d", msg.UserID),
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

func (r *BotServiceImpl) Bot(msg *channel.IncomingMessage) (*channel.OutgoingMessage, error) {

	user, err := r.checkUser(msg)
	if err != nil {
		return nil, err
	}

	if user.Provider != r.llmProvider.ProviderName() {
		user, err = r.changeProviderAndModel(user)
		if err != nil {
			return nil, err
		}
	}

	isCommand, response, err := r.command(user, msg)
	if err != nil {
		return nil, err
	}

	if isCommand {
		return &channel.OutgoingMessage{Text: response}, nil
	}

	return r.conversation(user, msg)
}

func (r *BotServiceImpl) BotStream(msg *channel.IncomingMessage, onChunk func(channel.StreamChunk)) (*channel.OutgoingMessage, error) {

	user, err := r.checkUser(msg)
	if err != nil {
		return nil, err
	}

	if user.Provider != r.llmProvider.ProviderName() {
		user, err = r.changeProviderAndModel(user)
		if err != nil {
			return nil, err
		}
	}

	isCommand, response, err := r.command(user, msg)
	if err != nil {
		return nil, err
	}

	if isCommand {
		out := &channel.OutgoingMessage{Text: response}
		onChunk(channel.StreamChunk{Text: response})
		return out, nil
	}

	return r.conversationStream(user, msg, onChunk)
}

func (r *BotServiceImpl) ProcessHeartbeat(prompt, to, channelName string) (*channel.IncomingMessage, *channel.OutgoingMessage, error) {
	log.Printf("Processing heartbeat request from %s (Channel: %s)...", to, channelName)

	userId, err := strconv.Atoi(to)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid user ID format: %v", err)
	}

	msg := &channel.IncomingMessage{
		UserID:  userId,
		Text:    prompt,
		Channel: channelName,
	}

	user, err := r.checkUser(msg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get/create user for heartbeat: %w", err)
	}

	out, err := r.conversation(user, msg)
	if err != nil {
		log.Printf("Heartbeat conversation error: %v", err)
		return msg, nil, err
	}

	log.Println("Heartbeat conversation processed successfully.")
	return msg, out, nil
}
