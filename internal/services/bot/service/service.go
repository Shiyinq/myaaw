package service

import (
	"errors"
	"fmt"
	"teo/internal/config"
	"teo/internal/services/bot/model"
	"teo/internal/services/bot/repository"

	"github.com/go-resty/resty/v2"
)

type BotService interface {
	checkUser(chat *model.TelegramIncommingChat) (*model.User, error)
	Bot(chat *model.TelegramIncommingChat) (*model.OllamaResponse, error)
	conversation(user *model.User, chat *model.TelegramIncommingChat) (*model.OllamaResponse, error)
}

type BotServiceImpl struct {
	userRepo repository.UserRepository
}

func NewBotService(userRepo repository.UserRepository) BotService {
	return &BotServiceImpl{userRepo: userRepo}
}

func ollama(messages []model.Message) (*model.OllamaResponse, error) {
	client := resty.New()

	request := model.OllamaRequest{
		Model:    config.OllamaDefaultModel,
		Stream:   false,
		Messages: messages,
	}

	var response model.OllamaResponse
	_, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		SetResult(&response).
		Post(config.OllamaBaseUrl + "/api/chat")

	if err != nil {
		return nil, err
	}

	return &response, nil
}

func sendTelegramMessage(text string, chatId int) (*model.TelegramSendMessageStatus, error) {
	client := resty.New()

	message := model.TelegramTextMessage{
		Text:             text,
		ParseMode:        "markdown",
		ReplyToMessageID: nil,
		ChatID:           chatId,
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.BotToken)

	var response model.TelegramSendMessageStatus
	resp, err := client.R().
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetBody(message).
		SetResult(&response).
		Post(url)

	if err != nil {
		return &response, err
	}

	if resp.StatusCode() != 200 {
		return &response, errors.New("failed send to telegram")
	}

	return &response, nil
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

func (r *BotServiceImpl) conversation(user *model.User, chat *model.TelegramIncommingChat) (*model.OllamaResponse, error) {
	messages := []model.Message{
		{
			Role:    "system",
			Content: "You are Teo, a helpful assistant living in Telegram. Respond to users using Telegram's supported MarkdownV2 style.",
		},
	}

	messages = append(messages, user.Messages...)
	newMessage := model.Message{
		Role:    "user",
		Content: chat.Message.Text,
	}
	messages = append(messages, newMessage)

	res, err := ollama(messages)

	if err != nil {
		return nil, err
	}

	messages = append(messages, res.Message)

	updateError := r.userRepo.UpdateMessages(chat.Message.From.Id, &messages)

	if updateError != nil {
		return nil, err
	}

	return res, nil
}

func (r *BotServiceImpl) Bot(chat *model.TelegramIncommingChat) (*model.OllamaResponse, error) {
	user, err := r.checkUser(chat)
	if err != nil {
		return nil, err
	}

	conv, err := r.conversation(user, chat)
	if err != nil {
		return nil, err
	}

	send, err := sendTelegramMessage(conv.Message.Content, chat.Message.From.Id)
	if err != nil || !send.Ok {
		return nil, err
	}

	return conv, nil
}
