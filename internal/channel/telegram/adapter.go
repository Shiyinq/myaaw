package telegram

import (
	"encoding/json"
	"fmt"
	"log"
	"myaaw/internal/channel"
	"myaaw/internal/provider"
	"myaaw/internal/utils"
	"time"
)

type TelegramMeta struct {
	ChatID    int
	MessageID int
}

type TelegramAdapter struct {
	transcriber provider.Transcriber
	transport   telegramTransport
}

func NewTelegramAdapter(transcriber provider.Transcriber) *TelegramAdapter {
	return &TelegramAdapter{
		transcriber: transcriber,
		transport:   telegramAPI{},
	}
}

func (t *TelegramAdapter) transportOrAPI() telegramTransport {
	if t.transport != nil {
		return t.transport
	}
	return telegramAPI{}
}

func (t *TelegramAdapter) Name() string {
	return "telegram"
}

func (t *TelegramAdapter) ParseIncoming(payload json.RawMessage) (*channel.IncomingMessage, error) {
	var chat TelegramIncommingChat
	if err := json.Unmarshal(payload, &chat); err != nil {
		return nil, fmt.Errorf("failed to parse telegram payload: %w", err)
	}

	msg := &channel.IncomingMessage{
		UserID:  chat.Message.From.Id,
		Channel: "telegram",
		RawMeta: TelegramMeta{
			ChatID:    chat.Message.Chat.Id,
			MessageID: chat.Message.MessageId,
		},
	}

	if chat.Message.Voice != nil {
		msg.Text = t.transcribeVoice(chat.Message.Voice.FileID)
		return msg, nil
	}

	if chat.Message.Photo != nil || chat.Message.Document != nil {
		msg.Text = utils.GetImageCaption(chat.Message.Caption)
		msg.Images = t.extractImages(&chat)
		return msg, nil
	}

	if chat.Message.ReplyToMessage != nil {
		msg.Text = chat.Message.Text
		msg.ReplyTo = chat.Message.ReplyToMessage.Text
		return msg, nil
	}

	msg.Text = chat.Message.Text
	return msg, nil
}

func (t *TelegramAdapter) Send(msg *channel.IncomingMessage, out *channel.OutgoingMessage) error {
	meta := msg.RawMeta.(TelegramMeta)
	items := buildTelegramTimeline(out.Trace, out.Text, out.Thought)
	if len(items) == 0 {
		return nil
	}

	timeline := &telegramTimeline{
		transport: t.transportOrAPI(),
		chatID:    meta.ChatID,
		replyTo:   meta.MessageID,
		tools:     map[string]*telegramTimelineItem{},
		now:       time.Now,
	}

	for i, item := range items {
		if item.Kind == "text" {
			timeline.writeText(item, item.Text, i == len(items)-1)
			continue
		}
		content, mode := timeline.render(item, true)
		item.MessageID = timeline.sendRaw(content, mode)
	}
	return timeline.err
}

func (t *TelegramAdapter) SendStream(msg *channel.IncomingMessage, streamFn func(onChunk func(chunk channel.StreamChunk)) error) (*channel.OutgoingMessage, error) {
	meta := msg.RawMeta.(TelegramMeta)
	transport := t.transportOrAPI()

	placeholderID := 0
	if sendID, err := transport.Send(meta.ChatID, meta.MessageID, indicator("typing"), "markdown"); err != nil {
		log.Println(err)
	} else {
		placeholderID = sendID
	}

	timeline := newTelegramTimeline(transport, meta.ChatID, meta.MessageID, placeholderID)
	err := streamFn(timeline.handleChunk)
	timeline.finish()
	if err != nil {
		return nil, err
	}

	return &channel.OutgoingMessage{
		Text:  timeline.finalText,
		Trace: timeline.finalTrace,
		Usage: timeline.finalUsage,
	}, nil
}

func (t *TelegramAdapter) SendError(msg *channel.IncomingMessage, errText string) error {
	meta := msg.RawMeta.(TelegramMeta)
	_, err := SendTelegramMessage(meta.ChatID, 0, errText, "markdown")
	return err
}

func indicator(text string) string {
	if text == "tool" {
		return "🛠️ Using "
	}
	return "✨ Typing..."
}

func (t *TelegramAdapter) transcribeVoice(fileID string) string {
	if t.transcriber == nil {
		log.Println("Transcriber provider is not available, transcription unavailable.")
		return "[Voice transcription not available]"
	}

	filePath, err := GetFilePath(fileID)
	if err != nil {
		log.Printf("Error getting file path for voice fileID %s: %v\n", fileID, err)
		return fmt.Sprintf("[Error getting file path: %s]", fileID)
	}

	audioData, err := DownloadTgFile(filePath)
	if err != nil {
		log.Printf("Error downloading audio file %s: %v\n", filePath, err)
		return fmt.Sprintf("[Error downloading audio file: %s]", filePath)
	}

	transcribed, err := t.transcriber.Transcribe(audioData)
	if err != nil {
		log.Printf("Error transcribing audio: %v\n", err)
		return "[Error transcribing audio]"
	}
	return transcribed
}

func (t *TelegramAdapter) extractImages(chat *TelegramIncommingChat) []string {
	var fileID string
	if chat.Message.Photo != nil {
		fileID = chat.Message.Photo[len(chat.Message.Photo)-1].FileID
	} else if chat.Message.Document != nil {
		fileID = chat.Message.Document.FileID
	}

	if fileID == "" {
		return nil
	}

	path, err := GetFilePath(fileID)
	if err != nil {
		log.Println("Error getting image file path:", err)
		return nil
	}

	base64, err := ImageURLToBase64(path)
	if err != nil {
		log.Println("Error converting image to base64:", err)
		return nil
	}

	return []string{base64}
}
