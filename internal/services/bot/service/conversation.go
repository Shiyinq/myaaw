package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/agent/skills"
	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/obs/agenttrace"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/events"
	"myaaw/internal/services/bot/model"
	"strconv"

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

// lockConversation serializes turns that share a conversation, or the same chat
// target while the conversation id is not known yet. A live chat and a cron job
// must not read the same transcript and write it back concurrently.
func (r *BotServiceImpl) lockConversation(msg *channel.IncomingMessage) func() {
	value, _ := r.locks.LoadOrStore(conversationLockKey(msg), &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

// conversationLockKey prefers the conversation id and falls back to the chat
// target, matching how a heartbeat chooses its delivery target.
func conversationLockKey(msg *channel.IncomingMessage) string {
	if id := strings.TrimSpace(msg.ConversationID); id != "" {
		return "conversation:" + id
	}
	channel := strings.ToLower(strings.TrimSpace(msg.Channel))
	return "target:" + channel + ":" + msg.ChatID + ":" + msg.TopicID
}

// publishTurnEvent tells SSE clients that a triggered turn was persisted.
func (r *BotServiceImpl) publishTurnEvent(msg *channel.IncomingMessage) {
	if msg.TriggerType == "" || msg.ConversationID == "" {
		return
	}
	events.Default().Publish(events.TurnEvent{
		ConversationID: msg.ConversationID,
		Trigger:        msg.TriggerType,
		// Nanoseconds matter: clients acknowledge the read watermark with this
		// timestamp, and a second-truncated value can sit just behind the turn's
		// stored updated_at, which would make the chat unread again.
		At: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (r *BotServiceImpl) runConversation(
	ctx context.Context,
	user *model.User,
	msg *channel.IncomingMessage,
	onChunk func(channel.StreamChunk),
) (*channel.OutgoingMessage, error) {
	unlock := r.lockConversation(msg)
	defer unlock()

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

	// Tools that schedule work (cron) or reference the current chat need the
	// session identity. The system prompt is static now, so it cannot be parsed
	// for this any more; carry it explicitly.
	ctxTools := agent.ToolsContextFor(msg)
	cfg.ToolsContext = &ctxTools

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

	// lastContext remembers the breakdown of the most recent request so the
	// provider's exact usage can update its total without recomputing it.
	var lastContext channel.ContextUsage
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
		case agent.EventLLMRequestStart:
			lastContext = event.Context
			contextUsage := lastContext
			onChunk(channel.StreamChunk{Kind: "context", Context: &contextUsage})
		case agent.EventUsage:
			if event.Usage.TotalTokens > 0 {
				onChunk(channel.StreamChunk{Kind: "usage", Usage: event.Usage})
			}
			// The provider's usage block carries the exact input count; replace the
			// pre-request estimate so the ring shows the real number.
			input := event.Usage.PromptTokens
			if input == 0 && event.Usage.TotalTokens > 0 {
				input = event.Usage.TotalTokens - event.Usage.CompletionTokens
			}
			if input > 0 {
				lastContext.Tokens = input
				contextUsage := lastContext
				onChunk(channel.StreamChunk{Kind: "context", Context: &contextUsage})
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

	// Tool attachments ride an early chunk so a live surface can show them while
	// the run continues. Delivery directives are only resolved after the final
	// text is known, so they get their own chunk below.
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

	// Turn explicit @attach(...) directives into attachments for this surface.
	// The cleaned text is what the chat shows and what the transcript keeps.
	deliveredText, deliveredAttachments := r.deliverAttachments(msg, finalText)
	if len(deliveredAttachments) > 0 {
		result.Attachments = append(result.Attachments, deliveredAttachments...)
		attachToFinalAssistant(result.Transcript, deliveredAttachments)
		// Streaming surfaces such as Telegram send the photos they collect from
		// chunks, not from the returned message, so the delivered files need a
		// chunk of their own.
		if onChunk != nil {
			onChunk(channel.StreamChunk{Kind: "attachment", Attachments: deliveredAttachments})
		}
	}
	if deliveredText != finalText {
		replaceFinalAssistantText(result.Transcript, deliveredText)
		finalText = deliveredText
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
		stampTurnDuration(result.Transcript, result.StartedAt, result.EndedAt)
		if err := r.saveConversation(user, msg, result.Transcript); err != nil {
			return nil, err
		}
		r.publishTurnEvent(msg)
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

// conversationAccessible reports whether a message may continue a conversation.
// Scoped conversations (for example a Telegram topic) are shared by everyone in
// that scope; unscoped conversations stay private to their owner.
func conversationAccessible(conv *model.Conversation, user *model.User, msg *channel.IncomingMessage) bool {
	if conv == nil {
		return false
	}
	if msg.Scope != "" && conv.Scope != "" {
		return conv.Scope == msg.Scope
	}
	return conv.UserId == user.UserId
}

func (r *BotServiceImpl) buildConversationMessages(user *model.User, msg *channel.IncomingMessage) ([]provider.Message, error) {
	userSystem := agent.NewSystemPromptBuilder(msg.Channel).Build()
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
			created, err := r.conversationRepo.CreateConversationWithId(user.UserId, msg.ConversationID, msg.Channel, "")
			if err != nil {
				return nil, err
			}
			conv = created
		case found == nil:
			return nil, ErrConversationNotFound
		case !conversationAccessible(found, user, msg):
			return nil, ErrConversationForbidden
		default:
			conv = found
		}
	default:
		active, err := r.conversationRepo.GetActiveConversationByScope(user.UserId, msg.Scope)
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
		created, err := r.conversationRepo.CreateConversationInScope(user.UserId, msg.Scope, msg.Channel, "New Chat")
		if err != nil {
			return nil, err
		}
		convMessages = agent.MigrateLegacyTranscript(created.Messages)
		msg.ConversationID = created.Id
	}

	// Keep the display name of the chat target fresh. Best-effort: a failed name
	// write must never break the turn.
	if msg.Scope != "" && (strings.TrimSpace(msg.ChatName) != "" || strings.TrimSpace(msg.TopicName) != "") {
		if err := r.conversationRepo.UpsertScopeNames(msg.Scope, msg.ChatName, msg.TopicName, msg.IsForum); err != nil {
			log.Printf("Failed to store chat scope names for %s: %v", msg.Scope, err)
		}
	}

	// A normal forum message carries only the topic id, never the topic name.
	// Fill the gaps from what the scope learned earlier so <userinfo> can name
	// the topic instead of showing the group only.
	if msg.Scope != "" && (strings.TrimSpace(msg.ChatName) == "" || strings.TrimSpace(msg.TopicName) == "") {
		chatName, topicName, _, err := r.conversationRepo.GetScopeNames(msg.Scope)
		if err != nil {
			log.Printf("Failed to read chat scope names for %s: %v", msg.Scope, err)
		}
		if strings.TrimSpace(msg.ChatName) == "" {
			msg.ChatName = chatName
		}
		if strings.TrimSpace(msg.TopicName) == "" {
			msg.TopicName = topicName
		}
	}

	// An edit or a regeneration rewrites the stored transcript instead of
	// appending a user message: everything after the target turn is dropped.
	if msg.RewindUserOffset != nil {
		rewound, rewindErr := rewindToUserMessage(convMessages, *msg.RewindUserOffset, msg.Text)
		if rewindErr != nil {
			return nil, rewindErr
		}
		// A regeneration sends no text of its own. Record the prompt it replays so
		// the observability run still shows which question the answer was for.
		if strings.TrimSpace(msg.Text) == "" {
			for i := len(rewound) - 1; i >= 0; i-- {
				if rewound[i].Role == "user" {
					msg.Text = provider.StripContextBlocks(provider.MessageText(rewound[i]))
					break
				}
			}
		}
		messages = append(messages, rewound...)
		return messages, nil
	}

	messages = append(messages, convMessages...)

	// A user upload is addressed by its own message id so the frontend can show
	// the files again under the right turn. The conversation id is already final
	// here (existing or freshly created), so the folder is stable before the run.
	messageID := uuid.New().String()
	var uploads []provider.Upload
	if len(msg.Files) > 0 || len(msg.Images) > 0 {
		userDir, rootErr := agentdir.ActiveUserDir()
		if rootErr != nil {
			return nil, rootErr
		}
		if len(msg.Files) > 0 {
			var saveErr error
			uploads, saveErr = saveUserFiles(userDir, msg.ConversationID, messageID, msg.Files)
			if saveErr != nil {
				return nil, saveErr
			}
		}
		// Images still travel inline to the model; the copy on disk lets the person
		// find them in the Files page next to the documents they uploaded.
		if saveErr := saveUserImages(userDir, msg.ConversationID, messageID, msg.Images); saveErr != nil {
			return nil, saveErr
		}
	}

	userMessage := r.buildUserMessage(msg, uploads)
	userMessage.ID = messageID
	userMessage.Uploads = uploads
	created := time.Now().UTC()
	userMessage.CreatedAt = &created
	messages = append(messages, userMessage)
	return messages, nil
}

// stampTurnDuration records how long the turn took on its final assistant
// message, so the transcript can still show "Completed · Ns" after a reload.
func stampTurnDuration(transcript []provider.Message, startedAt, endedAt time.Time) {
	if len(transcript) == 0 || !endedAt.After(startedAt) {
		return
	}
	duration := endedAt.Sub(startedAt).Milliseconds()
	for i := len(transcript) - 1; i >= 0; i-- {
		if transcript[i].Role == "assistant" {
			transcript[i].DurationMs = duration
			return
		}
	}
}

// rewindToUserMessage keeps the transcript up to the user message `offset`
// turns from the end (0 is the last one) and drops everything after it. A
// non-empty replacement rewrites that user message in place and refreshes its
// timestamp, which is how an edited turn is persisted.
func rewindToUserMessage(
	messages []provider.Message,
	offset int,
	replacement string,
) ([]provider.Message, error) {
	if offset < 0 {
		return nil, fmt.Errorf("rewind offset must be zero or greater")
	}

	total := 0
	for _, message := range messages {
		if message.Role == "user" {
			total++
		}
	}
	if offset >= total {
		return nil, fmt.Errorf("rewind target not found")
	}

	target := total - 1 - offset
	seen := -1
	for index, message := range messages {
		if message.Role != "user" {
			continue
		}
		seen++
		if seen != target {
			continue
		}

		kept := agent.CloneMessages(messages[:index+1])
		if strings.TrimSpace(replacement) != "" {
			kept[index].Content = replaceUserContent(kept[index].Content, replacement)
			edited := time.Now().UTC()
			kept[index].CreatedAt = &edited
		}
		return kept, nil
	}

	return nil, fmt.Errorf("rewind target not found")
}

// replaceUserContent rewrites the text a person typed while preserving the
// leading <userinfo>/<attached> blocks and any non-text parts (images). The
// blocks were written once when the message arrived; keeping them byte-identical
// is what lets provider prefix caching reuse the history.
func replaceUserContent(content interface{}, replacement string) interface{} {
	switch value := content.(type) {
	case string:
		blocks, _ := provider.SplitContextBlocks(value)
		return provider.JoinContextBlocks(blocks, replacement)
	case []provider.ContentItem:
		blocks := ""
		sawText := false
		others := make([]provider.ContentItem, 0, len(value)+1)
		for _, item := range value {
			if item.Type == "text" {
				if !sawText {
					blocks, _ = provider.SplitContextBlocks(item.Text)
					sawText = true
				}
				continue
			}
			others = append(others, item)
		}
		text := provider.JoinContextBlocks(blocks, replacement)
		result := make([]provider.ContentItem, 0, len(others)+1)
		if text != "" {
			result = append(result, provider.ContentItem{Type: "text", Text: text})
		}
		return append(result, others...)
	case provider.GeminiPart:
		blocks, _ := provider.SplitContextBlocks(value.Text)
		value.Text = provider.JoinContextBlocks(blocks, replacement)
		return value
	case *provider.GeminiPart:
		if value != nil {
			blocks, _ := provider.SplitContextBlocks(value.Text)
			value.Text = provider.JoinContextBlocks(blocks, replacement)
		}
		return value
	default:
		return replacement
	}
}

// buildUserMessage converts a generic IncomingMessage to a provider.Message.
// Uploaded files are introduced to the model by their absolute path, never by
// their bytes; images keep flowing to the provider as usual.
func (r *BotServiceImpl) buildUserMessage(msg *channel.IncomingMessage, uploads []provider.Upload) provider.Message {
	text := msg.Text
	role := "user"
	// A trigger turn is marked with a block the model reads and people never see;
	// the block carries the kind, the rules, and the content itself.
	if provider.IsTriggerKind(msg.TriggerType) {
		text = provider.TriggerBlock(msg.TriggerType, text)
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

	// The channel context goes in front of everything so the system prompt stays
	// static and provider prefix caching can reuse the whole history.
	if info := agent.RenderUserInfo(msg, time.Now()); info != "" {
		text = provider.JoinContextBlocks(info, text)
	}

	// Sender metadata is display-only and exists only for shared chats (groups,
	// forum topics). CLI, API, and DM messages stay exactly as before.
	message := provider.Message{Role: role, Version: 2}
	if msg.Scope != "" {
		message.SenderID = strings.TrimSpace(msg.SenderID)
		if message.SenderID == "" {
			message.SenderID = strconv.Itoa(msg.UserID)
		}
		message.SenderName = strings.TrimSpace(msg.SenderName)
		message.SenderUsername = strings.TrimSpace(msg.SenderUsername)
	}

	if msg.ReplyTo != "" {
		message.Content = text + "\n\ncontex:\n" + msg.ReplyTo
		return message
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
			message.Content = contentItems
			return message
		}

		message.Content = text
		message.Images = msg.Images
		return message
	}

	message.Content = text
	return message
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
		conv, err = r.conversationRepo.GetActiveConversationByScope(msg.UserID, msg.Scope)
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
		messagesCopy := agent.CloneMessages(messages)
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
	messages := agent.CloneMessages(transcript[start:])
	for i := range messages {
		messages[i].Version = 2
	}
	// Only the final answer of this turn is stamped. Messages loaded from an
	// older transcript have no time, and backfilling them would make every old
	// turn look like it just happened.
	if last := len(messages) - 1; last >= 0 && messages[last].Role == "assistant" && messages[last].CreatedAt == nil {
		now := time.Now().UTC()
		messages[last].CreatedAt = &now
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
		// Titles describe the conversation, not the uploaded file paths or the
		// per-turn channel context.
		if text := provider.StripContextBlocks(provider.MessageText(msg)); text != "" {
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
