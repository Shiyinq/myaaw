package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"myaaw/internal/channel"
	"myaaw/internal/services/queue/repository"
	"myaaw/internal/utils"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type DiscordMeta struct {
	ChannelID string
	MessageID string
	// UserID is the DM recipient. When ChannelID is empty and IsDM is set, Send
	// opens (or reuses) the DM channel for this user.
	UserID string
	IsDM   bool
}

type DiscordAdapter struct {
	session     *discordgo.Session
	transport   discordTransport
	dmResolver  dmAPI
	handlerOnce sync.Once

	dmMu       sync.Mutex
	dmChannels map[string]string
}

func NewDiscordAdapter(token string) (*DiscordAdapter, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %w", err)
	}
	// Guild channel messages carry no content unless the privileged Message
	// Content intent is requested; the Developer Portal toggle alone is not
	// enough. DMs work without it, which is why a bot can look half-broken.
	session.Identify.Intents |= discordgo.IntentMessageContent
	return &DiscordAdapter{
		session:    session,
		transport:  newRetryingTransport(discordAPI{session: session}),
		dmChannels: map[string]string{},
	}, nil
}

func (d *DiscordAdapter) transportOrAPI() discordTransport {
	if d.transport != nil {
		return d.transport
	}
	if d.session != nil {
		return newRetryingTransport(discordAPI{session: d.session})
	}
	return nil
}

func (d *DiscordAdapter) Name() string {
	return "discord"
}

// StartListener connects the gateway, retrying with backoff while the network
// is unavailable, then blocks until the context is cancelled so the session can
// be closed cleanly.
func (d *DiscordAdapter) StartListener(ctx context.Context, queueRepo repository.QueueRepository) error {
	d.handlerOnce.Do(func() { d.registerHandler(queueRepo) })

	if err := openGateway(ctx, func() error { return d.session.Open() }, sleepContext); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		return err
	}

	log.Println("Discord listener started")

	<-ctx.Done()
	if err := d.session.Close(); err != nil {
		return fmt.Errorf("close Discord session: %w", err)
	}
	return nil
}

func (d *DiscordAdapter) registerHandler(queueRepo repository.QueueRepository) {
	d.session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		protect("message handler", func() {
			handleDiscordMessage(s, m, queueRepo)
		})
	})
	d.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		protect("approval interaction", func() {
			handleApprovalInteraction(s, i)
		})
	})
}

func handleDiscordMessage(s *discordgo.Session, m *discordgo.MessageCreate, queueRepo repository.QueueRepository) {
	// Ignore invalid messages or messages from bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	images := []string{}
	for _, attachment := range m.Attachments {
		if attachment.ContentType != "" && utils.IsImage(attachment.ContentType) {
			base64Img, err := utils.DownloadFileToBase64(attachment.URL)
			if err != nil {
				log.Printf("Failed to download discord image: %v", err)
				continue
			}
			images = append(images, base64Img)
		}
	}

	// Name lookups are best-effort: a thread may not be cached by the state.
	var (
		channelInfo *discordgo.Channel
		guildInfo   *discordgo.Guild
	)
	if cached, err := s.State.Channel(m.ChannelID); err == nil {
		channelInfo = cached
	}
	if m.GuildID != "" {
		if cached, err := s.State.Guild(m.GuildID); err == nil {
			guildInfo = cached
		}
	}

	rawPayload := buildMessagePayload(m, channelInfo, guildInfo, images)
	envelope := &channel.QueueEnvelope{
		Channel: "discord",
		Payload: rawPayload,
	}

	envelopeBytes, _ := json.Marshal(envelope)
	if err := queueRepo.PublishMessage(envelopeBytes); err != nil {
		log.Printf("Failed to publish Discord message to queue: %v", err)
	} else {
		log.Printf("Published Discord message from %s to queue", m.Author.Username)
	}
}

// buildMessagePayload wraps a Discord message in the queue payload that
// ParseIncoming reads back. Guild/channel names are best-effort: the gateway
// state may not have a thread cached.
func buildMessagePayload(m *discordgo.MessageCreate, channelInfo *discordgo.Channel, guildInfo *discordgo.Guild, images []string) []byte {
	chatType := "guild"
	if m.GuildID == "" {
		chatType = "dm"
	}

	guildID := m.GuildID
	channelName := ""
	isThread := false
	if channelInfo != nil {
		if chatType != "dm" {
			channelName = channelInfo.Name
		}
		if guildID == "" {
			guildID = channelInfo.GuildID
		}
		isThread = channelInfo.IsThread()
	}
	guildName := ""
	if guildInfo != nil {
		guildName = guildInfo.Name
	}

	displayName := strings.TrimSpace(m.Author.GlobalName)
	if displayName == "" {
		displayName = strings.TrimSpace(m.Author.Username)
	}

	userID, _ := strconv.Atoi(m.Author.ID)
	payload := map[string]interface{}{
		"message": map[string]interface{}{
			"from": map[string]interface{}{
				"id":         userID,
				"first_name": displayName,
				"username":   m.Author.Username,
			},
			"chat": map[string]interface{}{
				"id":        m.ChannelID,
				"type":      chatType,
				"name":      channelName,
				"is_thread": isThread,
			},
			"guild": map[string]interface{}{
				"id":   guildID,
				"name": guildName,
			},
			"message_id": m.ID,
			"text":       m.Content,
			"images":     images,
		},
		"original_source": "discord",
		"discord_author": map[string]interface{}{
			"id":          m.Author.ID,
			"username":    m.Author.Username,
			"global_name": m.Author.GlobalName,
		},
	}

	raw, _ := json.Marshal(payload)
	return raw
}

func (d *DiscordAdapter) ParseIncoming(payload json.RawMessage) (*channel.IncomingMessage, error) {
	// We construct a custom map structure in the listener, so we parse it back here.
	var data struct {
		Message struct {
			Text      string   `json:"text"`
			Images    []string `json:"images"`
			MessageID string   `json:"message_id"`
			From      struct {
				FirstName string `json:"first_name"`
				Username  string `json:"username"`
			} `json:"from"`
			Chat struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Name     string `json:"name"`
				IsThread bool   `json:"is_thread"`
			} `json:"chat"`
			Guild struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"guild"`
		} `json:"message"`
		DiscordAuthor struct {
			ID         string `json:"id"`
			Username   string `json:"username"`
			GlobalName string `json:"global_name"`
		} `json:"discord_author"`
	}

	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("failed to parse discord payload: %w", err)
	}

	userID, _ := strconv.Atoi(data.DiscordAuthor.ID)

	senderName := strings.TrimSpace(data.DiscordAuthor.GlobalName)
	if senderName == "" {
		senderName = strings.TrimSpace(data.Message.From.FirstName)
	}
	if senderName == "" {
		senderName = strings.TrimSpace(data.DiscordAuthor.Username)
	}

	return &channel.IncomingMessage{
		UserID:         userID,
		SenderID:       strings.TrimSpace(data.DiscordAuthor.ID),
		SenderName:     senderName,
		SenderUsername: strings.TrimSpace(data.DiscordAuthor.Username),
		Text:           data.Message.Text,
		Images:         data.Message.Images,
		Channel:        "discord",
		ChatType:       strings.TrimSpace(data.Message.Chat.Type),
		IsThread:       data.Message.Chat.IsThread,
		ChatID:         strings.TrimSpace(data.Message.Chat.ID),
		Scope:          discordScope(data.Message.Chat.Type, data.Message.Chat.ID),
		ChatName:       strings.TrimSpace(data.Message.Guild.Name),
		TopicName:      strings.TrimSpace(data.Message.Chat.Name),
		RawMeta: DiscordMeta{
			ChannelID: data.Message.Chat.ID,
			MessageID: data.Message.MessageID,
		},
	}, nil
}

func (d *DiscordAdapter) Send(msg *channel.IncomingMessage, out *channel.OutgoingMessage) error {
	meta := msg.RawMeta.(DiscordMeta)
	channelID, err := d.resolveChannel(meta)
	if err != nil {
		return err
	}
	items := buildDiscordTimeline(out.Trace, out.Text, out.Thought)
	var sendErr error
	if len(items) > 0 {
		timeline := &discordTimeline{
			transport: d.transportOrAPI(),
			channelID: channelID,
			replyTo:   meta.MessageID,
			tools:     map[string]*discordTimelineItem{},
			now:       time.Now,
		}

		for i, item := range items {
			if item.Kind == "text" {
				timeline.writeText(item, item.Text, i == len(items)-1)
				continue
			}
			item.MessageID = timeline.sendRaw(timeline.render(item))
		}
		sendErr = timeline.err
	}

	if err := d.sendAttachments(channelID, out.Attachments); err != nil && sendErr == nil {
		sendErr = err
	}
	return sendErr
}

// resolveChannel returns the channel to send to, opening a DM channel for a
// user-targeted heartbeat or cron job when needed.
func (d *DiscordAdapter) resolveChannel(meta DiscordMeta) (string, error) {
	if meta.ChannelID != "" {
		return meta.ChannelID, nil
	}
	if !meta.IsDM || meta.UserID == "" {
		return "", fmt.Errorf("discord: no target channel")
	}
	return d.dmChannel(meta.UserID)
}

// dmAPI is the part of the Discord REST API needed to resolve a DM channel. It
// is an interface so the fallback can be tested without a gateway connection.
type dmAPI interface {
	UserChannelCreate(recipientID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	Channel(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
}

func (d *DiscordAdapter) dmAPIOrNil() dmAPI {
	if d.dmResolver != nil {
		return d.dmResolver
	}
	if d.session == nil {
		return nil
	}
	return d.session
}

// dmChannel opens (or reuses) the direct-message channel for a user id. The
// target may also be a DM channel id: a model reading the chat context can copy
// ChatID instead of the speaker's UserID, so a confirmed DM channel is accepted
// instead of failing with "Invalid Recipient(s)".
func (d *DiscordAdapter) dmChannel(target string) (string, error) {
	api := d.dmAPIOrNil()
	if api == nil {
		return "", fmt.Errorf("discord session is not available")
	}
	d.dmMu.Lock()
	defer d.dmMu.Unlock()
	if d.dmChannels == nil {
		d.dmChannels = map[string]string{}
	}
	if id, ok := d.dmChannels[target]; ok {
		return id, nil
	}
	channel, err := api.UserChannelCreate(target)
	if err != nil {
		if existing, lookupErr := api.Channel(target); lookupErr == nil && existing != nil && existing.Type == discordgo.ChannelTypeDM {
			d.dmChannels[target] = existing.ID
			return existing.ID, nil
		}
		return "", fmt.Errorf("open discord dm: %w", err)
	}
	d.dmChannels[target] = channel.ID
	return channel.ID, nil
}

func (d *DiscordAdapter) sendAttachments(channelID string, attachments []channel.Attachment) error {
	if len(attachments) == 0 {
		return nil
	}
	if d.session == nil {
		return fmt.Errorf("discord session is not available")
	}
	for _, att := range attachments {
		if len(att.Data) == 0 {
			continue
		}
		name := strings.TrimSpace(att.FileName)
		if name == "" {
			name = "attachment.png"
		}
		mimeType := att.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		_, err := d.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Files: []*discordgo.File{{
				Name:        name,
				ContentType: mimeType,
				Reader:      bytes.NewReader(att.Data),
			}},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DiscordAdapter) SendStream(msg *channel.IncomingMessage, streamFn func(onChunk func(chunk channel.StreamChunk)) error) (*channel.OutgoingMessage, error) {
	meta := msg.RawMeta.(DiscordMeta)
	transport := d.transportOrAPI()
	if transport == nil {
		return nil, fmt.Errorf("discord transport is not configured")
	}
	channelID, err := d.resolveChannel(meta)
	if err != nil {
		return nil, err
	}

	initialID, err := transport.SendReply(channelID, meta.MessageID, "✨ Typing...")
	if err != nil {
		return nil, err
	}

	timeline := newDiscordTimeline(transport, channelID, meta.MessageID, initialID)
	err = streamFn(timeline.handleChunk)
	timeline.finish()
	if err != nil {
		return nil, err
	}
	if err := d.sendAttachments(channelID, timeline.attachments); err != nil {
		return nil, err
	}

	return &channel.OutgoingMessage{
		Text:        timeline.finalText,
		Trace:       timeline.finalTrace,
		Usage:       timeline.finalUsage,
		Attachments: timeline.attachments,
	}, nil
}

func (d *DiscordAdapter) SendError(msg *channel.IncomingMessage, errText string) error {
	meta, ok := msg.RawMeta.(DiscordMeta)
	if !ok {
		return fmt.Errorf("discord: unexpected message metadata %T", msg.RawMeta)
	}
	transport := d.transportOrAPI()
	if transport == nil {
		return fmt.Errorf("discord transport is not configured")
	}
	channelID, err := d.resolveChannel(meta)
	if err != nil {
		return err
	}
	_, err = transport.Send(channelID, "❌ "+errText)
	return err
}

// protect keeps a panic in one gateway handler from taking down the process.
func protect(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("discord: %s panicked: %v", name, r)
		}
	}()
	fn()
}

const (
	discordInitialBackoff = time.Second
	discordMaxBackoff     = 60 * time.Second
)

// openGateway retries the gateway connection with exponential backoff. Errors
// that can never succeed (a bad token) stop the loop instead of spamming it.
func openGateway(ctx context.Context, open func() error, sleep func(context.Context, time.Duration) error) error {
	backoff := discordInitialBackoff
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := open()
		if err == nil {
			return nil
		}
		if isDiscordAuthError(err) {
			return fmt.Errorf("discord authentication failed, check the bot token: %w", err)
		}

		log.Printf("discord: gateway connection failed: %v; retrying in %s", err, backoff)
		if sleepErr := sleep(ctx, backoff); sleepErr != nil {
			return sleepErr
		}

		backoff *= 2
		if backoff > discordMaxBackoff {
			backoff = discordMaxBackoff
		}
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// isDiscordAuthError detects the close codes Discord uses for a rejected token.
func isDiscordAuthError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "4004") ||
		strings.Contains(msg, "4014") ||
		strings.Contains(msg, "authentication failed") ||
		strings.Contains(msg, "invalid token")
}
