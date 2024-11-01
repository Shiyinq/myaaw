package service

import (
	"log"
	"teo/internal/pkg"
	"teo/internal/provider"
	"teo/internal/utils"
)

type MessageFactory interface {
	CreateMessage(chat *pkg.TelegramIncommingChat) provider.Message
}

type ImageMessage struct{}

func NewImageMessage() MessageFactory {
	return &ImageMessage{}
}

func (f *ImageMessage) CreateMessage(chat *pkg.TelegramIncommingChat) provider.Message {
	var fileID string
	var newMessage provider.Message

	if chat.Message.Photo != nil {
		fileID = chat.Message.Photo[len(chat.Message.Photo)-1].FileID
	} else if chat.Message.Document != nil {
		fileID = chat.Message.Document.FileID
	}

	path, err := pkg.GetFilePath(fileID)
	if err != nil {
		log.Println(err)
		return newMessage
	}
	base64, err := pkg.ImageURLToBase64(path)
	if err != nil {
		log.Println(err)
		return newMessage
	}

	newMessage.Role = "user"
	newMessage.Content = utils.GetImageCaption(chat.Message.Caption)
	newMessage.Images = append(newMessage.Images, base64)

	return newMessage
}

type ImageMessageType2 struct{}

func NewImageMessageType2() MessageFactory {
	return &ImageMessageType2{}
}

func (f *ImageMessageType2) CreateMessage(chat *pkg.TelegramIncommingChat) provider.Message {
	var fileID string
	var newMessage provider.Message

	if chat.Message.Photo != nil {
		fileID = chat.Message.Photo[len(chat.Message.Photo)-1].FileID
	} else if chat.Message.Document != nil {
		fileID = chat.Message.Document.FileID
	}

	path, err := pkg.GetFilePath(fileID)
	if err != nil {
		return newMessage
	}

	newMessage.Role = "user"
	newMessage.Content = []provider.ContentItem{
		{
			Type: "text",
			Text: utils.GetImageCaption(chat.Message.Caption),
		},
		{
			Type: "image_url",
			ImageURL: &provider.ImageInfo{
				URL: pkg.TelegramImageURL(path),
			},
		},
	}

	return newMessage
}

type TextMessage struct{}

func NewTextMessage() MessageFactory {
	return &TextMessage{}
}

func (f *TextMessage) CreateMessage(chat *pkg.TelegramIncommingChat) provider.Message {
	return provider.Message{
		Role:    "user",
		Content: chat.Message.Text,
	}
}

func NewMessage(provider string, chat *pkg.TelegramIncommingChat) provider.Message {
	var factory MessageFactory

	isGroq := provider == "groq"
	isOpenAI := provider == "openai"
	isMistral := provider == "mistral"
	hasPhoto := chat.Message.Photo != nil
	hasDocument := chat.Message.Document != nil

	switch {
	case (hasPhoto || hasDocument) && (isOpenAI || isMistral || isGroq):
		factory = NewImageMessageType2()
	case hasPhoto || hasDocument:
		factory = NewImageMessage()
	default:
		factory = NewTextMessage()
	}

	return factory.CreateMessage(chat)
}
