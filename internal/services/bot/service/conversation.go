package service

import (
	"log"
	"myaaw/internal/agent"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
	"strings"
)

func (r *BotServiceImpl) conversation(user *model.User, msg *channel.IncomingMessage) (*channel.OutgoingMessage, error) {
	messages := r.buildConversationMessages(user, msg)
	context := r.contextWindow(messages)

	res, err := r.agent.Run(user.Model, context)
	if err != nil {
		return nil, err
	}

	content := res.Content.(string)
	response := provider.Message{
		Role:    "assistant",
		Content: content,
		Trace:   res.Trace,
		Usage:   res.Usage,
		Thought: res.Thought,
	}

	if err := r.updateUserMessages(msg, messages, response); err != nil {
		return nil, err
	}

	return &channel.OutgoingMessage{
		Text:    content,
		Trace:   response.Trace,
		Usage:   response.Usage,
		Thought: response.Thought,
	}, nil
}

func (r *BotServiceImpl) conversationStream(user *model.User, msg *channel.IncomingMessage, onChunk func(channel.StreamChunk)) (*channel.OutgoingMessage, error) {
	messages := r.buildConversationMessages(user, msg)
	context := r.contextWindow(messages)

	streamingContent := ""
	var traceSteps []provider.ReactStep
	var finalUsage provider.Usage
	var thought string

	err := r.agent.RunStream(user.Model, context, func(partial provider.Message) error {
		chunk := channel.StreamChunk{
			ToolCalls: partial.ToolCalls,
			Trace:     partial.Trace,
			Usage:     partial.Usage,
		}

		if partial.Usage.TotalTokens > 0 {
			finalUsage = partial.Usage
		}

		if partial.Thought != "" {
			text := partial.Thought
			chunk.Thought = text
			thought += text
		}

		if partial.Content != nil {
			text := partial.Content.(string)
			streamingContent += text
			chunk.Text = text
		}

		if len(partial.Trace) > 0 {
			traceSteps = partial.Trace
		}

		onChunk(chunk)
		return nil
	})

	if err != nil {
		return nil, err
	}

	response := provider.Message{
		Role:    "assistant",
		Content: streamingContent,
		Trace:   traceSteps,
		Usage:   finalUsage,
		Thought: thought,
	}

	if err := r.updateUserMessages(msg, messages, response); err != nil {
		return nil, err
	}

	return &channel.OutgoingMessage{
		Text:    streamingContent,
		Trace:   traceSteps,
		Usage:   finalUsage,
		Thought: thought,
	}, nil
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

func (r *BotServiceImpl) buildConversationMessages(user *model.User, msg *channel.IncomingMessage) []provider.Message {
	userSystem := agent.NewSystemPromptBuilder(int64(user.UserId), msg.Channel).Build()
	userSystem += agent.GetSkillsInstruction()
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

	// Build user message from generic IncomingMessage
	newMessage := r.buildUserMessage(msg)
	messages = append(messages, newMessage)

	return messages
}

// buildUserMessage converts a generic IncomingMessage to a provider.Message.
func (r *BotServiceImpl) buildUserMessage(msg *channel.IncomingMessage) provider.Message {
	text := msg.Text
	role := "user"
	switch msg.TriggerType {
	case "heartbeat":
		text = "[SYSTEM TRIGGER: HEARTBEAT]\n" + text
	case "cron":
		text = "[SYSTEM TRIGGER: CRON JOB]\n(NOTE: The user does NOT see this trigger message. You must now deliver the reminder or perform the scheduled task for the user based on this prompt)\n\n" + text
	case "subagent":
		text = "[SYSTEM TRIGGER: SUB-AGENT RESULT]\n(NOTE: The user does NOT see this trigger message. A background sub-agent has completed its task. Review its report below, inform the user about the completion, and summarize the key findings/results. If you started multiple sub-agents, check the batch progress.)\n\n" + text
	}

	// Voice: text is already transcribed by channel adapter
	// Text with reply context
	if msg.ReplyTo != "" {
		text = text + "\n\ncontex:\n" + msg.ReplyTo
		return provider.Message{
			Role:    role,
			Content: text,
		}
	}

	// Image message
	if len(msg.Images) > 0 {
		providerName := r.llmProvider.ProviderName()
		isOpenAI := providerName == "openai"
		isGroq := providerName == "groq"

		if isOpenAI || isGroq {
			// Type 2: content items with image_url
			contentItems := []provider.ContentItem{
				{
					Type: "text",
					Text: text,
				},
			}
			for _, img := range msg.Images {
				contentItems = append(contentItems, provider.ContentItem{
					Type: "image_url",
					ImageURL: &provider.ImageInfo{
						URL: "data:image/jpeg;base64," + img,
					},
				})
			}
			return provider.Message{
				Role:    role,
				Content: contentItems,
			}
		}

		// Type 1: images array (Ollama, Gemini)
		return provider.Message{
			Role:    role,
			Content: text,
			Images:  msg.Images,
		}
	}

	// Plain text
	return provider.Message{
		Role:    role,
		Content: text,
	}
}

func (r *BotServiceImpl) updateUserMessages(msg *channel.IncomingMessage, messages []provider.Message, response provider.Message) error {
	if msg.TriggerType == "heartbeat" {
		if contentStr, ok := response.Content.(string); ok && strings.TrimSpace(contentStr) == "HEARTBEAT_OK" {
			return nil
		}
	}

	messages = append(messages, response)
	messages = messages[1:] // exclude system message

	conv, err := r.conversationRepo.GetActiveConversationByUserId(msg.UserID)
	if err != nil && conv != nil {
		return err
	}

	if conv != nil && conv.Id != "" {
		convId := conv.Id
		title := ""
		if conv.Title == "" || conv.Title == "New Chat" {
			user, err := r.userRepo.GetUserById(msg.UserID)
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

func (r *BotServiceImpl) factoryChat(user *model.User, messages []provider.Message) (provider.Message, error) {
	log.Println("Processing incoming message")
	if config.StreamResponse {
		log.Println("Starting content streaming")
		return r.chatStream(user, messages)
	}
	return r.chat(user, messages)
}

func (r *BotServiceImpl) chat(user *model.User, messages []provider.Message) (provider.Message, error) {
	res, err := r.agent.Run(user.Model, messages)
	if err != nil {
		return provider.Message{}, err
	}

	content := res.Content.(string)
	return provider.Message{
		Role:    "assistant",
		Content: content,
		Trace:   res.Trace,
		Usage:   res.Usage,
	}, nil
}

func (r *BotServiceImpl) chatStream(user *model.User, messages []provider.Message) (provider.Message, error) {
	streamingContent := ""
	var traceSteps []provider.ReactStep
	var finalUsage provider.Usage

	err := r.agent.RunStream(user.Model, messages, func(partial provider.Message) error {
		if partial.Usage.TotalTokens > 0 {
			finalUsage = partial.Usage
		}

		if partial.ToolCalls != nil {
			// Tool calls are handled by agent loop, nothing to collect here for content
		} else if partial.Content != nil {
			streamingContent += partial.Content.(string)
		}

		if len(partial.Trace) > 0 {
			traceSteps = partial.Trace
		}

		return nil
	})

	if err != nil {
		return provider.Message{}, err
	}

	return provider.Message{
		Role:    "assistant",
		Content: streamingContent,
		Trace:   traceSteps,
		Usage:   finalUsage,
	}, nil
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
