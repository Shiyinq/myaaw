package service

import "teo/internal/services/bot/model"

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
