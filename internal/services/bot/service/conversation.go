package service

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/agent/skills"
	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/obs/agenttrace"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"

	"github.com/google/uuid"
)

var (
	ErrConversationNotFound  = errors.New("conversation not found")
	ErrConversationForbidden = errors.New("conversation belongs to another user")
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
	if err := r.requireProvider(); err != nil {
		return nil, err
	}

	messages, err := r.buildConversationMessages(user, msg)
	if err != nil {
		return nil, err
	}

	// Announce the conversation before the provider call: a run that is stopped
	// mid-stream never reaches "done", and the client still needs the id to
	// reopen what was saved.
	if onChunk != nil && msg.ConversationID != "" {
		onChunk(channel.StreamChunk{Kind: "conversation", ConversationID: msg.ConversationID})
	}

	cfg := agent.DefaultRunConfig(user.Model)
	cfg.ContextMaxTokens = config.AgentContextMaxTokens

	// Persist the transcript while the turn runs so a crash, restart, or kill
	// cannot lose the user message and the tool results already produced. The
	// heartbeat trigger is excluded because HEARTBEAT_OK turns must not enter the
	// transcript at all.
	if msg.TriggerType != "heartbeat" {
		var (
			lastCheckpoint  time.Time
			firstCheckpoint = true
		)
		cfg.OnCheckpoint = func(ctx context.Context, transcript []provider.Message) {
			now := time.Now()
			if !firstCheckpoint && now.Sub(lastCheckpoint) < conversationCheckpointInterval {
				return
			}
			firstCheckpoint = false
			lastCheckpoint = now
			r.checkpointConversation(msg, transcript)
		}
	}

	runID := ""
	recorder := r.traceRun(msg, user, messages)
	if recorder != nil {
		// One identifier for the engine events and the stored trace.
		runID = recorder.RunID()
		cfg.RunID = runID
		// Tools read this from ctx, so a subagent records itself as a child run.
		ctx = agenttrace.WithTrace(ctx, recorder)
	}

	// Attach the approval gate after the run id is known.
	if gate := r.approvalGate(ctx, msg); gate != nil {
		gate.Session.RunID = runID
		cfg.ApprovalGate = gate
	}

	result, runErr := r.agent.RunEvents(ctx, cfg, messages, func(event agent.AgentEvent) error {
		recorder.Event(event)
		if onChunk == nil {
			return nil
		}
		switch event.Type {
		case agent.EventStepStart:
			onChunk(channel.StreamChunk{Kind: "step", StepIndex: event.StepIndex})
		case agent.EventMessageUpdate:
			if event.Delta != "" {
				onChunk(channel.StreamChunk{Kind: "text", Text: event.Delta})
			}
			if event.ThinkingDelta != "" {
				onChunk(channel.StreamChunk{Kind: "thought", Thought: event.ThinkingDelta})
			}
		case agent.EventToolExecutionStart:
			onChunk(channel.StreamChunk{
				Kind:          "tool",
				ToolCallID:    event.ToolCallID,
				ToolName:      event.ToolName,
				ToolArguments: event.ToolArguments,
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
					Kind:           "tool_result",
					ToolCallID:     event.ToolCallID,
					ToolName:       event.ToolName,
					ToolResultText: agent.ToolResultText(*event.ToolResult),
					IsError:        event.ToolResult.IsError,
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
		case agent.EventLLMRetry:
			onChunk(channel.StreamChunk{Kind: "retry", StepIndex: event.StepIndex, Attempt: event.Attempt})
		case agent.EventContextCompacted:
			onChunk(channel.StreamChunk{
				Kind:            "compaction",
				StepIndex:       event.StepIndex,
				TokensBefore:    event.Compact.TokensBefore,
				TokensAfter:     event.Compact.TokensAfter,
				DroppedMessages: event.Compact.DroppedMessages,
			})
		}
		return nil
	})

	if recorder != nil {
		recorder.End(result)
		if err := recorder.Close(); err != nil {
			log.Printf("⚠️  Failed to write trace: %v", err)
		}
	}

	if onChunk != nil && len(result.Attachments) > 0 {
		onChunk(channel.StreamChunk{Kind: "attachment", Attachments: result.Attachments})
	}

	finalText := provider.MessageText(result.Final)
	if finalText == "" && result.StopReason == provider.StopMaxIterations {
		finalText = agent.MaxIterationsMessage
	}
	// A client stop is not a failure: keep the partial answer quiet instead of
	// surfacing "context canceled" as an agent error.
	stopped := agent.StoppedByClient(ctx)
	if finalText == "" && result.Err != nil && !stopped {
		finalText = "⚠️ Agent error: " + result.Err.Error()
	}

	out := &channel.OutgoingMessage{
		Text:           finalText,
		Trace:          result.Final.Trace,
		Usage:          result.Usage,
		Thought:        result.Final.Thought,
		ConversationID: msg.ConversationID,
		RunID:          runID,
		Attachments:    result.Attachments,
		Stopped:        stopped,
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
		if stopped {
			// The partial transcript above is already saved; report a clean stop.
			return out, nil
		}
		if errors.Is(runErr, context.Canceled) {
			return out, runErr
		}
		if errors.Is(runErr, context.DeadlineExceeded) {
			// The terminal message explains the timeout; keep the partial
			// timeline and transcript instead of turning it into an error UI.
			log.Printf("Agent run timed out: %v", runErr)
			return out, nil
		}
		// Provider errors are already surfaced in finalText; keep the partial
		// result and transcript instead of discarding them.
		log.Printf("Agent run ended with %s: %v", result.StopReason, runErr)
	}

	return out, nil
}

func (r *BotServiceImpl) buildConversationMessages(user *model.User, msg *channel.IncomingMessage) ([]provider.Message, error) {
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
	switch {
	case msg.ConversationID == "NEW":
		// Force a new conversation below.
	case msg.ConversationID != "":
		found, err := r.conversationRepo.GetConversationById(msg.ConversationID)
		if err != nil {
			return nil, err
		}
		switch {
		case found == nil && msg.Channel == "a2a":
			// An A2A contextId is caller-defined, so the first task that
			// uses one may not have a transcript yet. Create it under that
			// ID so later tasks continue the same conversation.
			created, err := r.conversationRepo.CreateConversationWithId(user.UserId, msg.ConversationID, "")
			if err != nil {
				return nil, err
			}
			conv = created
		case found == nil:
			return nil, ErrConversationNotFound
		case found.UserId != user.UserId:
			return nil, ErrConversationForbidden
		default:
			conv = found
		}
	default:
		active, err := r.conversationRepo.GetActiveConversationByUserId(user.UserId)
		if err != nil {
			return nil, err
		}
		conv = active
	}

	var convMessages []provider.Message
	if conv != nil {
		convMessages = agent.MigrateLegacyTranscript(conv.Messages)
		msg.ConversationID = conv.Id
	} else {
		created, err := r.conversationRepo.CreateConversation(user.UserId, "New Chat")
		if err != nil {
			return nil, err
		}
		convMessages = agent.MigrateLegacyTranscript(created.Messages)
		msg.ConversationID = created.Id
	}

	messages = append(messages, convMessages...)

	// A user upload is addressed by its own message id so the frontend can show
	// the files again under the right turn. The conversation id is already final
	// here (existing or freshly created), so the folder is stable before the run.
	messageID := uuid.New().String()
	var uploads []provider.Upload
	if len(msg.Files) > 0 {
		userDir, rootErr := agentdir.ActiveUserDir()
		if rootErr != nil {
			return nil, rootErr
		}
		var saveErr error
		uploads, saveErr = saveUserFiles(userDir, msg.ConversationID, messageID, msg.Files)
		if saveErr != nil {
			return nil, saveErr
		}
	}

	userMessage := r.buildUserMessage(msg, uploads)
	userMessage.ID = messageID
	userMessage.Uploads = uploads
	messages = append(messages, userMessage)
	return messages, nil
}

// buildUserMessage converts a generic IncomingMessage to a provider.Message.
// Uploaded files are introduced to the model by their absolute path, never by
// their bytes; images keep flowing to the provider as usual.
func (r *BotServiceImpl) buildUserMessage(msg *channel.IncomingMessage, uploads []provider.Upload) provider.Message {
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

	if len(uploads) > 0 {
		var builder strings.Builder
		builder.WriteString(provider.AttachmentBlockOpen)
		builder.WriteString("\n")
		builder.WriteString("[read them with the filesystem or bash tools]\n")
		for _, upload := range uploads {
			builder.WriteString(upload.Path)
			builder.WriteString("\n")
		}
		builder.WriteString(provider.AttachmentBlockClose)
		if text != "" {
			builder.WriteString("\n\n")
			builder.WriteString(text)
		}
		text = builder.String()
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
			contentItems := make([]provider.ContentItem, 0, len(msg.Images)+1)
			if text != "" {
				contentItems = append(contentItems, provider.ContentItem{Type: "text", Text: text})
			}
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

// conversationCheckpointInterval throttles transcript checkpoints so a long
// turn does not rewrite the whole conversation document on every iteration.
const conversationCheckpointInterval = 2 * time.Second

func (r *BotServiceImpl) saveConversation(user *model.User, msg *channel.IncomingMessage, transcript []provider.Message) error {
	messages := stripTranscript(transcript)
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

	if err := r.conversationRepo.UpdateConversationById(conv.Id, messages, ""); err != nil {
		return err
	}

	// Title generation is a second LLM call. Keep it off the response path so a
	// slow provider cannot make the chat appear stuck after the answer is ready.
	if conv.Title == "" || conv.Title == "New Chat" {
		messagesCopy := make([]provider.Message, len(messages))
		copy(messagesCopy, messages)
		go r.updateConversationTitle(user, conv.Id, messagesCopy)
	}
	return nil
}

// checkpointConversation persists the in-progress transcript. It is
// best-effort: a failed checkpoint must never break an otherwise healthy turn.
func (r *BotServiceImpl) checkpointConversation(msg *channel.IncomingMessage, transcript []provider.Message) {
	if msg.ConversationID == "" || msg.ConversationID == "NEW" {
		return
	}
	messages := stripTranscript(transcript)
	if len(messages) == 0 {
		return
	}
	if err := r.conversationRepo.UpdateConversationById(msg.ConversationID, messages, ""); err != nil {
		log.Printf("Failed to checkpoint conversation %s: %v", msg.ConversationID, err)
	}
}

// stripTranscript drops the leading system prompt and normalizes the version of
// the messages that are safe to persist.
func stripTranscript(transcript []provider.Message) []provider.Message {
	start := 0
	for start < len(transcript) && transcript[start].Role == "system" {
		start++
	}
	if start >= len(transcript) {
		return nil
	}
	messages := make([]provider.Message, len(transcript)-start)
	copy(messages, transcript[start:])
	for i := range messages {
		messages[i].Version = 2
	}
	return messages
}

func (r *BotServiceImpl) updateConversationTitle(user *model.User, conversationID string, messages []provider.Message) {
	title, err := r.GenerateConversationTitle(user, messages)
	if err != nil {
		log.Printf("Failed to generate conversation title: %v", err)
		return
	}
	if title == "" || title == "New Chat" {
		return
	}
	if err := r.conversationRepo.UpdateConversationTitle(conversationID, title); err != nil {
		log.Printf("Failed to save conversation title: %v", err)
	}
}

func (r *BotServiceImpl) GenerateConversationTitle(user *model.User, messages []provider.Message) (string, error) {
	defaultTitle := "New Chat"
	var firstUserMsg string
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		// Titles describe the conversation, not the uploaded file paths.
		if text := provider.StripAttachmentBlock(provider.MessageText(msg)); text != "" {
			firstUserMsg = text
			break
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
