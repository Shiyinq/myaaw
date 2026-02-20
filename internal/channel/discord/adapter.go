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
	"strings"
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

	// 1. Send thoughts if present
	if out.Thought != "" {
		thought := strings.TrimSpace(utils.StripMarkdown(out.Thought))
		quotedThought := "> ||" + strings.ReplaceAll(thought, "\n", "\n> ") + "||"
		displayThought := fmt.Sprintf("**💭 Reasoning...**\n%s", quotedThought)
		_, err := d.session.ChannelMessageSend(meta.ChannelID, displayThought)
		if err != nil {
			log.Printf("Error sending Discord thoughts: %v", err)
		}
	}

	// 2. Send main content
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
	maxLen := 2000
	updateInterval := 1500 * time.Millisecond // Slightly more conservative for Discord

	// 1. Send initial message
	initialMsg, err := d.session.ChannelMessageSend(meta.ChannelID, "✨ Typing...")
	if err != nil {
		return nil, err
	}
	firstMessageID := initialMsg.ID

	streamingContent := ""
	streamingThought := ""
	contentOffset := 0
	thoughtOffset := 0
	lastTraceLen := 0
	lastUpdate := time.Now()
	lastLoading := ""

	thoughtMessageID := ""
	mainMessageID := ""

	err = streamFn(func(chunk channel.StreamChunk) {
		loading := "✨ Typing..."

		if len(chunk.ToolCalls) > 0 {
			loading = fmt.Sprintf("🛠️ Using %s...", chunk.ToolCalls[0].Function.Name)
		}

		if chunk.Thought != "" {
			streamingThought += chunk.Thought
			loading = "💭 Reasoning..."
		}

		if chunk.Text != "" {
			streamingContent += chunk.Text
		}

		if len(chunk.Trace) > 0 {
			for i := lastTraceLen; i < len(chunk.Trace); i++ {
				step := chunk.Trace[i]
				if step.Observation != "" {
					streamingThought += fmt.Sprintf("\n\n🛠️ %s\n\n", step.Action)
					loading = "✨ Typing..."
				}
			}
			lastTraceLen = len(chunk.Trace)
		}

		now := time.Now()
		isThoughtUpdate := streamingThought != "" && (now.Sub(lastUpdate) > updateInterval || lastLoading != loading)
		isMainUpdate := streamingContent != "" && (now.Sub(lastUpdate) > updateInterval || lastLoading != loading)

		if streamingThought != "" {
			if thoughtMessageID == "" {
				if mainMessageID == "" {
					thoughtMessageID = firstMessageID
				} else {
					// Main message already took the first bubble. Spin up a new one for thoughts.
					newSend, err := d.session.ChannelMessageSend(meta.ChannelID, loading)
					if err != nil {
						log.Printf("Error creating Discord thought bubble: %v", err)
						thoughtMessageID = mainMessageID // Fallback
					} else {
						thoughtMessageID = newSend.ID
						thoughtOffset = len(streamingThought)
					}
				}
			}

			// Handle splitting if thought message is too long
			if len(streamingThought)-thoughtOffset >= maxLen-100 {
				displayThought := fmt.Sprintf("**💭 Reasoning...**\n> %s", utils.StripMarkdown(streamingThought[thoughtOffset:]))
				d.session.ChannelMessageEdit(meta.ChannelID, thoughtMessageID, displayThought) // Finalize current part

				newSend, err := d.session.ChannelMessageSend(meta.ChannelID, loading)
				if err == nil {
					thoughtMessageID = newSend.ID
					thoughtOffset = len(streamingThought)
				}
			}

			if isThoughtUpdate {
				thought := strings.TrimSpace(utils.StripMarkdown(streamingThought[thoughtOffset:]))
				quotedThought := "> ||" + strings.ReplaceAll(thought, "\n", "\n> ") + "||"
				displayThought := fmt.Sprintf("**💭 Reasoning...**\n%s", quotedThought)
				if len(displayThought) > maxLen-50 {
					displayThought = displayThought[:maxLen-50] + "..."
				}
				d.session.ChannelMessageEdit(meta.ChannelID, thoughtMessageID, displayThought+"\n\n"+loading)
				lastUpdate = now
			}
		}

		if streamingContent != "" {
			if mainMessageID == "" {
				if thoughtMessageID == "" {
					mainMessageID = firstMessageID
				} else {
					// Finalize thought message
					thought := strings.TrimSpace(utils.StripMarkdown(streamingThought[thoughtOffset:]))
					quotedThought := "> ||" + strings.ReplaceAll(thought, "\n", "\n> ") + "||"
					displayThought := fmt.Sprintf("**💭 Reasoning...**\n%s", quotedThought)
					if len(displayThought) > maxLen {
						displayThought = displayThought[:maxLen-3] + "..."
					}
					d.session.ChannelMessageEdit(meta.ChannelID, thoughtMessageID, displayThought)

					newSend, err := d.session.ChannelMessageSend(meta.ChannelID, loading)
					if err != nil {
						log.Printf("Error creating Discord main bubble: %v", err)
						mainMessageID = thoughtMessageID // Fallback
					} else {
						mainMessageID = newSend.ID
						contentOffset = len(streamingContent)
					}
				}
			}

			// Handle splitting if main message is too long
			if len(streamingContent)-contentOffset >= maxLen-100 {
				displayContent := streamingContent[contentOffset:]
				d.session.ChannelMessageEdit(meta.ChannelID, mainMessageID, displayContent) // Finalize current part

				newSend, err := d.session.ChannelMessageSend(meta.ChannelID, loading)
				if err == nil {
					mainMessageID = newSend.ID
					contentOffset = len(streamingContent)
				}
			}

			if isMainUpdate {
				displayContent := streamingContent[contentOffset:]
				if len(displayContent) > maxLen-50 {
					displayContent = displayContent[:maxLen-50] + "..."
				}
				d.session.ChannelMessageEdit(meta.ChannelID, mainMessageID, displayContent+"\n\n"+loading)
				lastUpdate = now
			}
		}

		lastLoading = loading
	})

	if err != nil {
		return nil, err
	}

	// Make sure the thought bubble has the indicator removed in all scenarios
	if thoughtMessageID != "" {
		thought := strings.TrimSpace(utils.StripMarkdown(streamingThought[thoughtOffset:]))
		quotedThought := "> ||" + strings.ReplaceAll(thought, "\n", "\n> ") + "||"
		displayThought := fmt.Sprintf("**💭 Reasoning...**\n%s", quotedThought)
		if len(displayThought) > maxLen {
			displayThought = displayThought[:maxLen-3] + "..."
		}
		d.session.ChannelMessageEdit(meta.ChannelID, thoughtMessageID, displayThought)
	}

	// Finalize main message
	if mainMessageID != "" {
		contentToSend := streamingContent[contentOffset:]
		if config.WatermarkModel {
			contentToSend = utils.Watermark(contentToSend, "", true)
		}
		if len(contentToSend) > maxLen {
			contentToSend = contentToSend[:maxLen-3] + "..."
		}
		d.session.ChannelMessageEdit(meta.ChannelID, mainMessageID, contentToSend)
	} else if mainMessageID == "" && thoughtMessageID == "" {
		// Fallback for first message if no content/thought ever came
		d.session.ChannelMessageEdit(meta.ChannelID, firstMessageID, utils.Watermark(streamingContent, "", true))
	}

	return &channel.OutgoingMessage{Text: streamingContent}, nil
}

func (d *DiscordAdapter) SendError(msg *channel.IncomingMessage, errText string) error {
	meta := msg.RawMeta.(DiscordMeta)
	_, err := d.session.ChannelMessageSend(meta.ChannelID, "❌ "+errText)
	return err
}
