package discord

import (
	"encoding/json"
	"fmt"
	"log"
	"myaaw/internal/channel"
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
	session   *discordgo.Session
	transport discordTransport
}

func NewDiscordAdapter(token string) (*DiscordAdapter, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %w", err)
	}
	return &DiscordAdapter{
		session:   session,
		transport: discordAPI{session: session},
	}, nil
}

func (d *DiscordAdapter) transportOrAPI() discordTransport {
	if d.transport != nil {
		return d.transport
	}
	if d.session != nil {
		return discordAPI{session: d.session}
	}
	return nil
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
	items := buildDiscordTimeline(out.Trace, out.Text, out.Thought)
	if len(items) == 0 {
		return nil
	}

	timeline := &discordTimeline{
		transport: d.transportOrAPI(),
		channelID: meta.ChannelID,
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
	return timeline.err
}

func (d *DiscordAdapter) SendStream(msg *channel.IncomingMessage, streamFn func(onChunk func(chunk channel.StreamChunk)) error) (*channel.OutgoingMessage, error) {
	meta := msg.RawMeta.(DiscordMeta)
	transport := d.transportOrAPI()
	if transport == nil {
		return nil, fmt.Errorf("discord transport is not configured")
	}

	initialID, err := transport.SendReply(meta.ChannelID, meta.MessageID, "✨ Typing...")
	if err != nil {
		return nil, err
	}

	timeline := newDiscordTimeline(transport, meta.ChannelID, meta.MessageID, initialID)
	err = streamFn(timeline.handleChunk)
	timeline.finish()
	if err != nil {
		return nil, err
	}

	return &channel.OutgoingMessage{
		Text:  timeline.finalText,
		Trace: timeline.finalTrace,
		Usage: timeline.finalUsage,
	}, nil
}

func (d *DiscordAdapter) SendError(msg *channel.IncomingMessage, errText string) error {
	meta := msg.RawMeta.(DiscordMeta)
	_, err := d.session.ChannelMessageSend(meta.ChannelID, "❌ "+errText)
	return err
}
