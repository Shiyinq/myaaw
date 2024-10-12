package service

import (
	"teo/internal/provider"
	"teo/internal/services/bot/model"
)

func (r *BotServiceImpl) conversation(user *model.User, chat *model.TelegramIncommingChat) (*provider.Message, error) {
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

	res, err := r.llmProvider.Chat(user.Model, messages)

	if err != nil {
		return nil, err
	}

	messages = append(messages, res)
	messages = messages[1:]
	updateError := r.userRepo.UpdateMessages(chat.Message.From.Id, &messages)

	if updateError != nil {
		return nil, err
	}

	return &res, nil
}
