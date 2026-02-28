package telegram

import (
	"encoding/json"
	"fmt"
	"html"
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
	transcriber provider.Transcriber
}

func NewTelegramAdapter(transcriber provider.Transcriber) *TelegramAdapter {
	return &TelegramAdapter{
		transcriber: transcriber,
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

	if out.Thought != "" {
		strippedThought := utils.StripMarkdown(out.Thought)
		displayThought := fmt.Sprintf("💭 <b>Reasoning...</b>\n<blockquote expandable>%s</blockquote>", html.EscapeString(strippedThought))
		send, err := SendTelegramMessage(meta.ChatID, meta.MessageID, displayThought, "HTML")
		if err != nil || !send.Ok {
			log.Println(err)
		}
	}

	if len(content) <= maxLen {
		watermarked := utils.Watermark(utils.ParseTelegramMarkdown(content), "", config.WatermarkModel)
		send, err := SendTelegramMessage(meta.ChatID, meta.MessageID, watermarked, "markdown")
		if err != nil || !send.Ok {

			watermarked = utils.Watermark(content, "", config.WatermarkModel)
			_, err = SendTelegramMessage(meta.ChatID, meta.MessageID, watermarked, "markdown")
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

	_, err := SendTelegramMessage(meta.ChatID, meta.MessageID, chunks[0], "markdown")
	if err != nil {
		return err
	}

	for i := 1; i < len(chunks)-1; i++ {
		_, err := SendTelegramMessage(meta.ChatID, meta.MessageID, chunks[i], "markdown")
		if err != nil {
			log.Println("Error sending chunk:", err)
		}
	}

	if len(chunks) > 1 {
		lastChunk := chunks[len(chunks)-1]
		watermarked := utils.Watermark(lastChunk, "", config.WatermarkModel)
		if len(watermarked) > maxLen {
			_, err = SendTelegramMessage(meta.ChatID, meta.MessageID, lastChunk, "markdown")
		} else {
			_, err = SendTelegramMessage(meta.ChatID, meta.MessageID, watermarked, "markdown")
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

	send, err := SendTelegramMessage(meta.ChatID, meta.MessageID, indicator("typing"), "markdown")
	if err != nil || !send.Ok {
		log.Println(err)
	}
	firstMessageId := send.Result.MessageId

	streamingContent := ""
	streamingThought := "" // Store thought and trace output here
	lastStreamingContent := ""
	bufferedContent := ""
	bufferedThought := ""
	var traceSteps []provider.ReactStep
	lastTraceLen := 0
	lastLoading := ""
	var finalUsage provider.Usage

	thoughtMessageId := 0
	mainMessageId := 0

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
		}

		if chunk.Thought != "" {
			streamingThought += chunk.Thought
			bufferedThought += chunk.Thought
			loading = indicator("reasoning")
		}

		if chunk.Text != "" {
			streamingContent += chunk.Text
			bufferedContent += chunk.Text
		}

		if len(chunk.Trace) > 0 {
			for i := lastTraceLen; i < len(chunk.Trace); i++ {
				step := chunk.Trace[i]
				if step.Observation != "" {
					action := step.Action
					log.Printf("[ReAct] Captured trace tool: %v", action)
					streamingThought += "\n\n🛠️ " + action + "\n\n"
					bufferedThought += "\n\n🛠️ " + action + "\n\n"
					loading = indicator("typing")
				}
			}
			lastTraceLen = len(chunk.Trace)
			traceSteps = chunk.Trace
		}

		// Handle UI updates based on whether we are showing thoughts or main text
		isThoughtUpdate := len(bufferedThought) >= bufferThreshold || chunk.ToolCalls != nil || (streamingThought != "" && lastLoading != loading)
		isMainUpdate := len(bufferedContent) >= bufferThreshold || (streamingContent != "" && lastLoading != loading)

		if streamingThought != "" {
			if thoughtMessageId == 0 {
				if mainMessageId == 0 {
					thoughtMessageId = firstMessageId
				} else {
					// Main message already took the first bubble. Spin up a new one for thoughts.
					newSend, err := SendTelegramMessage(meta.ChatID, meta.MessageID, loading, "markdown")
					if err != nil || !newSend.Ok {
						log.Println(err)
						thoughtMessageId = mainMessageId // Fallback to sharing
					} else {
						thoughtMessageId = newSend.Result.MessageId
					}
				}
			}

			if len(streamingThought) >= maxLen-100 {
				// Prevent thoughts from exceeding message max length
				streamingThought = streamingThought[(len(streamingThought) - lastTraceLen):] // Very rough truncation safeguard
				bufferedThought = ""
			} else if isThoughtUpdate {
				strippedThought := utils.StripMarkdown(streamingThought)
				displayThought := fmt.Sprintf("💭 <b>Reasoning...</b>\n<blockquote expandable>%s</blockquote>", html.EscapeString(strippedThought))
				editMessage, err := EditTelegramMessage(meta.ChatID, meta.MessageID, thoughtMessageId, displayThought+"\n\n"+html.EscapeString(loading), "HTML")
				if err != nil || !editMessage.Ok {
					log.Println(err)
				}
				bufferedThought = ""
			}
		}

		if streamingContent != "" {
			if mainMessageId == 0 {
				if thoughtMessageId == 0 {
					mainMessageId = firstMessageId
				} else {
					// Finalize the thought message without the loading indicator
					parsedThought := utils.ParseTelegramHTML(html.EscapeString(streamingThought))
					displayThought := fmt.Sprintf("💭 <b>Reasoning...</b>\n<blockquote expandable>%s</blockquote>", parsedThought)
					EditTelegramMessage(meta.ChatID, meta.MessageID, thoughtMessageId, displayThought, "HTML")

					newSend, err := SendTelegramMessage(meta.ChatID, meta.MessageID, loading, "markdown")
					if err != nil || !newSend.Ok {
						log.Println(err)
						// Fallback: overwrite the thought message if creation fails
						mainMessageId = thoughtMessageId
					} else {
						mainMessageId = newSend.Result.MessageId
					}
				}
			}

			if len(streamingContent) >= maxLen-100 {
				streamingContent = streamingContent[len(lastStreamingContent):]
				bufferedContent = ""

				newSend, err := SendTelegramMessage(meta.ChatID, meta.MessageID, loading, "markdown")
				if err != nil || !newSend.Ok {
					log.Println(err)
				} else {
					mainMessageId = newSend.Result.MessageId
				}
			} else if isMainUpdate {
				strippedContent := utils.StripMarkdown(streamingContent)
				editMessage, err := EditTelegramMessage(meta.ChatID, meta.MessageID, mainMessageId, strippedContent+"\n\n"+utils.EscapeMarkdown(loading), "markdown")
				lastStreamingContent = streamingContent

				if err != nil || !editMessage.Ok {
					log.Println(err)
				}
				bufferedContent = ""
			}
		}

		lastLoading = loading
	})

	if err != nil {
		return nil, err
	}

	// Make sure the thought bubble has the indicator removed if no text ever came
	if thoughtMessageId > 0 && streamingThought != "" {
		parsedThought := utils.ParseTelegramHTML(html.EscapeString(streamingThought))
		displayThought := fmt.Sprintf("💭 <b>Reasoning...</b>\n<blockquote expandable>%s</blockquote>", parsedThought)
		EditTelegramMessage(meta.ChatID, meta.MessageID, thoughtMessageId, displayThought, "HTML")
	}

	// If no main message was ever created, use the original messageId for the final watermark (or if it's empty)
	targetMessageId := mainMessageId
	if targetMessageId == 0 {
		targetMessageId = thoughtMessageId
	}
	if targetMessageId == 0 {
		targetMessageId = firstMessageId
	}

	watermarked := utils.Watermark(utils.ParseTelegramMarkdown(streamingContent), "", config.WatermarkModel)
	editMessage, err := EditTelegramMessage(meta.ChatID, meta.MessageID, targetMessageId, watermarked, "markdown")
	if err != nil || !editMessage.Ok {

		_, err := EditTelegramMessage(meta.ChatID, meta.MessageID, targetMessageId, utils.Watermark(streamingContent, "", config.WatermarkModel), "markdown")
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
