package discord

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"myaaw/internal/channel"
	"myaaw/internal/chatdb"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/services/queue/repository"
)

const (
	discordExtraButtonPrefix = "em:"
	discordExtraQRPrefix     = "em_qr:"
	discordExtraChkPrefix    = "em_chk:"
	discordExtraFormPrefix   = "em_form:"
	discordExtraModalPrefix  = "em_modal:"
)

func (d *DiscordAdapter) sendExtraMessages(channelID string, userID int, conversationID string, extras []provider.ExtraMessage) error {
	if len(extras) == 0 {
		return nil
	}
	if d.session == nil {
		return nil
	}
	return sendDiscordExtraMessagesWithSender(config.DB, channelID, userID, conversationID, extras, func(msg *discordgo.MessageSend) error {
		_, err := d.session.ChannelMessageSendComplex(channelID, msg)
		return err
	})
}

func sendDiscordExtraMessagesWithSender(
	db *sql.DB,
	channelID string,
	userID int,
	conversationID string,
	extras []provider.ExtraMessage,
	sendFn func(msg *discordgo.MessageSend) error,
) error {
	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	for _, em := range extras {
		for _, block := range em.Blocks {
			payloads := buildDiscordBlockMessages(db, userID, conversationID, block)
			for _, p := range payloads {
				recordErr(sendFn(p))
			}
		}
	}
	return firstErr
}

func buildDiscordBlockMessages(db *sql.DB, userID int, conversationID string, block provider.ExtraBlock) []*discordgo.MessageSend {
	switch block.Type {
	case "media":
		return buildDiscordMediaMessages(block)
	case "buttons":
		return buildDiscordButtonsMessages(db, userID, conversationID, block)
	case "quick_replies":
		return buildDiscordQuickRepliesMessages(db, userID, conversationID, block)
	case "carousel":
		return buildDiscordCarouselMessages(db, userID, conversationID, block)
	case "form":
		return buildDiscordFormMessages(db, userID, conversationID, block)
	case "checklist":
		return buildDiscordChecklistMessages(db, userID, conversationID, block)
	case "callout":
		return buildDiscordCalloutMessages(block)
	case "summary":
		return buildDiscordSummaryMessages(block)
	case "link_preview":
		return buildDiscordLinkPreviewMessages(block)
	default:
		if text := channel.FormatExtraBlockFallback(block); text != "" {
			return []*discordgo.MessageSend{{Content: text}}
		}
		return nil
	}
}

func buildDiscordMediaMessages(block provider.ExtraBlock) []*discordgo.MessageSend {
	var data channel.ExtraMediaData
	if json.Unmarshal(block.Data, &data) != nil || len(data.Items) == 0 {
		return nil
	}
	var embeds []*discordgo.MessageEmbed
	var linkLines []string
	for _, item := range data.Items {
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		if item.Kind == "image" {
			emb := &discordgo.MessageEmbed{
				Title:       item.Title,
				Description: item.Caption,
				Image:       &discordgo.MessageEmbedImage{URL: item.URL},
			}
			embeds = append(embeds, emb)
		} else {
			label := item.Title
			if label == "" {
				label = item.FileName
			}
			if label == "" {
				label = item.URL
			}
			linkLines = append(linkLines, fmt.Sprintf("📎 [%s](%s)", label, item.URL))
		}
	}
	if len(embeds) == 0 && len(linkLines) == 0 {
		return nil
	}
	msg := &discordgo.MessageSend{}
	if len(linkLines) > 0 {
		msg.Content = strings.Join(linkLines, "\n")
	}
	if data.Caption != "" {
		if msg.Content != "" {
			msg.Content += "\n" + data.Caption
		} else {
			msg.Content = data.Caption
		}
	}
	if len(embeds) > 10 {
		embeds = embeds[:10]
	}
	msg.Embeds = embeds
	return []*discordgo.MessageSend{msg}
}

func discordButtonStyle(style string) discordgo.ButtonStyle {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "primary":
		return discordgo.PrimaryButton
	case "danger":
		return discordgo.DangerButton
	default:
		return discordgo.SecondaryButton
	}
}

func buildDiscordButtonsMessages(db *sql.DB, userID int, conversationID string, block provider.ExtraBlock) []*discordgo.MessageSend {
	var data channel.ExtraButtonsData
	if json.Unmarshal(block.Data, &data) != nil || len(data.Buttons) == 0 {
		return nil
	}
	rows := buildDiscordButtonRows(db, userID, conversationID, block.ID, "", data.Buttons, data.Layout == "stack")
	if len(rows) == 0 {
		return nil
	}
	content := data.Prompt
	if content == "" {
		content = "Choose an option:"
	}
	return []*discordgo.MessageSend{{
		Content:    content,
		Components: rows,
	}}
}

func buildDiscordButtonRows(
	db *sql.DB,
	userID int,
	conversationID, blockID, cardID string,
	buttons []channel.ExtraButton,
	stack bool,
) []discordgo.MessageComponent {
	perRow := 5
	if stack {
		perRow = 1
	}
	var rows []discordgo.MessageComponent
	var cur []discordgo.MessageComponent
	for _, btn := range buttons {
		if len(rows) >= 5 {
			break
		}
		lbl := clipDiscordText(btn.Label, 80)
		var comp discordgo.MessageComponent
		if btn.Type == "link" && btn.URL != "" {
			comp = discordgo.Button{
				Label: lbl,
				Style: discordgo.LinkButton,
				URL:   btn.URL,
			}
		} else {
			key, err := saveDiscordCallback(db, chatdb.InteractionCallback{
				ConversationID: conversationID,
				UserID:         userID,
				Channel:        "discord",
				BlockID:        blockID,
				Kind:           provider.InteractionButton,
				ActionID:       btn.ID,
				CardID:         cardID,
				Label:          btn.Label,
				Value:          btn.Value,
			})
			if err != nil {
				continue
			}
			comp = discordgo.Button{
				Label:    lbl,
				Style:    discordButtonStyle(btn.Style),
				CustomID: discordExtraButtonPrefix + key,
			}
		}
		cur = append(cur, comp)
		if len(cur) >= perRow {
			rows = append(rows, discordgo.ActionsRow{Components: cur})
			cur = nil
		}
	}
	if len(cur) > 0 && len(rows) < 5 {
		rows = append(rows, discordgo.ActionsRow{Components: cur})
	}
	return rows
}

func buildDiscordQuickRepliesMessages(db *sql.DB, userID int, conversationID string, block provider.ExtraBlock) []*discordgo.MessageSend {
	var data channel.ExtraQuickRepliesData
	if json.Unmarshal(block.Data, &data) != nil || len(data.Options) == 0 {
		return nil
	}
	content := data.Prompt
	if content == "" {
		content = "Quick replies:"
	}

	if len(data.Options) <= 5 {
		var comps []discordgo.MessageComponent
		for _, opt := range data.Options {
			key, err := saveDiscordCallback(db, chatdb.InteractionCallback{
				ConversationID: conversationID,
				UserID:         userID,
				Channel:        "discord",
				BlockID:        block.ID,
				Kind:           provider.InteractionQuickReply,
				ActionID:       opt.ID,
				Label:          opt.Label,
				Value:          opt.Value,
			})
			if err != nil {
				continue
			}
			comps = append(comps, discordgo.Button{
				Label:    clipDiscordText(opt.Label, 80),
				Style:    discordgo.SecondaryButton,
				CustomID: discordExtraButtonPrefix + key,
			})
		}
		if len(comps) == 0 {
			return nil
		}
		return []*discordgo.MessageSend{{
			Content:    content,
			Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: comps}},
		}}
	}

	// >5 options: use a Discord SelectMenu.
	metaBytes, _ := json.Marshal(data.Options)
	key, err := saveDiscordCallback(db, chatdb.InteractionCallback{
		ConversationID: conversationID,
		UserID:         userID,
		Channel:        "discord",
		BlockID:        block.ID,
		Kind:           provider.InteractionQuickReply,
		Label:          content,
		MetaJSON:       string(metaBytes),
	})
	if err != nil {
		return []*discordgo.MessageSend{{Content: channel.FormatExtraBlockFallback(block)}}
	}
	opts := make([]discordgo.SelectMenuOption, 0, len(data.Options))
	for i, opt := range data.Options {
		if i >= 25 {
			break
		}
		opts = append(opts, discordgo.SelectMenuOption{
			Label: clipDiscordText(opt.Label, 100),
			Value: clipDiscordText(opt.ID, 100),
		})
	}
	minOne := 1
	return []*discordgo.MessageSend{{
		Content: content,
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    discordExtraQRPrefix + key,
					Placeholder: "Choose an option…",
					MinValues:   &minOne,
					MaxValues:   1,
					Options:     opts,
				},
			}},
		},
	}}
}

func buildDiscordCarouselMessages(db *sql.DB, userID int, conversationID string, block provider.ExtraBlock) []*discordgo.MessageSend {
	var data channel.ExtraCarouselData
	if json.Unmarshal(block.Data, &data) != nil || len(data.Cards) == 0 {
		return nil
	}
	embeds := make([]*discordgo.MessageEmbed, 0, len(data.Cards))
	var allButtons []channel.ExtraButton
	cardByBtn := make(map[string]string)

	for i, card := range data.Cards {
		if i >= 10 {
			break
		}
		desc := card.Subtitle
		if card.Description != "" {
			if desc != "" {
				desc += "\n\n" + card.Description
			} else {
				desc = card.Description
			}
		}
		emb := &discordgo.MessageEmbed{
			Title:       clipDiscordText(card.Title, 256),
			Description: clipDiscordText(desc, 2000),
		}
		if card.ImageURL != "" {
			emb.Image = &discordgo.MessageEmbedImage{URL: card.ImageURL}
		}
		if len(card.Badges) > 0 {
			emb.Footer = &discordgo.MessageEmbedFooter{Text: strings.Join(card.Badges, " · ")}
		}
		embeds = append(embeds, emb)
		for _, btn := range card.Buttons {
			b := btn
			if len(data.Cards) > 1 {
				b.Label = clipDiscordText(card.Title+": "+btn.Label, 80)
			}
			allButtons = append(allButtons, b)
			cardByBtn[b.ID] = card.ID
		}
	}

	var rows []discordgo.MessageComponent
	if len(allButtons) > 0 {
		var cur []discordgo.MessageComponent
		for _, btn := range allButtons {
			if len(rows) >= 5 {
				break
			}
			var comp discordgo.MessageComponent
			if btn.Type == "link" && btn.URL != "" {
				comp = discordgo.Button{
					Label: clipDiscordText(btn.Label, 80),
					Style: discordgo.LinkButton,
					URL:   btn.URL,
				}
			} else {
				key, err := saveDiscordCallback(db, chatdb.InteractionCallback{
					ConversationID: conversationID,
					UserID:         userID,
					Channel:        "discord",
					BlockID:        block.ID,
					Kind:           provider.InteractionButton,
					ActionID:       btn.ID,
					CardID:         cardByBtn[btn.ID],
					Label:          btn.Label,
					Value:          btn.Value,
				})
				if err != nil {
					continue
				}
				comp = discordgo.Button{
					Label:    clipDiscordText(btn.Label, 80),
					Style:    discordButtonStyle(btn.Style),
					CustomID: discordExtraButtonPrefix + key,
				}
			}
			cur = append(cur, comp)
			if len(cur) >= 5 {
				rows = append(rows, discordgo.ActionsRow{Components: cur})
				cur = nil
			}
		}
		if len(cur) > 0 && len(rows) < 5 {
			rows = append(rows, discordgo.ActionsRow{Components: cur})
		}
	}

	return []*discordgo.MessageSend{{
		Embeds:     embeds,
		Components: rows,
	}}
}

func isDiscordModalCompatible(form channel.ExtraFormData) bool {
	if len(form.Fields) == 0 || len(form.Fields) > 5 {
		return false
	}
	for _, f := range form.Fields {
		switch strings.ToLower(strings.TrimSpace(f.Type)) {
		case "text", "textarea", "number", "":
		default:
			return false
		}
	}
	return true
}

func buildDiscordFormMessages(db *sql.DB, userID int, conversationID string, block provider.ExtraBlock) []*discordgo.MessageSend {
	var data channel.ExtraFormData
	if json.Unmarshal(block.Data, &data) != nil || len(data.Fields) == 0 {
		return nil
	}

	if isDiscordModalCompatible(data) {
		metaBytes, _ := json.Marshal(data)
		key, err := saveDiscordCallback(db, chatdb.InteractionCallback{
			ConversationID: conversationID,
			UserID:         userID,
			Channel:        "discord",
			BlockID:        block.ID,
			Kind:           provider.InteractionForm,
			ActionID:       data.ID,
			Label:          data.Title,
			MetaJSON:       string(metaBytes),
		})
		if err == nil {
			btnLabel := data.SubmitLabel
			if btnLabel == "" {
				btnLabel = "Fill Form"
			}
			return []*discordgo.MessageSend{{
				Embeds: []*discordgo.MessageEmbed{{
					Title:       "📋 " + clipDiscordText(data.Title, 250),
					Description: clipDiscordText(data.Description, 2000),
					Color:       0x5865F2,
				}},
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    clipDiscordText(btnLabel, 80),
							Style:    discordgo.PrimaryButton,
							CustomID: discordExtraFormPrefix + key,
						},
					}},
				},
			}}
		}
	}

	return []*discordgo.MessageSend{{
		Embeds: []*discordgo.MessageEmbed{{
			Title:       "📋 " + clipDiscordText(data.Title, 250),
			Description: clipDiscordText(channel.FormatExtraBlockFallback(block), 4000),
			Color:       0x5865F2,
		}},
	}}
}

func buildDiscordChecklistMessages(db *sql.DB, userID int, conversationID string, block provider.ExtraBlock) []*discordgo.MessageSend {
	var data channel.ExtraChecklistData
	if json.Unmarshal(block.Data, &data) != nil || len(data.Items) == 0 {
		return nil
	}
	if len(data.Items) <= 25 {
		metaBytes, _ := json.Marshal(data)
		key, err := saveDiscordCallback(db, chatdb.InteractionCallback{
			ConversationID: conversationID,
			UserID:         userID,
			Channel:        "discord",
			BlockID:        block.ID,
			Kind:           provider.InteractionChecklist,
			Label:          data.Title,
			MetaJSON:       string(metaBytes),
		})
		if err == nil {
			opts := make([]discordgo.SelectMenuOption, 0, len(data.Items))
			for _, it := range data.Items {
				opts = append(opts, discordgo.SelectMenuOption{
					Label:       clipDiscordText(it.Label, 100),
					Value:       clipDiscordText(it.ID, 100),
					Description: clipDiscordText(it.Description, 100),
					Default:     it.Checked,
				})
			}
			minZero := 0
			title := data.Title
			if title == "" {
				title = "Checklist"
			}
			return []*discordgo.MessageSend{{
				Content: "**" + title + "**" + func() string {
					if data.Description != "" {
						return "\n" + data.Description
					}
					return ""
				}(),
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.SelectMenu{
							CustomID:    discordExtraChkPrefix + key,
							Placeholder: "Select items…",
							MinValues:   &minZero,
							MaxValues:   len(opts),
							Options:     opts,
						},
					}},
				},
			}}
		}
	}
	return []*discordgo.MessageSend{{Content: channel.FormatExtraBlockFallback(block)}}
}

func buildDiscordCalloutMessages(block provider.ExtraBlock) []*discordgo.MessageSend {
	var data channel.ExtraCalloutData
	if json.Unmarshal(block.Data, &data) != nil || data.Text == "" {
		return nil
	}
	color := 0x38BDF8
	prefix := "ℹ️ "
	switch strings.ToLower(data.Variant) {
	case "success":
		color = 0x22C55E
		prefix = "✅ "
	case "warning":
		color = 0xF59E0B
		prefix = "⚠️ "
	case "error":
		color = 0xEF4444
		prefix = "❌ "
	}
	title := data.Title
	if title != "" {
		title = prefix + title
	}
	desc := data.Text
	if title == "" {
		desc = prefix + desc
	}
	return []*discordgo.MessageSend{{
		Embeds: []*discordgo.MessageEmbed{{
			Title:       clipDiscordText(title, 256),
			Description: clipDiscordText(desc, 4000),
			Color:       color,
		}},
	}}
}

func buildDiscordSummaryMessages(block provider.ExtraBlock) []*discordgo.MessageSend {
	var data channel.ExtraSummaryData
	if json.Unmarshal(block.Data, &data) != nil || len(data.Items) == 0 {
		return nil
	}
	fields := make([]*discordgo.MessageEmbedField, 0, len(data.Items))
	for _, it := range data.Items {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   clipDiscordText(it.Label, 256),
			Value:  clipDiscordText(it.Value, 1024),
			Inline: true,
		})
	}
	title := data.Title
	if title == "" {
		title = "Summary"
	}
	return []*discordgo.MessageSend{{
		Embeds: []*discordgo.MessageEmbed{{
			Title:       clipDiscordText(title, 256),
			Description: clipDiscordText(data.Subtitle, 2000),
			Fields:      fields,
			Color:       0x64748B,
		}},
	}}
}

func buildDiscordLinkPreviewMessages(block provider.ExtraBlock) []*discordgo.MessageSend {
	var data channel.ExtraLinkPreviewData
	if json.Unmarshal(block.Data, &data) != nil || data.URL == "" {
		return nil
	}
	emb := &discordgo.MessageEmbed{
		Title:       clipDiscordText(data.Title, 256),
		URL:         data.URL,
		Description: clipDiscordText(data.Description, 2000),
	}
	if data.Domain != "" {
		emb.Footer = &discordgo.MessageEmbedFooter{Text: data.Domain}
	}
	if data.ImageURL != "" {
		emb.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: data.ImageURL}
	}
	return []*discordgo.MessageSend{{Embeds: []*discordgo.MessageEmbed{emb}}}
}

func saveDiscordCallback(db *sql.DB, cb chatdb.InteractionCallback) (string, error) {
	if db == nil {
		db = config.DB
	}
	if db == nil {
		return "", fmt.Errorf("sqlite db not initialized")
	}
	return chatdb.SaveInteractionCallback(db, cb)
}

func clipDiscordText(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

// ---------------------------------------------------------------------------
// Discord InteractionCreate handler for extra_messages (buttons, menus, modals)
// ---------------------------------------------------------------------------

func handleExtraMessageInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, queueRepo repository.QueueRepository) {
	switch i.Type {
	case discordgo.InteractionMessageComponent:
		customID := i.MessageComponentData().CustomID
		switch {
		case strings.HasPrefix(customID, discordExtraFormPrefix):
			openDiscordExtraFormModal(config.DB, s, i, strings.TrimPrefix(customID, discordExtraFormPrefix))
		case strings.HasPrefix(customID, discordExtraButtonPrefix):
			resolveDiscordButtonInteraction(config.DB, s, i, queueRepo, strings.TrimPrefix(customID, discordExtraButtonPrefix))
		case strings.HasPrefix(customID, discordExtraQRPrefix):
			resolveDiscordQuickReplyMenu(config.DB, s, i, queueRepo, strings.TrimPrefix(customID, discordExtraQRPrefix))
		case strings.HasPrefix(customID, discordExtraChkPrefix):
			resolveDiscordChecklistMenu(config.DB, s, i, queueRepo, strings.TrimPrefix(customID, discordExtraChkPrefix))
		}
	case discordgo.InteractionModalSubmit:
		customID := i.ModalSubmitData().CustomID
		if strings.HasPrefix(customID, discordExtraModalPrefix) {
			resolveDiscordFormModalSubmit(config.DB, s, i, queueRepo, strings.TrimPrefix(customID, discordExtraModalPrefix))
		}
	}
}

func respondDiscordEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	if s == nil {
		return
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func resolveDiscordButtonInteraction(db *sql.DB, s *discordgo.Session, i *discordgo.InteractionCreate, queueRepo repository.QueueRepository, key string) {
	rec, found, err := chatdb.GetInteractionCallback(db, key)
	if err != nil || !found {
		respondDiscordEphemeral(s, i, "This button has expired.")
		return
	}
	payload := map[string]any{
		"block_id":  rec.BlockID,
		"action_id": rec.ActionID,
		"label":     rec.Label,
		"value":     rec.Value,
	}
	if rec.CardID != "" {
		payload["card_id"] = rec.CardID
	}
	summary := strings.TrimSpace(rec.Label)
	if summary == "" {
		summary = strings.TrimSpace(rec.Value)
	}

	already, _, lockErr := chatdb.MarkBlockSubmitted(db, rec.ConversationID, rec.BlockID, "discord", summary, payload)
	if lockErr != nil {
		log.Printf("discord extra_messages: lock block %s: %v", rec.BlockID, lockErr)
	}
	if already {
		respondDiscordEphemeral(s, i, "Already submitted.")
		return
	}

	if s != nil {
		updatedContent := "✓ Selected: **" + summary + "**"
		if i.Message != nil && strings.TrimSpace(i.Message.Content) != "" {
			updatedContent = i.Message.Content + "\n✓ Selected: **" + summary + "**"
		}
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    updatedContent,
				Components: []discordgo.MessageComponent{},
			},
		})
	}

	enqueueDiscordInteraction(s, i, queueRepo, provider.InteractionBlock(rec.Kind, payload, summary))
}

func resolveDiscordQuickReplyMenu(db *sql.DB, s *discordgo.Session, i *discordgo.InteractionCreate, queueRepo repository.QueueRepository, key string) {
	rec, found, err := chatdb.GetInteractionCallback(db, key)
	if err != nil || !found {
		respondDiscordEphemeral(s, i, "This menu has expired.")
		return
	}
	vals := i.MessageComponentData().Values
	if len(vals) == 0 {
		respondDiscordEphemeral(s, i, "No option selected.")
		return
	}
	chosenID := vals[0]
	chosenLabel := chosenID
	chosenVal := chosenID
	var opts []channel.ExtraQuickReplyOption
	if json.Unmarshal([]byte(rec.MetaJSON), &opts) == nil {
		for _, o := range opts {
			if o.ID == chosenID {
				chosenLabel = o.Label
				chosenVal = o.Value
				break
			}
		}
	}
	payload := map[string]any{
		"block_id":  rec.BlockID,
		"action_id": chosenID,
		"label":     chosenLabel,
		"value":     chosenVal,
	}
	already, _, _ := chatdb.MarkBlockSubmitted(db, rec.ConversationID, rec.BlockID, "discord", chosenLabel, payload)
	if already {
		respondDiscordEphemeral(s, i, "Already submitted.")
		return
	}
	if s != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "✓ Selected: **" + chosenLabel + "**",
				Components: []discordgo.MessageComponent{},
			},
		})
	}
	enqueueDiscordInteraction(s, i, queueRepo, provider.InteractionBlock(provider.InteractionQuickReply, payload, chosenLabel))
}

func resolveDiscordChecklistMenu(db *sql.DB, s *discordgo.Session, i *discordgo.InteractionCreate, queueRepo repository.QueueRepository, key string) {
	rec, found, err := chatdb.GetInteractionCallback(db, key)
	if err != nil || !found {
		respondDiscordEphemeral(s, i, "This checklist has expired.")
		return
	}
	selectedIDs := i.MessageComponentData().Values
	idSet := make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		idSet[id] = true
	}
	var data channel.ExtraChecklistData
	_ = json.Unmarshal([]byte(rec.MetaJSON), &data)
	var labels []string
	for _, item := range data.Items {
		if idSet[item.ID] {
			labels = append(labels, item.Label)
		}
	}
	summary := "None selected"
	if len(labels) > 0 {
		summary = strings.Join(labels, ", ")
	}
	if data.Title != "" {
		summary = data.Title + ": " + summary
	}
	payload := map[string]any{
		"block_id": rec.BlockID,
		"label":    data.Title,
		"values": map[string]any{
			"checked_ids":    selectedIDs,
			"checked_labels": labels,
		},
	}
	already, _, _ := chatdb.MarkBlockSubmitted(db, rec.ConversationID, rec.BlockID, "discord", summary, payload)
	if already {
		respondDiscordEphemeral(s, i, "Already submitted.")
		return
	}
	if s != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "☑️ Submitted: **" + summary + "**",
				Components: []discordgo.MessageComponent{},
			},
		})
	}
	enqueueDiscordInteraction(s, i, queueRepo, provider.InteractionBlock(provider.InteractionChecklist, payload, summary))
}

func openDiscordExtraFormModal(db *sql.DB, s *discordgo.Session, i *discordgo.InteractionCreate, key string) {
	rec, found, err := chatdb.GetInteractionCallback(db, key)
	if err != nil || !found {
		respondDiscordEphemeral(s, i, "This form has expired.")
		return
	}
	var form channel.ExtraFormData
	if err := json.Unmarshal([]byte(rec.MetaJSON), &form); err != nil || len(form.Fields) == 0 {
		respondDiscordEphemeral(s, i, "Form definition unavailable.")
		return
	}
	components := make([]discordgo.MessageComponent, 0, len(form.Fields))
	for idx, f := range form.Fields {
		if idx >= 5 {
			break
		}
		style := discordgo.TextInputShort
		if strings.EqualFold(f.Type, "textarea") {
			style = discordgo.TextInputParagraph
		}
		defaultVal := ""
		if f.Default != nil {
			defaultVal = fmt.Sprint(f.Default)
		}
		components = append(components, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    f.Name,
					Label:       clipDiscordText(f.Label, 45),
					Style:       style,
					Placeholder: clipDiscordText(f.Placeholder, 100),
					Value:       clipDiscordText(defaultVal, 4000),
					Required:    f.Required,
				},
			},
		})
	}
	if s != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID:   discordExtraModalPrefix + key,
				Title:      clipDiscordText(form.Title, 45),
				Components: components,
			},
		})
	}
}

func resolveDiscordFormModalSubmit(db *sql.DB, s *discordgo.Session, i *discordgo.InteractionCreate, queueRepo repository.QueueRepository, key string) {
	rec, found, err := chatdb.GetInteractionCallback(db, key)
	if err != nil || !found {
		respondDiscordEphemeral(s, i, "This form has expired.")
		return
	}
	var form channel.ExtraFormData
	_ = json.Unmarshal([]byte(rec.MetaJSON), &form)
	labelByName := make(map[string]string, len(form.Fields))
	for _, f := range form.Fields {
		labelByName[f.Name] = f.Label
	}

	values := make(map[string]any)
	var summaryParts []string
	data := i.ModalSubmitData()
	for _, comp := range data.Components {
		row, ok := comp.(*discordgo.ActionsRow)
		if !ok {
			if valRow, okVal := comp.(discordgo.ActionsRow); okVal {
				row = &valRow
			}
		}
		if row == nil {
			continue
		}
		for _, inner := range row.Components {
			ti, ok := inner.(*discordgo.TextInput)
			if !ok {
				if valTI, okVal := inner.(discordgo.TextInput); okVal {
					ti = &valTI
				}
			}
			if ti == nil {
				continue
			}
			values[ti.CustomID] = ti.Value
			if strings.TrimSpace(ti.Value) != "" {
				lbl := labelByName[ti.CustomID]
				if lbl == "" {
					lbl = ti.CustomID
				}
				summaryParts = append(summaryParts, fmt.Sprintf("%s: %s", lbl, strings.TrimSpace(ti.Value)))
			}
		}
	}

	summary := form.Title
	if len(summaryParts) > 0 {
		summary = fmt.Sprintf("%s: %s", form.Title, strings.Join(summaryParts, " · "))
	}
	payload := map[string]any{
		"block_id":  rec.BlockID,
		"action_id": rec.ActionID,
		"label":     form.Title,
		"values":    values,
	}

	already, _, _ := chatdb.MarkBlockSubmitted(db, rec.ConversationID, rec.BlockID, "discord", summary, payload)
	if already {
		respondDiscordEphemeral(s, i, "Already submitted.")
		return
	}
	if s != nil {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "📋 Submitted **" + form.Title + "**",
			},
		})
	}
	enqueueDiscordInteraction(s, i, queueRepo, provider.InteractionBlock(provider.InteractionForm, payload, summary))
}

func enqueueDiscordInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, queueRepo repository.QueueRepository, interactionText string) {
	if queueRepo == nil {
		return
	}
	var author *discordgo.User
	switch {
	case i.User != nil:
		author = i.User
	case i.Member != nil && i.Member.User != nil:
		author = i.Member.User
	}
	if author == nil {
		return
	}
	msgID := ""
	if i.Message != nil {
		msgID = i.Message.ID
	}
	var (
		channelInfo *discordgo.Channel
		guildInfo   *discordgo.Guild
	)
	if s != nil && s.State != nil {
		if c, err := s.State.Channel(i.ChannelID); err == nil {
			channelInfo = c
		}
		if i.GuildID != "" {
			if g, err := s.State.Guild(i.GuildID); err == nil {
				guildInfo = g
			}
		}
	}

	rawPayload := buildInteractionMessagePayload(i.ChannelID, i.GuildID, msgID, author, channelInfo, guildInfo, interactionText)
	envelope := &channel.QueueEnvelope{
		Channel: "discord",
		Payload: rawPayload,
	}
	envelopeBytes, _ := json.Marshal(envelope)
	if err := queueRepo.PublishMessage(envelopeBytes); err != nil {
		log.Printf("discord: failed to publish interaction message: %v", err)
	}
}

func buildInteractionMessagePayload(
	channelID, guildID, messageID string,
	author *discordgo.User,
	channelInfo *discordgo.Channel,
	guildInfo *discordgo.Guild,
	text string,
) []byte {
	chatType := "guild"
	if guildID == "" {
		chatType = "dm"
	}
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
	displayName := strings.TrimSpace(author.GlobalName)
	if displayName == "" {
		displayName = strings.TrimSpace(author.Username)
	}
	userID, _ := strconv.Atoi(author.ID)

	payload := map[string]any{
		"message": map[string]any{
			"from": map[string]any{
				"id":         userID,
				"first_name": displayName,
				"username":   author.Username,
			},
			"chat": map[string]any{
				"id":        channelID,
				"type":      chatType,
				"name":      channelName,
				"is_thread": isThread,
			},
			"guild": map[string]any{
				"id":   guildID,
				"name": guildName,
			},
			"message_id": messageID,
			"text":       text,
			"images":     []string{},
		},
		"original_source": "discord",
		"discord_author": map[string]any{
			"id":          author.ID,
			"username":    author.Username,
			"global_name": author.GlobalName,
		},
	}
	raw, _ := json.Marshal(payload)
	return raw
}
