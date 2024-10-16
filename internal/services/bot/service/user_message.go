package service

import (
	"log"
	"teo/internal/provider"
	"teo/internal/services/bot/model"
)

type MessageFactory interface {
	CreateMessage(chat *model.TelegramIncommingChat) provider.Message
}

type ImageMessageFactory struct{}

func (f *ImageMessageFactory) CreateMessage(chat *model.TelegramIncommingChat) provider.Message {
	var newMessage provider.Message

	if chat.Message.Photo != nil {
		fileID := chat.Message.Photo[len(chat.Message.Photo)-1].FileID
		path, err := getFilePath(fileID)
		if err != nil {
			log.Println(err)
			return newMessage
		}
		base64, err := imageURLToBase64(path)
		if err != nil {
			log.Println(err)
			return newMessage
		}
		newMessage.Role = "user"
		newMessage.Content = getCaption(chat.Message.Caption)
		newMessage.Images = append(newMessage.Images, base64)
	}

	return newMessage
}

type DocumentMessageFactory struct{}

func (f *DocumentMessageFactory) CreateMessage(chat *model.TelegramIncommingChat) provider.Message {
	var newMessage provider.Message

	if chat.Message.Document != nil {
		fileID := chat.Message.Document.FileID
		path, err := getFilePath(fileID)
		if err != nil {
			log.Println(err)
			return newMessage
		}
		base64, err := imageURLToBase64(path)
		if err != nil {
			log.Println(err)
			return newMessage
		}
		newMessage.Role = "user"
		newMessage.Content = getCaption(chat.Message.Caption)
		newMessage.Images = append(newMessage.Images, base64)
	}

	return newMessage
}

type TextMessageFactory struct{}

func (f *TextMessageFactory) CreateMessage(chat *model.TelegramIncommingChat) provider.Message {
	return provider.Message{
		Role:    "user",
		Content: chat.Message.Text,
	}
}

func getCaption(caption string) string {
	if caption != "" {
		return caption
	}
	return "Explain this image"
}

func MessageHandler(chat *model.TelegramIncommingChat) provider.Message {
	var factory MessageFactory
	if chat.Message.Photo != nil {
		factory = &ImageMessageFactory{}
	} else if chat.Message.Document != nil {
		factory = &DocumentMessageFactory{}
	} else {
		factory = &TextMessageFactory{}
	}

	return factory.CreateMessage(chat)
}
