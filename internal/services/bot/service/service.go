package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"myaaw/internal/agent"
	"myaaw/internal/channel"
	"myaaw/internal/channel/discord"
	"myaaw/internal/channel/telegram"
	"myaaw/internal/config"
	"myaaw/internal/obs"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
	"myaaw/internal/services/bot/repository"
	"slices"
	"strconv"
	"strings"
	"sync"
)

type BotService interface {
	IsAllowed(userID int) bool
	checkUser(msg *channel.IncomingMessage) (*model.User, error)
	Bot(msg *channel.IncomingMessage) (*channel.OutgoingMessage, error)
	BotContext(ctx context.Context, msg *channel.IncomingMessage) (*channel.OutgoingMessage, error)
	BotStream(msg *channel.IncomingMessage, onChunk func(channel.StreamChunk)) (*channel.OutgoingMessage, error)
	BotStreamContext(ctx context.Context, msg *channel.IncomingMessage, onChunk func(channel.StreamChunk)) (*channel.OutgoingMessage, error)
	command(user *model.User, msg *channel.IncomingMessage) (bool, string, error)
	conversation(user *model.User, msg *channel.IncomingMessage) (*channel.OutgoingMessage, error)
	ProcessHeartbeat(prompt string, target HeartbeatTarget, triggerType string) (*channel.IncomingMessage, *channel.OutgoingMessage, error)
}

// HeartbeatTarget is where a scheduled turn should run and be delivered.
type HeartbeatTarget struct {
	Channel   string
	To        string
	ThreadID  string
	SessionID string
	ChatType  string
}

type BotServiceImpl struct {
	userRepo              repository.UserRepository
	conversationRepo      repository.ConversationRepository
	llmProvider           provider.LLMProvider
	transcriber           provider.Transcriber
	agent                 agent.AgentProvider
	currentProviderID     string
	currentProviderConfig config.ProviderConfig
	providerErr           error

	// locks serializes turns that share a conversation. A live chat and a cron
	// job must not read the same transcript and write it back concurrently.
	locks sync.Map

	// Trace recording. The store is opened lazily on the first run so commands
	// and tests that never execute the agent do not create a database.
	obsOnce     sync.Once
	obsStore    *obs.Store
	obsExporter *obs.Exporter
	obsPricer   *obs.Pricer
	obsConfig   obs.Config
}

func selectedProviderConfig() config.ProviderConfig {
	return config.ProviderConfig{
		Type: config.LLMProviderName, BaseURL: config.LLMProviderBaseURL,
		APIKey: config.LLMProviderAPIKey, DefaultModel: config.LLMDefaultModel,
		AuthMode: config.LLMProviderAuthMode, CredentialID: config.LLMProviderCredentialID,
	}
}

func NewBotService(userRepo repository.UserRepository, conversationRepo repository.ConversationRepository) BotService {
	selected := selectedProviderConfig()
	llmProvider, providerErr := provider.CreateLLMProviderFromConfig(selected)
	if providerErr != nil {
		llmProvider = nil
		// Not fatal: the supervisor would otherwise crash-loop the agent when a
		// provider key is missing or a provider name is mistyped. Requests fail
		// with a clear message instead, and a config fix is picked up by
		// checkAndReloadProvider on the next message.
		log.Printf("⚠️  Failed to create LLM provider %s: %v. Chat requests will fail until the provider is fixed.", config.LLMProviderName, providerErr)
	}

	// A transcriber is optional: without one, voice notes are skipped instead of
	// crashing the agent. Only warn when a provider is configured but unusable.
	var transcriber provider.Transcriber
	if name := strings.TrimSpace(config.TranscriberProviderName); name != "" {
		created, err := provider.CreateTranscriber(name, config.TranscriberAPIKey)
		if err != nil {
			log.Printf("Warning: Error creating Transcriber provider %s: %v. Transcriber functionality might be affected or disabled depending on message handling logic.", name, err)
		} else {
			transcriber = created
		}
	}

	svc := &BotServiceImpl{
		userRepo:              userRepo,
		conversationRepo:      conversationRepo,
		llmProvider:           llmProvider,
		transcriber:           transcriber,
		currentProviderID:     config.CurrentProviderID,
		currentProviderConfig: selected,
		providerErr:           providerErr,
	}
	if llmProvider != nil {
		svc.agent = agent.NewAgent(llmProvider)
	}
	return svc
}

// requireProvider reports whether chat requests can reach an LLM.
func (r *BotServiceImpl) requireProvider() error {
	if r.llmProvider == nil || r.agent == nil {
		if r.providerErr != nil {
			return fmt.Errorf("LLM provider is not configured: %w", r.providerErr)
		}
		return errors.New("LLM provider is not configured: check the provider type and API key or signed-in credential in config.json")
	}
	return nil
}

func (r *BotServiceImpl) checkAndReloadProvider() {
	selected := selectedProviderConfig()
	if r.currentProviderID == config.CurrentProviderID && r.currentProviderConfig == selected {
		return
	}

	// Never keep the previous account/client when the selected configuration is
	// invalid (notably a disconnected OAuth reference). A later reconnect is
	// detected even if the provider ID and model have not changed.
	r.currentProviderID = config.CurrentProviderID
	r.currentProviderConfig = selected
	r.llmProvider, r.providerErr = provider.CreateLLMProviderFromConfig(selected)
	r.agent = nil
	if r.providerErr != nil {
		r.llmProvider = nil
		log.Printf("Failed to hot-reload provider %s: %v", config.CurrentProviderID, r.providerErr)
		return
	}
	r.agent = agent.NewAgent(r.llmProvider)
	log.Printf("Successfully hot-reloaded LLM Provider to: %s (Model: %s)", config.CurrentProviderID, config.LLMDefaultModel)
}

func (r *BotServiceImpl) IsAllowed(userID int) bool {
	if config.BotType != "private" {
		return true
	}
	return slices.Contains(config.OwnerIDs, strconv.Itoa(userID))
}

func (r *BotServiceImpl) checkUser(msg *channel.IncomingMessage) (*model.User, error) {
	var user *model.User
	var err error
	user, err = r.userRepo.GetUserById(msg.UserID)
	if err != nil {
		return nil, err
	}

	if user == nil {
		name := fmt.Sprintf("user-%d", msg.UserID)
		if sender := strings.TrimSpace(msg.SenderName); sender != "" {
			name = sender
		}
		newUser := model.User{
			UserId:   msg.UserID,
			Name:     name,
			Provider: r.llmProvider.ProviderName(),
			Model:    r.llmProvider.DefaultModel(""),
		}
		user, err = r.userRepo.CreateUser(&newUser)

		if err != nil {
			return nil, err
		}
	}

	return user, nil
}

func (r *BotServiceImpl) changeProviderAndModel(user *model.User) (*model.User, error) {
	systemProvider := r.llmProvider.ProviderName()
	systemModel := r.llmProvider.DefaultModel("")

	log.Printf("Provider mismatch!")
	log.Printf("Automatically updating user %v configurations", user.UserId)

	err := r.userRepo.UpdateModel(user.UserId, systemModel)
	if err != nil {
		return nil, err
	}

	err = r.userRepo.UpdateProvider(user.UserId, systemProvider)
	if err != nil {
		return nil, err
	}

	user, err = r.userRepo.GetUserById(user.UserId)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (r *BotServiceImpl) Bot(msg *channel.IncomingMessage) (*channel.OutgoingMessage, error) {
	return r.BotContext(context.Background(), msg)
}

func (r *BotServiceImpl) BotContext(ctx context.Context, msg *channel.IncomingMessage) (*channel.OutgoingMessage, error) {
	r.checkAndReloadProvider()
	if err := r.requireProvider(); err != nil {
		return nil, err
	}

	user, err := r.checkUser(msg)
	if err != nil {
		return nil, err
	}

	if user.Provider != r.llmProvider.ProviderName() || user.Model != r.llmProvider.DefaultModel("") {
		user, err = r.changeProviderAndModel(user)
		if err != nil {
			return nil, err
		}
	}

	isCommand, response, err := r.command(user, msg)
	if err != nil {
		return nil, err
	}

	if isCommand {
		return &channel.OutgoingMessage{Text: response}, nil
	}

	return r.conversationContext(ctx, user, msg)
}

func (r *BotServiceImpl) BotStream(msg *channel.IncomingMessage, onChunk func(channel.StreamChunk)) (*channel.OutgoingMessage, error) {
	return r.BotStreamContext(context.Background(), msg, onChunk)
}

func (r *BotServiceImpl) BotStreamContext(ctx context.Context, msg *channel.IncomingMessage, onChunk func(channel.StreamChunk)) (*channel.OutgoingMessage, error) {
	r.checkAndReloadProvider()
	if err := r.requireProvider(); err != nil {
		return nil, err
	}

	user, err := r.checkUser(msg)
	if err != nil {
		return nil, err
	}

	if user.Provider != r.llmProvider.ProviderName() || user.Model != r.llmProvider.DefaultModel("") {
		user, err = r.changeProviderAndModel(user)
		if err != nil {
			return nil, err
		}
	}

	isCommand, response, err := r.command(user, msg)
	if err != nil {
		return nil, err
	}

	if isCommand {
		out := &channel.OutgoingMessage{Text: response}
		if onChunk != nil {
			onChunk(channel.StreamChunk{Kind: "text", Text: response})
		}
		return out, nil
	}

	return r.conversationStreamContext(ctx, user, msg, onChunk)
}

func (r *BotServiceImpl) ProcessHeartbeat(prompt string, target HeartbeatTarget, triggerType string) (*channel.IncomingMessage, *channel.OutgoingMessage, error) {
	r.checkAndReloadProvider()
	if err := r.requireProvider(); err != nil {
		return nil, nil, err
	}

	msg, err := r.heartbeatMessage(prompt, target, triggerType)
	if err != nil {
		return nil, nil, err
	}

	user, err := r.checkUser(msg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get/create user for heartbeat: %w", err)
	}

	out, err := r.conversation(user, msg)
	if err != nil {
		log.Printf("Heartbeat conversation error: %v", err)
		return msg, nil, err
	}

	log.Println("Heartbeat conversation processed successfully.")
	return msg, out, nil
}

// heartbeatMessage builds the synthetic inbound message for a scheduled target.
// It is the single place where a job's scope, room ids, owner, and display names
// are resolved, so a scheduled turn looks exactly like a live message.
func (r *BotServiceImpl) heartbeatMessage(prompt string, target HeartbeatTarget, triggerType string) (*channel.IncomingMessage, error) {
	target.Channel = strings.ToLower(strings.TrimSpace(target.Channel))
	target.To = strings.TrimSpace(target.To)
	target.ThreadID = strings.TrimSpace(target.ThreadID)
	target.SessionID = strings.TrimSpace(target.SessionID)
	target.ChatType = strings.ToLower(strings.TrimSpace(target.ChatType))

	if target.Channel == "" {
		return nil, fmt.Errorf("heartbeat target is missing a channel")
	}
	if target.To == "" && target.SessionID == "" {
		return nil, fmt.Errorf("heartbeat target is missing an id")
	}
	if (target.Channel == "api" || target.Channel == "cli") && target.SessionID == "" {
		// A web or terminal chat has no platform room id, so without a pinned
		// session the turn would land in whichever conversation is active.
		return nil, fmt.Errorf("heartbeat target %s requires a session id", target.Channel)
	}

	msg := &channel.IncomingMessage{
		Text:        prompt,
		Channel:     target.Channel,
		TriggerType: triggerType,
		ChatType:    target.ChatType,
	}

	// A pinned session supplies its own owner and scope, and bypasses the
	// per-user lookup. This is how web/terminal reminders reach the exact chat.
	pinned := target.SessionID != ""
	if pinned {
		conv, err := r.conversationRepo.GetConversationById(target.SessionID)
		if err != nil {
			return nil, fmt.Errorf("load heartbeat session: %w", err)
		}
		if conv == nil {
			return nil, fmt.Errorf("heartbeat session %s not found", target.SessionID)
		}
		msg.ConversationID = conv.Id
		msg.Scope = conv.Scope
		msg.UserID = conv.UserId
	}

	if !pinned {
		msg.Scope = heartbeatScope(target)
	}

	// Platform identity: the prompt sees the same room fields as a live message.
	switch target.Channel {
	case "telegram":
		chatID, err := strconv.Atoi(target.To)
		if err != nil {
			return nil, fmt.Errorf("invalid telegram chat id %q", target.To)
		}
		// The chat id sign is the room kind: positive is a user (private), a
		// negative id is a group. A chat_type inherited from the chat that
		// created the job (for example a DM aimed at a group) must not win.
		if chatID > 0 {
			msg.ChatType = "private"
		} else {
			msg.ChatType = "supergroup"
		}
		threadID := 0
		if target.ThreadID != "" {
			threadID, err = strconv.Atoi(target.ThreadID)
			if err != nil {
				return nil, fmt.Errorf("invalid telegram thread id %q", target.ThreadID)
			}
		}
		msg.IsThread = threadID >= 2
		msg.ChatID = target.To
		msg.TopicID = target.ThreadID
		msg.RawMeta = telegram.TelegramMeta{ChatID: chatID, ThreadID: threadID}
	case "discord":
		switch {
		case strings.HasPrefix(msg.Scope, "dc:"):
			// A pinned guild conversation has a dc: scope even when the job
			// inherited chat_type from a DM it was created in. Guild rooms always
			// carry that scope, so a pinned conversation without one is a DM.
			msg.ChatType = "guild"
		case pinned:
			msg.ChatType = "dm"
		case msg.ChatType == "":
			msg.ChatType = "guild"
		}
		if msg.ChatType == "dm" {
			// The target is a user id, not a channel id; Send opens the DM channel.
			msg.RawMeta = discord.DiscordMeta{UserID: target.To, IsDM: true}
		} else {
			msg.ChatID = target.To
			msg.RawMeta = discord.DiscordMeta{ChannelID: target.To}
		}
	case "api", "cli":
		// No outbound adapter: the turn is saved in the pinned session and the
		// dashboard or terminal shows it on the next read.
	default:
		return nil, fmt.Errorf("unsupported heartbeat channel %q", target.Channel)
	}

	// A pinned session keeps its owner; a DM belongs to the target user; a
	// shared chat uses the configured owner so no fake users are created.
	if !pinned && msg.UserID == 0 {
		if msg.Scope == "" {
			if id, err := strconv.Atoi(target.To); err == nil {
				msg.UserID = id
			}
		}
		if msg.UserID == 0 {
			msg.UserID = ownerUserID()
		}
	}

	// Names make the dashboard badge and <userinfo> match a live message.
	if msg.Scope != "" {
		if chatName, topicName, isForum, err := r.conversationRepo.GetScopeNames(msg.Scope); err == nil {
			msg.ChatName = chatName
			msg.TopicName = topicName
			msg.IsForum = isForum
		}
	}

	return msg, nil
}

// heartbeatScope derives the conversation scope from a scheduled target, using
// the same rules as an inbound message.
func heartbeatScope(target HeartbeatTarget) string {
	switch target.Channel {
	case "telegram":
		chatID, err := strconv.Atoi(target.To)
		if err != nil || chatID > 0 {
			// Direct messages are per-user and keep the legacy empty scope.
			return ""
		}
		threadID := 0
		if target.ThreadID != "" {
			threadID, _ = strconv.Atoi(target.ThreadID)
		}
		if threadID <= 1 {
			return "tg:" + target.To
		}
		return fmt.Sprintf("tg:%s:%d", target.To, threadID)
	case "discord":
		if target.ChatType == "dm" || target.To == "" {
			return ""
		}
		return "dc:" + target.To
	}
	return ""
}

// ownerUserID is the conversation owner for shared targets, so scheduled turns
// never create a user whose id is a chat or channel id.
func ownerUserID() int {
	for _, id := range config.OwnerIDs {
		if n, err := strconv.Atoi(strings.TrimSpace(id)); err == nil && n != 0 {
			return n
		}
	}
	return 0
}
