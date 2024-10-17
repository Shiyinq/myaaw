package service

import (
	"log"
	"teo/internal/config"
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
	newMessage := MessageHandler(chat)
	messages = append(messages, newMessage)

	result, response, err := r.factoryChat(user, chat, messages)
	if err != nil {
		return nil, err
	}

	messages = append(messages, response)
	messages = messages[1:]
	updateError := r.userRepo.UpdateMessages(chat.Message.From.Id, &messages)

	if updateError != nil {
		return nil, err
	}

	return result, nil
}

func (r *BotServiceImpl) factoryChat(user *model.User, chat *model.TelegramIncommingChat, messages []provider.Message) (*model.TelegramSendMessageStatus, provider.Message, error) {
	var err error
	var content string
	var response provider.Message
	var result *model.TelegramSendMessageStatus

	if config.StreamResponse {
		result, content, err = r.chatStream(user, chat, messages)
	} else {
		result, content, err = r.chat(user, chat, messages)
	}

	response.Role = "assistant"
	response.Content = content

	return result, response, err
}

func (r *BotServiceImpl) chat(user *model.User, chat *model.TelegramIncommingChat, messages []provider.Message) (*model.TelegramSendMessageStatus, string, error) {
	res, err := r.llmProvider.Chat(user.Model, messages)

	if err != nil {
		return nil, "", err
	}

	send, err := sendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, utils.Watermark(res.Content, user.Model), true)
	if err != nil || !send.Ok {
		return nil, "", nil
	}

	return send, res.Content, nil
}

func (r *BotServiceImpl) chatStream(user *model.User, chat *model.TelegramIncommingChat, messages []provider.Message) (*model.TelegramSendMessageStatus, string, error) {
	messageId := 0
	streamingContent := ""
	bufferThreshold := 500
	bufferedContent := ""
	err := r.llmProvider.ChatStream(user.Model, messages, func(partial provider.Message) error {
		chunk := partial.Content
		streamingContent += chunk
		bufferedContent += chunk

		if messageId == 0 {
			send, err := sendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, "✨Typing...", false)
			if err != nil || !send.Ok {
				log.Println(err)
			}
			messageId = send.Result.MessageId
		} else {
			if len(bufferedContent) >= bufferThreshold {
				editMessage, err := editTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, messageId, streamingContent+"\n✨Typing...", false)
				if err != nil || !editMessage.Ok {
					log.Println(err)
				}
				bufferedContent = ""
			}
		}

		return nil
	})

	if err != nil {
		return nil, "", err
	}

	editMessage, err := editTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, messageId, utils.Watermark(streamingContent, user.Model), true)
	if err != nil || !editMessage.Ok {
		_, err := editTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, messageId, utils.Watermark(streamingContent, user.Model), false)
		if err != nil {
			log.Println(err)
			return nil, "", err
		}
	}

	return editMessage, streamingContent, err
}
