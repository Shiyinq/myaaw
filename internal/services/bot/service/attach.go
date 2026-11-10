package service

import (
	"log"
	"os"
	"strings"

	"myaaw/internal/agentdir"
	"myaaw/internal/attachment"
	"myaaw/internal/channel"
	"myaaw/internal/provider"
)

// messagingAttachmentMaxBytes caps a file read into memory for a messaging
// channel, which uploads bytes instead of exposing a path. Files above the cap
// are skipped rather than failing the whole turn.
const messagingAttachmentMaxBytes = 20 << 20

// inlineAttachmentMaxBytes keeps small API attachments base64-encoded as well as
// path-based, so clients that only understand the older shape keep working.
// Larger files are path-only and never enter the event stream.
const inlineAttachmentMaxBytes = 5 << 20

func attachmentRoots() attachment.Roots {
	name := agentdir.Active()
	home, _ := agentdir.Home(name)
	user, _ := agentdir.UserDir(name)
	skills, _ := agentdir.SkillsDir(name)
	jobs, _ := agentdir.JobsDir(name)
	return attachment.Roots{Home: home, User: user, Skills: skills, Jobs: jobs}
}

// deliverAttachments converts @attach(...) directives in the model's final
// answer into attachments the surface can show, and returns the text with the
// resolved directives removed.
//
// The API/WebUI surface receives a virtual path it can fetch (and stream with
// Range). Messaging surfaces receive the bytes, because their adapters upload
// files through the platform API. CLI/TUI keeps the text as-is: its prompt asks
// for a plain path instead of a directive.
func (r *BotServiceImpl) deliverAttachments(msg *channel.IncomingMessage, text string) (string, []channel.Attachment) {
	return attachmentsForChannel(msg.Channel, text, attachmentRoots())
}

func attachmentsForChannel(channelName, text string, roots attachment.Roots) (string, []channel.Attachment) {
	switch strings.ToLower(strings.TrimSpace(channelName)) {
	case "api":
		cleaned, items := collectItems(text, roots)
		atts := make([]channel.Attachment, 0, len(items))
		for _, item := range items {
			att := channel.Attachment{
				FileName: item.Name,
				MimeType: item.Mime,
				Path:     item.Virtual,
				Kind:     string(item.Kind),
				Size:     item.Size,
			}
			// Small files also carry their bytes for older API clients. The
			// WebUI ignores them and streams through the path instead.
			if item.Size <= inlineAttachmentMaxBytes {
				if data, err := os.ReadFile(item.AbsPath); err == nil {
					att.Data = data
				}
			}
			atts = append(atts, att)
		}
		return cleaned, atts
	case "telegram", "discord":
		return messagingAttachments(text, roots)
	default:
		// Surfaces without an attachment renderer get the plain path instead of
		// the directive.
		return attachment.Substitute(text, roots), nil
	}
}

// collectItems gathers both explicit @attach(...) directives and bare media
// paths, deduplicating by virtual path.
func collectItems(text string, roots attachment.Roots) (string, []attachment.Item) {
	cleaned, items := attachment.Extract(text, roots)
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		seen[item.Virtual] = true
	}
	for _, item := range attachment.Promote(cleaned, roots) {
		if seen[item.Virtual] {
			continue
		}
		seen[item.Virtual] = true
		items = append(items, item)
	}
	return cleaned, items
}

func messagingAttachments(text string, roots attachment.Roots) (string, []channel.Attachment) {
	cleaned, items := collectItems(text, roots)
	atts := make([]channel.Attachment, 0, len(items))
	for _, item := range items {
		if item.Size > messagingAttachmentMaxBytes {
			log.Printf("Skipping attachment %s: %d bytes exceeds the messaging cap", item.AbsPath, item.Size)
			continue
		}
		data, err := os.ReadFile(item.AbsPath)
		if err != nil {
			log.Printf("Skipping attachment %s: %v", item.AbsPath, err)
			continue
		}
		atts = append(atts, channel.Attachment{
			FileName: item.Name,
			MimeType: item.Mime,
			Data:     data,
		})
	}
	return cleaned, atts
}

// attachToFinalAssistant persists path-based attachments with the assistant
// turn so a reopened chat renders them again. Byte-only attachments (tool
// screenshots, messaging uploads) are not stored.
func attachToFinalAssistant(transcript []provider.Message, attachments []channel.Attachment) {
	items := make([]provider.MessageAttachment, 0, len(attachments))
	for _, att := range attachments {
		if att.Path == "" {
			continue
		}
		items = append(items, provider.MessageAttachment{
			FileName: att.FileName,
			MimeType: att.MimeType,
			Path:     att.Path,
			Kind:     att.Kind,
			Size:     att.Size,
		})
	}
	if len(items) == 0 {
		return
	}
	for i := len(transcript) - 1; i >= 0; i-- {
		if transcript[i].Role != "assistant" {
			continue
		}
		transcript[i].Attachments = items
		return
	}
}

// replaceFinalAssistantText rewrites the last assistant turn so the persisted
// transcript does not contain the delivery directive. Multimodal turns keep
// their non-text parts; only the text is replaced.
func replaceFinalAssistantText(transcript []provider.Message, text string) {
	for i := len(transcript) - 1; i >= 0; i-- {
		message := &transcript[i]
		if message.Role != "assistant" {
			continue
		}
		switch content := message.Content.(type) {
		case string:
			message.Content = text
		case []provider.ContentItem:
			items := append([]provider.ContentItem(nil), content...)
			firstText := -1
			for j := range items {
				if items[j].Type == "text" {
					firstText = j
					break
				}
			}
			if firstText == -1 {
				return
			}
			items[firstText].Text = text
			for j := firstText + 1; j < len(items); j++ {
				if items[j].Type == "text" {
					items[j].Text = ""
				}
			}
			message.Content = items
		}
		return
	}
}
