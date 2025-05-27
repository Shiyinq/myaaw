package service

import (
	"fmt"
	"log"
	"teo/internal/config"
	"teo/internal/pkg"
	"teo/internal/provider"
	"teo/internal/utils"
)

type MessageFactory interface {
	CreateMessage(chat *pkg.TelegramIncommingChat) provider.Message
}

// AudioMessage handles audio messages by transcribing them to text.
type AudioMessage struct {
	TTSProvider provider.TTSProvider
}

// NewAudioMessage creates a new MessageFactory for audio messages.
func NewAudioMessage(ttsProvider provider.TTSProvider) MessageFactory {
	return &AudioMessage{TTSProvider: ttsProvider}
}

// CreateMessage transcribes the audio message to text.
func (f *AudioMessage) CreateMessage(chat *pkg.TelegramIncommingChat) provider.Message {
	var fileID string
	var newMessage provider.Message
	var audioData []byte
	var err error

	if chat.Message.Voice != nil {
		fileID = chat.Message.Voice.FileID
	} else if chat.Message.Audio != nil {
		fileID = chat.Message.Audio.FileID
	} else {
		log.Println("AudioMessage CreateMessage called without Voice or Audio message")
		return provider.Message{Role: "user", Content: ""} // Or handle error appropriately
	}

	// 1. Get file path from Telegram
	filePath, err := pkg.GetFilePath(fileID)
	if err != nil {
		log.Printf("Error getting file path for fileID %s: %v\n", fileID, err)
		return provider.Message{Role: "user", Content: fmt.Sprintf("[Error getting file path: %s]", fileID)}
	}

	// 2. Download the file content
	audioData, err = pkg.DownloadTgFile(filePath)
	if err != nil {
		log.Printf("Error downloading audio file %s: %v\n", filePath, err)
		return provider.Message{Role: "user", Content: fmt.Sprintf("[Error downloading audio file: %s]", filePath)}
	}

	// 3. Get TTS Model from config (or use a default if not specified)
	// For now, pass an empty string to use the provider's default.
	ttsModel := "" // Or: config.TTSModel if you add it to your config

	// 4. Transcribe audio to text
	transcribedText, err := f.TTSProvider.SpeechToText(audioData, ttsModel)
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

// NewMessage creates the appropriate message factory based on the incoming message type and provider availability.
// llmProviderName is the name of the configured LLM provider (e.g., "openai", "groq").
// ttsProvider is the initialized TTS provider; it can be nil if TTS is not configured or failed to initialize.
func NewMessage(chat *pkg.TelegramIncommingChat, llmProviderName string, ttsProvider provider.TTSProvider) provider.Message {
	var factory MessageFactory

	isGroq := llmProviderName == config.ProviderGroq
	isOpenAI := llmProviderName == config.ProviderOpenAI
	isMistral := llmProviderName == config.ProviderMistral // Assuming config.ProviderMistral exists
	// If not, then llmProviderName == "mistral" would be the direct string comparison.
	// For consistency with how Groq and OpenAI might be checked (e.g. using constants from config),
	// it's good practice to define these provider names as constants in the config package.
	// Let's assume config.ProviderGroq, config.ProviderOpenAI, config.ProviderMistral are defined.
	// If config.ProviderMistral is not defined, the original string "mistral" is fine.
	// The original code used direct string comparison: `provider == "mistral"`. I'll stick to that if constants aren't there.
	// Re-checking original code: it was `provider == "groq"`, `provider == "openai"`, `provider == "mistral"`.
	// So, I'll use llmProviderName == "groq", etc.

	isGroq = llmProviderName == "groq"
	isOpenAI = llmProviderName == "openai"
	isMistral = llmProviderName == "mistral"

	hasPhoto := chat.Message.Photo != nil
	hasDocument := chat.Message.Document != nil
	isReplyToMessage := chat.Message.ReplyToMessage != nil
	isAudioMessage := chat.Message.Voice != nil || chat.Message.Audio != nil

	switch {
	case isAudioMessage:
		if ttsProvider == nil {
			log.Println("TTS provider is not available for AudioMessage factory, transcription unavailable.")
			// Return a message indicating that transcription is not available.
			// The user will receive this as a text message.
			return provider.Message{Role: "user", Content: "[Audio transcription not available]"}
		}
		// Use the ttsProvider passed into the function
		factory = NewAudioMessage(ttsProvider)

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
