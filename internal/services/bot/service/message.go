package service

import (
	"fmt"
	"log"
	"teo/internal/pkg"
	"teo/internal/provider"
	"teo/internal/utils"
)

type MessageFactory interface {
	CreateMessage(chat *pkg.TelegramIncommingChat) provider.Message
}

type VoiceMessage struct {
	TTSProvider provider.TTSProvider
}

func NewVoiceMessage(ttsProvider provider.TTSProvider) MessageFactory {
	return &VoiceMessage{TTSProvider: ttsProvider}
}

func (f *VoiceMessage) CreateMessage(chat *pkg.TelegramIncommingChat) provider.Message {
	var fileID string
	var newMessage provider.Message
	var audioData []byte
	var err error

	if chat.Message.Voice != nil {
		fileID = chat.Message.Voice.FileID
	} else {
		log.Println("VoiceMessage CreateMessage called without Voice or Voice message")
		return provider.Message{Role: "user", Content: ""}
	}

	filePath, err := pkg.GetFilePath(fileID)
	if err != nil {
		log.Printf("Error getting file path for fileID %s: %v\n", fileID, err)
		return provider.Message{Role: "user", Content: fmt.Sprintf("[Error getting file path: %s]", fileID)}
	}

	audioData, err = pkg.DownloadTgFile(filePath)
	if err != nil {
		log.Printf("Error downloading audio file %s: %v\n", filePath, err)
		return provider.Message{Role: "user", Content: fmt.Sprintf("[Error downloading audio file: %s]", filePath)}
	}

	transcribedText, err := f.TTSProvider.SpeechToText(audioData)
	if err != nil {
		log.Printf("Error transcribing audio: %v\n", err)
		return provider.Message{Role: "user", Content: "[Error transcribing audio]"}
	}

	newMessage.Role = "user"
	newMessage.Content = transcribedText
	return newMessage
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

type ReplyToMessage struct{}

func NewReplyToMessage() MessageFactory {
	return &ReplyToMessage{}
}

func (f *ReplyToMessage) CreateMessage(chat *pkg.TelegramIncommingChat) provider.Message {
	text := chat.Message.Text
	text += "\n\ncontex:\n" + chat.Message.ReplyToMessage.Text

	return provider.Message{
		Role:    "user",
		Content: text,
	}
}

func NewMessage(chat *pkg.TelegramIncommingChat, llmProviderName string, ttsProvider provider.TTSProvider) provider.Message {
	var factory MessageFactory

	isGroq := llmProviderName == "groq"
	isOpenAI := llmProviderName == "openai"
	isMistral := llmProviderName == "mistral"

	hasPhoto := chat.Message.Photo != nil
	hasDocument := chat.Message.Document != nil
	isReplyToMessage := chat.Message.ReplyToMessage != nil
	isVoiceMessage := chat.Message.Voice != nil

	switch {
	case isVoiceMessage:
		if ttsProvider == nil {
			log.Println("TTS provider is not available for VoiceMessage factory, transcription unavailable.")
			return provider.Message{Role: "user", Content: "[Voice transcription not available]"}
		}
		factory = NewVoiceMessage(ttsProvider)

	case (hasPhoto || hasDocument) && (isOpenAI || isMistral || isGroq):
		factory = NewImageMessageType2()
	case hasPhoto || hasDocument:
		factory = NewImageMessage()
	case isReplyToMessage:
		factory = NewReplyToMessage()
	default:
		factory = NewTextMessage()
	}

	return factory.CreateMessage(chat)
}
