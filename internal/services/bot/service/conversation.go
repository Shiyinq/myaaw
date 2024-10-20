package service

import (
	"log"
	"teo/internal/config"
	"teo/internal/provider"
	"teo/internal/services/bot/model"
	"teo/internal/utils"
)

func (r *BotServiceImpl) conversation(user *model.User, chat *model.TelegramIncommingChat) (*model.TelegramSendMessageStatus, error) {
	messages := r.buildConversationMessages(user, chat)

	result, response, err := r.factoryChat(user, chat, messages)
	if err != nil {
		return nil, err
	}

	if err := r.updateUserMessages(chat, messages, response); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *BotServiceImpl) buildConversationMessages(user *model.User, chat *model.TelegramIncommingChat) []provider.Message {
	messages := []provider.Message{
		{
			Role:    "system",
			Content: user.System,
		},
	}

	messages = append(messages, user.Messages...)
	newMessage := NewMessage(r.llmProvider.ProviderName(), chat)
	messages = append(messages, newMessage)

	return messages
}

func (r *BotServiceImpl) updateUserMessages(chat *model.TelegramIncommingChat, messages []provider.Message, response provider.Message) error {
	messages = append(messages, response)
	messages = messages[1:]
	if err := r.userRepo.UpdateMessages(chat.Message.From.Id, &messages); err != nil {
		return err
	}

	return nil
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

	send, err := sendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, utils.Watermark(res.Content.(string), user.Model), true)
	if err != nil || !send.Ok {
		return nil, "", nil
	}

	return send, res.Content.(string), nil
}

func (r *BotServiceImpl) chatStream(user *model.User, chat *model.TelegramIncommingChat, messages []provider.Message) (*model.TelegramSendMessageStatus, string, error) {
	messageId := 0
	streamingContent := ""
	bufferThreshold := 500
	bufferedContent := ""

	send, err := sendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, "✨Typing...", false)
	if err != nil || !send.Ok {
		log.Println(err)
	}
	messageId = send.Result.MessageId

	err = r.llmProvider.ChatStream(user.Model, messages, func(partial provider.Message) error {
		chunk := partial.Content
		streamingContent += chunk.(string)
		bufferedContent += chunk.(string)
		if len(bufferedContent) >= bufferThreshold {
			editMessage, err := editTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, messageId, streamingContent+"\n✨Typing...", false)
			if err != nil || !editMessage.Ok {
				log.Println(err)
			}
			bufferedContent = ""
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
	err = nil
	return editMessage, streamingContent, err
}
