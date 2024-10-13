package service

import (
	"fmt"
	"teo/internal/provider"
	"teo/internal/services/bot/model"
	"teo/internal/utils"
)

func (r *BotServiceImpl) conversation(user *model.User, chat *model.TelegramIncommingChat) (*model.TelegramSendMessageStatus, error) {
	messages := []provider.Message{
		{
			Role:    "system",
			Content: user.System,
		},
	}

	messages = append(messages, user.Messages...)
	newMessage := provider.Message{
		Role:    "user",
		Content: chat.Message.Text,
	}
	messages = append(messages, newMessage)

	var response provider.Message
	messageId, content, err := r.chatStream(user, chat, messages)
	if err != nil {
		return nil, err
	}

	response.Role = "assistant"
	response.Content = content

	editMessage, err := editTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, messageId, utils.Watermark(content, user.Model), true)
	if err != nil || !editMessage.Ok {
		return nil, err
	}

	messages = append(messages, response)
	messages = messages[1:]
	updateError := r.userRepo.UpdateMessages(chat.Message.From.Id, &messages)

	if updateError != nil {
		return nil, err
	}

	return editMessage, nil
}

func (r *BotServiceImpl) chatStream(user *model.User, chat *model.TelegramIncommingChat, messages []provider.Message) (int, string, error) {
	messageId := 0
	streamingContent := ""
	err := r.llmProvider.ChatStream(user.Model, messages, func(partial provider.Message) error {
		chunk := partial.Content
		streamingContent += chunk

		if messageId == 0 {
			send, err := sendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, chunk, false)
			if err != nil || !send.Ok {
				return fmt.Errorf("failed to send message to Telegram: %v", err)
			}
			messageId = send.Result.MessageId
		} else {
			if utils.ContainsPunctuation(chunk) {
				editMessage, err := editTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, messageId, streamingContent, false)
				if err != nil || !editMessage.Ok {
					return fmt.Errorf("failed to edit message on Telegram: %v", err)
				}
			}
		}

		return nil
	})

	if err != nil {
		return 0, "", err
	}

	return messageId, streamingContent, err
}
