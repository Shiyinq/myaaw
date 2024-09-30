package service

import (
	"errors"
	"fmt"
	"teo/internal/common"
	"teo/internal/config"
	"teo/internal/services/bot/model"
	"teo/internal/services/bot/repository"
	"teo/internal/utils"

	"github.com/go-resty/resty/v2"
)

type BotService interface {
	checkUser(chat *model.TelegramIncommingChat) (*model.User, error)
	Bot(chat *model.TelegramIncommingChat) (*model.TelegramSendMessageStatus, error)
	command(user *model.User, chat *model.TelegramIncommingChat) (bool, string, error)
	conversation(user *model.User, chat *model.TelegramIncommingChat) (*model.OllamaResponse, error)
}

type BotServiceImpl struct {
	userRepo repository.UserRepository
}

func NewBotService(userRepo repository.UserRepository) BotService {
	return &BotServiceImpl{userRepo: userRepo}
}

func ollama(modelName string, messages []model.Message) (*model.OllamaResponse, error) {
	client := resty.New()

	request := model.OllamaRequest{
		Model:    modelName,
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

func sendTelegramMessage(chatId int, text string) (*model.TelegramSendMessageStatus, error) {
	client := resty.New()

	message := model.TelegramSendMessage{
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
		if commandArgs == "" {
			return true, common.CommandSystemNeedArgs(), nil
		}
		err := r.userRepo.UpdateSystem(chat.Message.From.Id, commandArgs)
		if err != nil {
			return true, common.CommandSystemFailed(), nil
		}
		return true, common.CommandSystem(), nil
	case "reset":
		err := r.userRepo.UpdateMessages(chat.Message.From.Id, &[]model.Message{})
		if err != nil {
			return true, common.CommandResetFailed(), nil
		}
		return true, common.CommandReset(), nil
	default:
		return true, common.CommandNotFound(command), nil
	}
}

func (r *BotServiceImpl) conversation(user *model.User, chat *model.TelegramIncommingChat) (*model.OllamaResponse, error) {
	messages := []model.Message{
		{
			Role:    "system",
			Content: user.System,
		},
	}

	messages = append(messages, user.Messages...)
	newMessage := model.Message{
		Role:    "user",
		Content: chat.Message.Text,
	}
	messages = append(messages, newMessage)

	res, err := ollama(user.Model, messages)

	if err != nil {
		return nil, err
	}

	messages = append(messages, res.Message)
	messages = messages[1:]
	updateError := r.userRepo.UpdateMessages(chat.Message.From.Id, &messages)

	if updateError != nil {
		return nil, err
	}

	return res, nil
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
		response = conv.Message.Content
	}

	send, err := sendTelegramMessage(chat.Message.From.Id, response)
	if err != nil || !send.Ok {
		return nil, err
	}

	return send, nil
}
