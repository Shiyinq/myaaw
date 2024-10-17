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
	var fileID string
	var newMessage provider.Message

	if chat.Message.Photo != nil {
		fileID = chat.Message.Photo[len(chat.Message.Photo)-1].FileID
	} else if chat.Message.Document != nil {
		fileID = chat.Message.Document.FileID
	}

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

	return newMessage
}

type ImageMessageType2Factory struct{}

func (f *ImageMessageType2Factory) CreateMessage(chat *model.TelegramIncommingChat) provider.Message {
	var fileID string
	var newMessage provider.Message

	if chat.Message.Photo != nil {
		fileID = chat.Message.Photo[len(chat.Message.Photo)-1].FileID
	} else if chat.Message.Document != nil {
		fileID = chat.Message.Document.FileID
	}

	path, err := getFilePath(fileID)
	if err != nil {
		return newMessage
	}

	newMessage.Role = "user"
	newMessage.Content = []provider.ContentItem{
		{
			Type: "text",
			Text: getCaption(chat.Message.Caption),
		},
		{
			Type: "image_url",
			ImageURL: &provider.ImageInfo{
				URL: telegramImageURL(path),
			},
		},
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

func MessageHandler(provider string, chat *model.TelegramIncommingChat) provider.Message {
	var factory MessageFactory

	if chat.Message.Photo != nil && provider == "openai" {
		factory = &ImageMessageType2Factory{}
	} else if chat.Message.Document != nil && provider == "openai" {
		factory = &ImageMessageType2Factory{}
	} else if chat.Message.Photo != nil {
		factory = &ImageMessageFactory{}
	} else if chat.Message.Document != nil {
		factory = &ImageMessageFactory{}
	} else {
		factory = &TextMessageFactory{}
	}

	return factory.CreateMessage(chat)
}
