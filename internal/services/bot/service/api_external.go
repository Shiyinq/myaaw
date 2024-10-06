package service

import (
	"errors"
	"fmt"
	"teo/internal/config"
	"teo/internal/pkg"
	"teo/internal/services/bot/model"

	"github.com/go-resty/resty/v2"
)

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

func ollamaTags() (*model.OllamaTagsResponse, error) {
	tags, errRedis := pkg.GetOllamaTagsFromRedis(config.RedisClient)
	if errRedis != nil {
		return nil, errRedis
	}

	if tags != nil {
		return tags, nil
	}

	client := resty.New()

	var response model.OllamaTagsResponse
	_, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetResult(&response).
		Get(config.OllamaBaseUrl + "/api/tags")

	if err != nil {
		return nil, err
	}

	err = pkg.SaveOllamaTagsToRedis(config.RedisClient, &response)
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
