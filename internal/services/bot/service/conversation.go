package service

import (
	"fmt"
	"log"
	"teo/internal/config"
	"teo/internal/pkg"
	"teo/internal/provider"
	"teo/internal/services/bot/model"
	"teo/internal/utils"
)

func (r *BotServiceImpl) conversation(user *model.User, chat *pkg.TelegramIncommingChat) (*pkg.TelegramSendMessageStatus, error) {
	messages := r.buildConversationMessages(user, chat)
	context := r.contextWindow(messages)

	result, response, err := r.factoryChat(user, chat, context)
	if err != nil {
		return nil, err
	}

	if err := r.updateUserMessages(chat, messages, response); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *BotServiceImpl) contextWindow(history []provider.Message) []provider.Message {
	total := 10

	if total >= len(history) {
		total = len(history) - 1
	}

	context := make([]provider.Message, total+1)
	context[0] = history[0]

	for i := 1; i <= total; i++ {
		context[i] = history[len(history)-total+i-1]
	}

	return context
}

func (r *BotServiceImpl) buildConversationMessages(user *model.User, chat *pkg.TelegramIncommingChat) []provider.Message {
	userSystem := fmt.Sprintf("%s\n\nUser info:\nUser ID: %v\nToday's date is: %s", user.System, user.UserId, utils.GetCurrentTime())
	messages := []provider.Message{
		{
			Role:    "system",
			Content: userSystem,
		},
	}

	messages = append(messages, user.Messages...)
	// Updated call to NewMessage with the new signature
	newMessage := NewMessage(chat, r.llmProvider.ProviderName(), r.ttsProvider)
	messages = append(messages, newMessage)

	return messages
}

func (r *BotServiceImpl) updateUserMessages(chat *pkg.TelegramIncommingChat, messages []provider.Message, response provider.Message) error {
	messages = append(messages, response)
	messages = messages[1:]
	if err := r.userRepo.UpdateMessages(chat.Message.From.Id, &messages); err != nil {
		return err
	}

	return nil
}

func (r *BotServiceImpl) factoryChat(user *model.User, chat *pkg.TelegramIncommingChat, messages []provider.Message) (*pkg.TelegramSendMessageStatus, provider.Message, error) {
	var err error
	var content string
	var response provider.Message
	var result *pkg.TelegramSendMessageStatus

	log.Println("Processing incoming message")
	if config.StreamResponse {
		log.Println("Starting content streaming")
		result, content, err = r.chatStream(user, chat, messages)
	} else {
		result, content, err = r.chat(user, chat, messages)
	}

	response.Role = "assistant"
	response.Content = content

	return result, response, err
}

func (r *BotServiceImpl) chat(user *model.User, chat *pkg.TelegramIncommingChat, messages []provider.Message) (*pkg.TelegramSendMessageStatus, string, error) {
	res, err := r.llmProvider.Chat(user.Model, messages)

	if err != nil {
		return nil, "", err
	}

	send, err := pkg.SendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, utils.Watermark(res.Content.(string), user.Model), true)
	if err != nil || !send.Ok {
		return nil, "", nil
	}

	return send, res.Content.(string), nil
}

func indicator(text string) string {
	if text == "tool" {
		return "⚙️ Using tool..."
	}
	return "✨ Typing..."
}

func (r *BotServiceImpl) chatStream(user *model.User, chat *pkg.TelegramIncommingChat, messages []provider.Message) (*pkg.TelegramSendMessageStatus, string, error) {
	messageId := 0
	streamingContent := ""
	bufferThreshold := 500
	bufferedContent := ""

	send, err := pkg.SendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, indicator("typing"), false)
	if err != nil || !send.Ok {
		log.Println(err)
	}

	messageId = send.Result.MessageId
	err = r.llmProvider.ChatStream(user.Model, messages, func(partial provider.Message) error {
		loading := indicator("typing")
		chunk := partial.Content
		if partial.ToolCalls != nil {
			loading = indicator("tool")
		} else {
			streamingContent += chunk.(string)
			bufferedContent += chunk.(string)
		}

		if len(bufferedContent) >= bufferThreshold || partial.ToolCalls != nil {
			editMessage, err := pkg.EditTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, messageId, streamingContent+"\n"+loading, false)
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

	editMessage, err := pkg.EditTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, messageId, utils.Watermark(streamingContent, user.Model), true)
	if err != nil || !editMessage.Ok {
		_, err := pkg.EditTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, messageId, utils.Watermark(streamingContent, user.Model), false)
		if err != nil {
			log.Println(err)
			return nil, "", err
		}
	}
	err = nil
	return editMessage, streamingContent, err
}
