package discord

import (
	"encoding/json"
	"fmt"
	"log"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/services/queue/repository"
	"myaaw/internal/utils"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
)

// DiscordMeta holds Discord-specific metadata.
type DiscordMeta struct {
	ChannelID string
	MessageID string
}

// DiscordAdapter implements channel.Adapter for Discord.
type DiscordAdapter struct {
	session *discordgo.Session
}

// NewDiscordAdapter creates a new Discord channel adapter.
func NewDiscordAdapter(token string) (*DiscordAdapter, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %w", err)
	}
	return &DiscordAdapter{
		session: session,
	}, nil
}

func (d *DiscordAdapter) Name() string {
	return "discord"
}

// StartListener connects to Discord Gateway and forwards events to RabbitMQ.
func (d *DiscordAdapter) StartListener(queueRepo repository.QueueRepository) error {
	d.session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore invalid messages or messages from bot itself
		if m.Author.ID == s.State.User.ID {
			return
		}

		// Parse user ID
		userID, _ := strconv.Atoi(m.Author.ID) // Discord IDs are strings (snowflakes), but our internal system uses int.
		// NOTE: Discord IDs are too large for standard int on 32-bit systems, but fit in int64.
		// Our internal UserID is int. If this overflows we might need to map Discord ID to internal ID or change UserID to string.
		// For now assuming we just use the numeric value if it fits, or we might need a better mapping strategy.
		// Actually, Discord IDs are snowflakes (uint64). They WON'T fit in 32-bit int, but might fit in 64-bit int.
		// However, strict conversion here might be an issue if the system expects small integers.
		// Let's use a hash or just the last few digits for now if we want to be safe, BUT ideally we should change UserID to string or int64 globally.
		// Since we can't refactor UserID right now, let's treat it as is (Discord ID as int64 inside int).
		// If int is 64-bit (standard on 64-bit OS), it's fine.

		// TODO: Handle Image/Attachment processing if needed.

		payload := map[string]interface{}{
			"message": map[string]interface{}{
				"from": map[string]interface{}{
					"id": userID, // We pass it as int, hope it fits
					// Better approach: pass string in raw payload, logic below is for Telegram
				},
				"chat": map[string]interface{}{
					"id": m.ChannelID, // ChannelID is string
				},
				"message_id": m.ID,
				"text":       m.Content,
			},
			"original_source": "discord",
			"discord_author": map[string]interface{}{
				"id":       m.Author.ID,
				"username": m.Author.Username,
			},
		}

		rawPayload, _ := json.Marshal(payload)
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
	})

	err := d.session.Open()
	if err != nil {
		return fmt.Errorf("error opening connection: %w", err)
	}

	log.Println("Discord Listener started. Press CTRL-C to exit.")
	return nil
}

// ParseIncoming converts our custom Discord payload from Queue into generic IncomingMessage.
func (d *DiscordAdapter) ParseIncoming(payload json.RawMessage) (*channel.IncomingMessage, error) {
	// We construct a custom map structure in the listener, so we parse it back here.
	var data struct {
		Message struct {
			Text      string `json:"text"`
			MessageID string `json:"message_id"`
			Chat      struct {
				ID string `json:"id"`
			} `json:"chat"`
		} `json:"message"`
		DiscordAuthor struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"discord_author"`
	}

	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("failed to parse discord payload: %w", err)
	}

	// For Discord, we use the numeric ID as UserID (assuming 64-bit env), but better to rely on string ID in metadata.
	// Since generic IncomingMessage.UserID is int, we try to parse it.
	userID, _ := strconv.Atoi(data.DiscordAuthor.ID)

	return &channel.IncomingMessage{
		UserID:  userID,
		Text:    data.Message.Text,
		Channel: "discord",
		RawMeta: DiscordMeta{
			ChannelID: data.Message.Chat.ID,
			MessageID: data.Message.MessageID,
		},
	}, nil
}

// Send delivers a non-streaming response via Discord.
func (d *DiscordAdapter) Send(msg *channel.IncomingMessage, out *channel.OutgoingMessage) error {
	meta := msg.RawMeta.(DiscordMeta)

	// Discord message limit is 2000 characters.
	content := out.Text
	if config.WatermarkModel {
		content = utils.Watermark(content, "", true)
	}

	// Simple send for now (no chunking implemented yet, assuming short responses or verify limit)
	// TODO: Implement chunking if > 2000 chars
	if len(content) > 2000 {
		content = content[:1990] + "..." // Truncate for safety for now
	}

	_, err := d.session.ChannelMessageSend(meta.ChannelID, content)
	return err
}

// SendStream delivers a streaming response via Discord (Send initial -> Edit loop).
func (d *DiscordAdapter) SendStream(msg *channel.IncomingMessage, streamFn func(onChunk func(chunk channel.StreamChunk)) error) (*channel.OutgoingMessage, error) {
	meta := msg.RawMeta.(DiscordMeta)

	// 1. Send initial message
	initialMsg, err := d.session.ChannelMessageSend(meta.ChannelID, "✨ Typing...")
	if err != nil {
		return nil, err
	}
	messageID := initialMsg.ID

	fullContent := ""
	lastUpdate := time.Now()
	updateInterval := 1000 * time.Millisecond // Discord rate limits edits, so we throttle updates

	err = streamFn(func(chunk channel.StreamChunk) {
		textToAdd := ""
		if chunk.ToolCalls != nil && len(chunk.ToolCalls) > 0 {
			textToAdd = fmt.Sprintf("\n🛠️ Using %s...\n", chunk.ToolCalls[0].Function.Name)
		} else if chunk.Text != "" {
			textToAdd = chunk.Text
		}

		if textToAdd != "" {
			fullContent += textToAdd

			// Update only if enough time passed or it's a tool call (immediate feedback)
			if time.Since(lastUpdate) > updateInterval {
				contentToSend := fullContent + " ✨"
				if len(contentToSend) > 2000 {
					contentToSend = contentToSend[len(contentToSend)-2000:] // Keep last 2000 chars
				}
				d.session.ChannelMessageEdit(meta.ChannelID, messageID, contentToSend)
				lastUpdate = time.Now()
			}
		}
	})

	// Final update
	contentToSend := fullContent
	if config.WatermarkModel {
		contentToSend = utils.Watermark(contentToSend, "", true)
	}
	if len(contentToSend) > 2000 {
		contentToSend = contentToSend[len(contentToSend)-2000:]
	}
	d.session.ChannelMessageEdit(meta.ChannelID, messageID, contentToSend)

	return &channel.OutgoingMessage{Text: fullContent}, err
}

// SendError delivers an error message via Discord.
func (d *DiscordAdapter) SendError(msg *channel.IncomingMessage, errText string) error {
	meta := msg.RawMeta.(DiscordMeta)
	_, err := d.session.ChannelMessageSend(meta.ChannelID, "❌ "+errText)
	return err
}
