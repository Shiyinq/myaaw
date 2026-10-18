package service

import (
	"context"
	"errors"
	"log"
	"strings"

	"myaaw/internal/agent"
	"myaaw/internal/agent/skills"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
)

func (r *BotServiceImpl) conversation(user *model.User, msg *channel.IncomingMessage) (*channel.OutgoingMessage, error) {
	return r.runConversation(context.Background(), user, msg, nil)
}

func (r *BotServiceImpl) conversationStream(user *model.User, msg *channel.IncomingMessage, onChunk func(channel.StreamChunk)) (*channel.OutgoingMessage, error) {
	return r.runConversation(context.Background(), user, msg, onChunk)
}

func (r *BotServiceImpl) conversationContext(ctx context.Context, user *model.User, msg *channel.IncomingMessage) (*channel.OutgoingMessage, error) {
	return r.runConversation(ctx, user, msg, nil)
}

func (r *BotServiceImpl) conversationStreamContext(ctx context.Context, user *model.User, msg *channel.IncomingMessage, onChunk func(channel.StreamChunk)) (*channel.OutgoingMessage, error) {
	return r.runConversation(ctx, user, msg, onChunk)
}

func (r *BotServiceImpl) runConversation(
	ctx context.Context,
	user *model.User,
	msg *channel.IncomingMessage,
	onChunk func(channel.StreamChunk),
) (*channel.OutgoingMessage, error) {
	messages := r.buildConversationMessages(user, msg)

	cfg := agent.DefaultRunConfig(user.Model)
	cfg.ContextMaxTokens = config.AgentContextMaxTokens

	result, runErr := r.agent.RunEvents(ctx, cfg, messages, func(event agent.AgentEvent) error {
		if onChunk == nil {
			return nil
		}
		switch event.Type {
		case agent.EventMessageUpdate:
			if event.Delta != "" {
				onChunk(channel.StreamChunk{Kind: "text", Text: event.Delta})
			}
			if event.ThinkingDelta != "" {
				onChunk(channel.StreamChunk{Kind: "thought", Thought: event.ThinkingDelta})
			}
		case agent.EventToolExecutionStart:
			onChunk(channel.StreamChunk{
				Kind:       "tool",
				ToolCallID: event.ToolCallID,
				ToolCalls: []provider.ToolCall{{
					ID:   event.ToolCallID,
					Type: "function",
					Function: provider.FunctionCall{
						Name: event.ToolName,
					},
				}},
			})
		case agent.EventToolExecutionEnd:
			if event.ToolResult != nil {
				onChunk(channel.StreamChunk{
					Kind:       "tool_result",
					ToolCallID: event.ToolCallID,
					IsError:    event.ToolResult.IsError,
				})
			}
		case agent.EventTraceUpdate:
			if len(event.Trace) > 0 {
				onChunk(channel.StreamChunk{Kind: "trace", Trace: event.Trace})
			}
		case agent.EventUsage:
			if event.Usage.TotalTokens > 0 {
				onChunk(channel.StreamChunk{Kind: "usage", Usage: event.Usage})
			}
		}
		return nil
	})

	finalText := provider.MessageText(result.Final)
	if finalText == "" && result.StopReason == provider.StopMaxIterations {
		finalText = agent.MaxIterationsMessage
	}
	if finalText == "" && result.Err != nil {
		finalText = "⚠️ Agent error: " + result.Err.Error()
	}

	out := &channel.OutgoingMessage{
		Text:    finalText,
		Trace:   result.Final.Trace,
		Usage:   result.Usage,
		Thought: result.Final.Thought,
	}

	// Tell streaming consumers which text is the final answer. Intermediate
	// assistant text (before tool calls) must not be persisted or displayed as
	// the final message.
	if onChunk != nil {
		onChunk(channel.StreamChunk{
			Kind:    "final",
			Text:    finalText,
			Trace:   result.Final.Trace,
			Usage:   result.Usage,
			Thought: result.Final.Thought,
		})
	}

	skipPersistence := msg.TriggerType == "heartbeat" && strings.TrimSpace(finalText) == "HEARTBEAT_OK"
	if !skipPersistence {
		if err := r.saveConversation(user, msg, result.Transcript); err != nil {
			return nil, err
		}
	}

	if runErr != nil {
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			return out, runErr
		}
		// Provider errors are already surfaced in finalText; keep the partial
		// result and transcript instead of discarding them.
		log.Printf("Agent run ended with %s: %v", result.StopReason, runErr)
	}

	return out, nil
}

func (r *BotServiceImpl) buildConversationMessages(user *model.User, msg *channel.IncomingMessage) []provider.Message {
	userSystem := agent.NewSystemPromptBuilder(int64(user.UserId), msg.Channel).Build()
	userSystem += skills.GetSkillsInstruction()
	messages := []provider.Message{
		{
			Role:    "system",
			Content: userSystem,
			Version: 2,
		},
	}

	var conv *model.Conversation
	var err error
	if msg.ConversationID == "NEW" {
		conv = nil
		err = errors.New("force new")
	} else if msg.ConversationID != "" {
		conv, err = r.conversationRepo.GetConversationById(msg.ConversationID)
	}

	if conv == nil && msg.ConversationID != "NEW" {
		conv, err = r.conversationRepo.GetActiveConversationByUserId(user.UserId)
	}

	var convMessages []provider.Message
	if err == nil && conv != nil {
		convMessages = agent.MigrateLegacyTranscript(conv.Messages)
		msg.ConversationID = conv.Id
	} else {
		conv, err := r.conversationRepo.CreateConversation(user.UserId, "New Chat")
		if err == nil {
			convMessages = agent.MigrateLegacyTranscript(conv.Messages)
			msg.ConversationID = conv.Id
		} else {
			convMessages = []provider.Message{}
		}
	}

	messages = append(messages, convMessages...)
	messages = append(messages, r.buildUserMessage(msg))
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

	if msg.ReplyTo != "" {
		text = text + "\n\ncontex:\n" + msg.ReplyTo
		return provider.Message{Role: role, Content: text, Version: 2}
	}

	if len(msg.Images) > 0 {
		providerName := r.llmProvider.ProviderName()
		isOpenAI := providerName == "openai"
		isGroq := providerName == "groq"

		if isOpenAI || isGroq {
			contentItems := []provider.ContentItem{{Type: "text", Text: text}}
			for _, img := range msg.Images {
				contentItems = append(contentItems, provider.ContentItem{
					Type:     "image_url",
					ImageURL: &provider.ImageInfo{URL: "data:image/jpeg;base64," + img},
				})
			}
			return provider.Message{Role: role, Content: contentItems, Version: 2}
		}

		return provider.Message{Role: role, Content: text, Images: msg.Images, Version: 2}
	}

	return provider.Message{Role: role, Content: text, Version: 2}
}

func (r *BotServiceImpl) saveConversation(user *model.User, msg *channel.IncomingMessage, transcript []provider.Message) error {
	start := 0
	for start < len(transcript) && transcript[start].Role == "system" {
		start++
	}
	messages := make([]provider.Message, len(transcript)-start)
	copy(messages, transcript[start:])
	for i := range messages {
		messages[i].Version = 2
	}
	if len(messages) == 0 {
		return nil
	}

	var conv *model.Conversation
	var err error
	if msg.ConversationID != "" {
		conv, err = r.conversationRepo.GetConversationById(msg.ConversationID)
	}
	if conv == nil {
		conv, err = r.conversationRepo.GetActiveConversationByUserId(msg.UserID)
	}
	if err != nil && conv == nil {
		return err
	}
	if conv == nil || conv.Id == "" {
		return nil
	}

	title := ""
	if conv.Title == "" || conv.Title == "New Chat" {
		title, err = r.GenerateConversationTitle(user, messages)
		if err != nil {
			log.Printf("Failed to generate conversation title: %v", err)
			title = ""
		}
	}
	return r.conversationRepo.UpdateConversationById(conv.Id, messages, title)
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
	res, err := r.llmProvider.Chat(user.Model, llmMessages, nil)
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
