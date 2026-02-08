package service

import (
	"fmt"
	"log"
	"strings"
	"teo/internal/config"
	"teo/internal/pkg"
	"teo/internal/provider"
	"teo/internal/services/bot/model"
	"teo/internal/utils"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (r *BotServiceImpl) conversation(user *model.User, chat *pkg.TelegramIncommingChat) (*pkg.TelegramSendMessageStatus, error) {
	messages := r.buildConversationMessages(user, chat)
	context := r.contextWindow(messages)

	result, response, err := r.factoryChat(user, chat, context)
	if err != nil {
		return nil, err
	}

	if err := r.updateUserMessages(chat, messages, response); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *BotServiceImpl) contextWindow(history []provider.Message) []provider.Message {
	total := 10

	if total >= len(history) {
		total = len(history) - 1
	}

	context := make([]provider.Message, total+1)
	context[0] = history[0]

	for i := 1; i <= total; i++ {
		context[i] = history[len(history)-total+i-1]
	}

	return context
}

func (r *BotServiceImpl) buildConversationMessages(user *model.User, chat *pkg.TelegramIncommingChat) []provider.Message {
	userSystem := fmt.Sprintf("%s\n\n# User Info\n\nUser ID: %v (you can use this User ID for tools/skills if needed)\nToday's date is: %s", user.System, user.UserId, utils.GetCurrentTime())
	userSystem += utils.GetSkillsInstruction()
	messages := []provider.Message{
		{
			Role:    "system",
			Content: userSystem,
		},
	}

	conv, err := r.conversationRepo.GetActiveConversationByUserId(user.UserId)
	var convMessages []provider.Message
	if err == nil && conv != nil {
		convMessages = conv.Messages
	} else {
		title, err := r.GenerateConversationTitle(user, messages)
		if err != nil {
			title = "New Chat"
		}

		conv, err := r.conversationRepo.CreateConversation(user.UserId, title)
		if err == nil {
			convMessages = conv.Messages
		} else {
			convMessages = []provider.Message{}
		}
	}

	messages = append(messages, convMessages...)
	newMessage := NewMessage(chat, r.llmProvider.ProviderName(), r.ttsProvider)
	messages = append(messages, newMessage)

	return messages
}

func (r *BotServiceImpl) updateUserMessages(chat *pkg.TelegramIncommingChat, messages []provider.Message, response provider.Message) error {
	messages = append(messages, response)
	messages = messages[1:] // exclude system message

	conv, err := r.conversationRepo.GetActiveConversationByUserId(chat.Message.From.Id)
	var convId primitive.ObjectID
	if err != nil && conv != nil {
		return err
	}

	convId = conv.Id
	if convId != primitive.NilObjectID {
		title := ""
		if conv.Title == "" || conv.Title == "New Chat" {
			user, err := r.userRepo.GetUserById(chat.Message.From.Id)
			if err != nil {
				return err
			}
			title, err = r.GenerateConversationTitle(user, messages)
			if err != nil {
				return err
			}
		}
		return r.conversationRepo.UpdateConversationById(convId, messages, title)
	}

	return nil
}

func (r *BotServiceImpl) factoryChat(user *model.User, chat *pkg.TelegramIncommingChat, messages []provider.Message) (*pkg.TelegramSendMessageStatus, provider.Message, error) {
	var err error
	var response provider.Message
	var result *pkg.TelegramSendMessageStatus

	log.Println("Processing incoming message")
	if config.StreamResponse {
		log.Println("Starting content streaming")
		result, response, err = r.chatStream(user, chat, messages)
	} else {
		result, response, err = r.chat(user, chat, messages)
	}

	return result, response, err
}

func (r *BotServiceImpl) chat(user *model.User, chat *pkg.TelegramIncommingChat, messages []provider.Message) (*pkg.TelegramSendMessageStatus, provider.Message, error) {
	// Use Agent for iteration
	res, err := r.agent.Run(user.Model, messages)

	if err != nil {
		return nil, provider.Message{}, err
	}

	content := res.Content.(string)
	maxTelegramLength := 4096

	// Build response with trace from LLM
	response := provider.Message{
		Role:    "assistant",
		Content: content,
		Trace:   res.Trace,
	}

	if len(content) > maxTelegramLength {
		var chunks []string
		for i := 0; i < len(content); i += maxTelegramLength {
			end := i + maxTelegramLength
			if end > len(content) {
				end = len(content)
			}
			chunks = append(chunks, content[i:end])
		}

		send, err := pkg.SendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, chunks[0], false)
		if err != nil || !send.Ok {
			return nil, provider.Message{}, err
		}

		for i := 1; i < len(chunks)-1; i++ {
			_, err := pkg.SendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, chunks[i], false)
			if err != nil {
				log.Println("Error sending chunk:", err)
			}
		}

		if len(chunks) > 1 {
			lastChunk := chunks[len(chunks)-1]
			watermarkedChunk := utils.Watermark(lastChunk, user.Model, config.WatermarkModel)

			if len(watermarkedChunk) > maxTelegramLength {
				_, err := pkg.SendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, lastChunk, false)
				if err != nil {
					log.Println("Error sending final chunk:", err)
				}
			} else {
				_, err := pkg.SendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, watermarkedChunk, false)
				if err != nil {
					log.Println("Error sending final chunk:", err)
				}
			}
		}

		return send, response, nil
	}

	send, err := pkg.SendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, utils.Watermark(content, user.Model, config.WatermarkModel), true)
	if err != nil || !send.Ok {
		return nil, provider.Message{}, nil
	}

	return send, response, nil
}

func indicator(text string) string {
	if text == "tool" {
		return "🛠️ Using "
	}
	return "✨ Typing..."
}

func (r *BotServiceImpl) chatStream(user *model.User, chat *pkg.TelegramIncommingChat, messages []provider.Message) (*pkg.TelegramSendMessageStatus, provider.Message, error) {
	messageId := 0
	streamingContent := ""
	lastStreamingContent := ""
	bufferThreshold := 500
	bufferedContent := ""
	maxTelegramLength := 4096
	var traceSteps []provider.ReactStep
	lastTraceLen := 0
	lastLoading := ""

	send, err := pkg.SendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, indicator("typing"), false)
	if err != nil || !send.Ok {
		log.Println(err)
	}

	messageId = send.Result.MessageId

	// Use Agent for streaming iteration
	err = r.agent.RunStream(user.Model, messages, func(partial provider.Message) error {
		loading := indicator("typing")
		chunk := partial.Content
		if partial.ToolCalls != nil {
			// streamingContent += "\n"
			toolName := "tool"
			if len(partial.ToolCalls) > 0 {
				toolName = partial.ToolCalls[0].Function.Name
			}
			loading = indicator("tool") + toolName + "..."
		} else if chunk != nil {
			streamingContent += chunk.(string)
			bufferedContent += chunk.(string)
		}

		// Capture trace if present
		if len(partial.Trace) > 0 {
			// Iterate through all new steps since last check
			for i := lastTraceLen; i < len(partial.Trace); i++ {
				step := partial.Trace[i]
				// Only print if Observation is not empty (tool execution finished)
				if step.Observation != "" {
					action := step.Action
					log.Printf("[ReAct] Captured trace tool: %v", action)
					streamingContent += "\n\n🛠️ " + action + "\n\n"
					loading = indicator("typing")
				}
			}
			lastTraceLen = len(partial.Trace)
			traceSteps = partial.Trace
		}

		if len(streamingContent) >= maxTelegramLength-100 {
			streamingContent = streamingContent[len(lastStreamingContent):]
			bufferedContent = ""

			newSend, err := pkg.SendTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, loading, false)
			if err != nil || !newSend.Ok {
				log.Println(err)
			} else {
				messageId = newSend.Result.MessageId
			}
		} else if len(bufferedContent) >= bufferThreshold || partial.ToolCalls != nil || lastLoading != loading {
			editMessage, err := pkg.EditTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, messageId, streamingContent+"\n\n"+loading, false)

			lastStreamingContent = streamingContent
			lastLoading = loading

			if err != nil || !editMessage.Ok {
				log.Println(err)
			}
			bufferedContent = ""
		}

		return nil
	})

	if err != nil {
		return nil, provider.Message{}, err
	}

	editMessage, err := pkg.EditTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, messageId, utils.Watermark(utils.ParseTelegramMarkdown(streamingContent), user.Model, config.WatermarkModel), true)
	if err != nil || !editMessage.Ok {
		_, err := pkg.EditTelegramMessage(chat.Message.Chat.Id, chat.Message.MessageId, messageId, utils.Watermark(streamingContent, user.Model, config.WatermarkModel), false)
		if err != nil {
			log.Println(err)
			return nil, provider.Message{}, err
		}
	}

	// Return full message with trace
	response := provider.Message{
		Role:    "assistant",
		Content: streamingContent,
		Trace:   traceSteps,
	}
	return editMessage, response, nil
}

func (r *BotServiceImpl) GenerateConversationTitle(user *model.User, messages []provider.Message) (string, error) {
	defaultTitle := "New Chat"
	var firstUserMsg string
	for _, msg := range messages {
		if msg.Role == "user" && msg.Content != nil {
			if content, ok := msg.Content.(string); ok && content != "" {
				firstUserMsg = content
				break
			}
		}
	}
	if firstUserMsg == "" {
		return defaultTitle, nil
	}

	prompt := "Generate a short and clear conversation title (max 7 words) for the following user message: " + firstUserMsg
	llmMessages := []provider.Message{
		{Role: "system", Content: "You are a conversation title assistant. The title must be short, clear, and a maximum of 7 words."},
		{Role: "user", Content: prompt},
	}
	res, err := r.llmProvider.Chat(user.Model, llmMessages)
	if err != nil || res.Content == nil {
		return defaultTitle, err
	}
	if title, ok := res.Content.(string); ok && title != "" {
		title = strings.ReplaceAll(title, "\n", " ")
		title = strings.ReplaceAll(title, "\r", " ")
		title = strings.ReplaceAll(title, "\"", "")
		title = strings.TrimSpace(title)
		return title, nil
	}
	return defaultTitle, nil
}
