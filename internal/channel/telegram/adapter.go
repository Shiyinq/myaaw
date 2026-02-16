package telegram

import (
	"encoding/json"
	"fmt"
	"log"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/utils"
)

type TelegramMeta struct {
	ChatID    int
	MessageID int
}

type TelegramAdapter struct {
	ttsProvider provider.TTSProvider
}

func NewTelegramAdapter(ttsProvider provider.TTSProvider) *TelegramAdapter {
	return &TelegramAdapter{
		ttsProvider: ttsProvider,
	}
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
	content := out.Text
	maxLen := 4096

	if len(content) <= maxLen {
		watermarked := utils.Watermark(utils.ParseTelegramMarkdown(content), "", config.WatermarkModel)
		send, err := SendTelegramMessage(meta.ChatID, meta.MessageID, watermarked, true)
		if err != nil || !send.Ok {

			watermarked = utils.Watermark(content, "", config.WatermarkModel)
			_, err = SendTelegramMessage(meta.ChatID, meta.MessageID, watermarked, false)
		}
		return err
	}

	var chunks []string
	for i := 0; i < len(content); i += maxLen {
		end := i + maxLen
		if end > len(content) {
			end = len(content)
		}
		chunks = append(chunks, content[i:end])
	}

	_, err := SendTelegramMessage(meta.ChatID, meta.MessageID, chunks[0], false)
	if err != nil {
		return err
	}

	for i := 1; i < len(chunks)-1; i++ {
		_, err := SendTelegramMessage(meta.ChatID, meta.MessageID, chunks[i], false)
		if err != nil {
			log.Println("Error sending chunk:", err)
		}
	}

	if len(chunks) > 1 {
		lastChunk := chunks[len(chunks)-1]
		watermarked := utils.Watermark(lastChunk, "", config.WatermarkModel)
		if len(watermarked) > maxLen {
			_, err = SendTelegramMessage(meta.ChatID, meta.MessageID, lastChunk, false)
		} else {
			_, err = SendTelegramMessage(meta.ChatID, meta.MessageID, watermarked, false)
		}
		if err != nil {
			log.Println("Error sending final chunk:", err)
		}
	}

	return nil
}

func (t *TelegramAdapter) SendStream(msg *channel.IncomingMessage, streamFn func(onChunk func(chunk channel.StreamChunk)) error) (*channel.OutgoingMessage, error) {
	meta := msg.RawMeta.(TelegramMeta)
	maxLen := 4096
	bufferThreshold := 500

	send, err := SendTelegramMessage(meta.ChatID, meta.MessageID, indicator("typing"), false)
	if err != nil || !send.Ok {
		log.Println(err)
	}
	messageId := send.Result.MessageId

	streamingContent := ""
	lastStreamingContent := ""
	bufferedContent := ""
	var traceSteps []provider.ReactStep
	lastTraceLen := 0
	lastLoading := ""
	var finalUsage provider.Usage

	err = streamFn(func(chunk channel.StreamChunk) {
		loading := indicator("typing")

		if chunk.Usage.TotalTokens > 0 {
			finalUsage = chunk.Usage
		}

		if chunk.ToolCalls != nil {
			toolName := "tool"
			if len(chunk.ToolCalls) > 0 {
				toolName = chunk.ToolCalls[0].Function.Name
			}
			loading = indicator("tool") + toolName + "..."
		} else if chunk.Text != "" {
			streamingContent += chunk.Text
			bufferedContent += chunk.Text
		}

		if len(chunk.Trace) > 0 {
			for i := lastTraceLen; i < len(chunk.Trace); i++ {
				step := chunk.Trace[i]
				if step.Observation != "" {
					action := step.Action
					log.Printf("[ReAct] Captured trace tool: %v", action)
					streamingContent += "\n\n🛠️ " + action + "\n\n"
					loading = indicator("typing")
				}
			}
			lastTraceLen = len(chunk.Trace)
			traceSteps = chunk.Trace
		}

		if len(streamingContent) >= maxLen-100 {
			streamingContent = streamingContent[len(lastStreamingContent):]
			bufferedContent = ""

			newSend, err := SendTelegramMessage(meta.ChatID, meta.MessageID, loading, false)
			if err != nil || !newSend.Ok {
				log.Println(err)
			} else {
				messageId = newSend.Result.MessageId
			}
		} else if len(bufferedContent) >= bufferThreshold || chunk.ToolCalls != nil || lastLoading != loading {
			editMessage, err := EditTelegramMessage(meta.ChatID, meta.MessageID, messageId, streamingContent+"\n\n"+loading, false)
			lastStreamingContent = streamingContent
			lastLoading = loading

			if err != nil || !editMessage.Ok {
				log.Println(err)
			}
			bufferedContent = ""
		}
	})

	if err != nil {
		return nil, err
	}

	watermarked := utils.Watermark(utils.ParseTelegramMarkdown(streamingContent), "", config.WatermarkModel)
	editMessage, err := EditTelegramMessage(meta.ChatID, meta.MessageID, messageId, watermarked, true)
	if err != nil || !editMessage.Ok {

		_, err := EditTelegramMessage(meta.ChatID, meta.MessageID, messageId, utils.Watermark(streamingContent, "", config.WatermarkModel), false)
		if err != nil {
			log.Println(err)
			return nil, err
		}
	}

	return &channel.OutgoingMessage{
		Text:  streamingContent,
		Trace: traceSteps,
		Usage: finalUsage,
	}, nil
}

func (t *TelegramAdapter) SendError(msg *channel.IncomingMessage, errText string) error {
	meta := msg.RawMeta.(TelegramMeta)
	_, err := SendTelegramMessage(meta.ChatID, 0, errText, true)
	return err
}

func indicator(text string) string {
	if text == "tool" {
		return "🛠️ Using "
	}
	return "✨ Typing..."
}

func (t *TelegramAdapter) transcribeVoice(fileID string) string {
	if t.ttsProvider == nil {
		log.Println("TTS provider is not available, transcription unavailable.")
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

	transcribed, err := t.ttsProvider.SpeechToText(audioData)
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
