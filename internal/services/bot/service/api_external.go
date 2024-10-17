package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
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
		Text:   text,
		ChatID: chatId,
	}

	if replyId != 0 {
		body.ReplyToMessageID = replyId
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
		SetError(&response).
		Post(url)

	if err != nil {
		return &response, err
	}

	if resp.StatusCode() != 200 {
		errMessage := fmt.Sprintf("failed to %s message, %s %v", method, resp.Status(), response.Description)
		return &response, errors.New(errMessage)
	}

	return &response, nil
}

type TelegramFileResponse struct {
	Ok     bool `json:"ok"`
	Result struct {
		FileID       string `json:"file_id"`
		FileUniqueID string `json:"file_unique_id"`
		FileSize     int    `json:"file_size"`
		FilePath     string `json:"file_path"`
	} `json:"result"`
}

func getFilePath(fileID string) (string, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", config.BotToken, fileID)
	client := resty.New()
	resp, err := client.R().Get(url)
	if err != nil {
		return "", fmt.Errorf("error while making GET request: %v", err)
	}

	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("failed to get file path, status code: %d", resp.StatusCode())
	}

	var fileResponse TelegramFileResponse
	if err := json.Unmarshal(resp.Body(), &fileResponse); err != nil {
		return "", fmt.Errorf("error unmarshalling JSON response: %v", err)
	}

	if !fileResponse.Ok {
		return "", fmt.Errorf("failed to get file path, API response not OK")
	}

	return fileResponse.Result.FilePath, nil
}

func telegramImageURL(filePath string) string {
	return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", config.BotToken, filePath)
}

func imageURLToBase64(filePath string) (string, error) {
	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", config.BotToken, filePath)
	client := resty.New()
	resp, err := client.R().Get(url)
	if err != nil {
		return "", fmt.Errorf("error while making GET request: %v", err)
	}

	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("failed to fetch image, status code: %d", resp.StatusCode())
	}

	base64Image := base64.StdEncoding.EncodeToString(resp.Body())
	base64Cleaned := strings.TrimPrefix(base64Image, "data:image/png;base64,")

	return base64Cleaned, nil
}
