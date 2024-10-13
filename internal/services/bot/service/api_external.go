package service

import (
	"errors"
	"fmt"
	"log"
	"teo/internal/config"
	"teo/internal/services/bot/model"

	"github.com/go-resty/resty/v2"
)

func sendTelegramTypingAction(chatId int) {
	client := resty.New()

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendChatAction", config.BotToken)
	_, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]interface{}{
			"chat_id": chatId,
			"action":  "typing",
		}).
		Post(url)

	if err != nil {
		log.Fatalf("Error sending chat action: %v", err)
	}
}

func sendTelegramMessage(chatId int, replyId int, text string, markdown bool) (*model.TelegramSendMessageStatus, error) {
	body := &model.TelegramSendMessage{
		Text:             text,
		ReplyToMessageID: replyId,
		ChatID:           chatId,
	}

	if markdown {
		body.ParseMode = "markdown"
	}

	return sendTelegramRequest("sendMessage", body, chatId)
}

func editTelegramMessage(chatId int, replyId int, editMessageId int, text string, markdown bool) (*model.TelegramSendMessageStatus, error) {
	body := &model.TelegramEditMessage{
		Text:             text,
		MessageID:        editMessageId,
		ReplyToMessageID: replyId,
		ChatID:           chatId,
	}

	if markdown {
		body.ParseMode = "markdown"
	}

	return sendTelegramRequest("editMessageText", body, chatId)
}

func sendTelegramRequest(method string, message interface{}, chatId int) (*model.TelegramSendMessageStatus, error) {
	if method != "editMessageText" {
		sendTelegramTypingAction(chatId)
	}

	client := resty.New()
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", config.BotToken, method)

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
		log.Println(resp.String())
		errMessage := fmt.Sprintf("failed to %s message", method)
		return &response, errors.New(errMessage)
	}

	return &response, nil
}
