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

type DiscordMeta struct {
	ChannelID string
	MessageID string
}

type DiscordAdapter struct {
	session *discordgo.Session
}

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

func (d *DiscordAdapter) StartListener(queueRepo repository.QueueRepository) error {
	d.session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore invalid messages or messages from bot itself
		if m.Author.ID == s.State.User.ID {
			return
		}

		userID, _ := strconv.Atoi(m.Author.ID)

		var images []string
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

		payload := map[string]interface{}{
			"message": map[string]interface{}{
				"from": map[string]interface{}{
					"id": userID,
				},
				"chat": map[string]interface{}{
					"id": m.ChannelID, // ChannelID is string
				},
				"message_id": m.ID,
				"text":       m.Content,
				"images":     images,
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

func (d *DiscordAdapter) ParseIncoming(payload json.RawMessage) (*channel.IncomingMessage, error) {
	// We construct a custom map structure in the listener, so we parse it back here.
	var data struct {
		Message struct {
			Text      string   `json:"text"`
			Images    []string `json:"images"`
			MessageID string   `json:"message_id"`
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

	userID, _ := strconv.Atoi(data.DiscordAuthor.ID)

	return &channel.IncomingMessage{
		UserID:  userID,
		Text:    data.Message.Text,
		Images:  data.Message.Images,
		Channel: "discord",
		RawMeta: DiscordMeta{
			ChannelID: data.Message.Chat.ID,
			MessageID: data.Message.MessageID,
		},
	}, nil
}

func (d *DiscordAdapter) Send(msg *channel.IncomingMessage, out *channel.OutgoingMessage) error {
	meta := msg.RawMeta.(DiscordMeta)

	// Discord message limit is 2000 characters.
	content := out.Text
	if config.WatermarkModel {
		content = utils.Watermark(content, "", true)
	}

	if len(content) > 2000 {
		content = content[:1990] + "..." // Truncate for safety for now
	}

	_, err := d.session.ChannelMessageSend(meta.ChannelID, content)
	return err
}

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
		if len(chunk.ToolCalls) > 0 {
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

func (d *DiscordAdapter) SendError(msg *channel.IncomingMessage, errText string) error {
	meta := msg.RawMeta.(DiscordMeta)
	_, err := d.session.ChannelMessageSend(meta.ChannelID, "❌ "+errText)
	return err
}
