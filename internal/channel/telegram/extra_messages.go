package telegram

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"myaaw/internal/channel"
	"myaaw/internal/chatdb"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

const extraCallbackPrefix = "em:"

// telegramExtraCallbackQuery mirrors Telegram's CallbackQuery payload with the
// fields needed to verify ownership, lock the inline keyboard, and synthesize
// an incoming <interaction> chat message.
type telegramExtraCallbackQuery struct {
	ID      string       `json:"id"`
	Data    string       `json:"data"`
	From    *From        `json:"from"`
	Message *UserMessage `json:"message"`
}

// sendTelegramExtraMessages translates canonical extra_messages into native
// Telegram primitives (media groups/URLs, InlineKeyboardMarkup buttons, or
// clean Markdown fallbacks for unsupported widgets like dynamic forms).
func sendTelegramExtraMessages(
	db *sql.DB,
	chatID, threadID, replyTo, userID int,
	conversationID string,
	extras []provider.ExtraMessage,
) error {
	if len(extras) == 0 {
		return nil
	}
	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	for _, em := range extras {
		for _, block := range em.Blocks {
			switch block.Type {
			case "media":
				recordErr(sendTelegramExtraMedia(chatID, threadID, replyTo, block))
			case "buttons":
				recordErr(sendTelegramExtraButtons(db, chatID, threadID, replyTo, userID, conversationID, block))
			case "quick_replies":
				recordErr(sendTelegramExtraQuickReplies(db, chatID, threadID, replyTo, userID, conversationID, block))
			case "carousel":
				recordErr(sendTelegramExtraCarousel(db, chatID, threadID, replyTo, userID, conversationID, block))
			default:
				text := channel.FormatExtraBlockFallback(block)
				if text != "" {
					_, err := SendTelegramMessage(chatID, threadID, replyTo, text, "markdown")
					recordErr(err)
				}
			}
		}
	}
	return firstErr
}

// sendTelegramExtraMedia sends online-URL media items natively via Telegram
// (albums of 2–10 photos/videos via sendMediaGroup, or individual sendPhoto /
// sendVideo / sendAudio / sendDocument calls). Local path items already ride
// OutgoingMessage.Attachments bytes and are sent by sendTelegramAttachments.
func sendTelegramExtraMedia(chatID, threadID, replyTo int, block provider.ExtraBlock) error {
	var data channel.ExtraMediaData
	if err := json.Unmarshal(block.Data, &data); err != nil {
		return nil
	}

	var visualURLs []channel.ExtraMediaItem
	var otherURLs []channel.ExtraMediaItem
	for _, item := range data.Items {
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		if item.Kind == "image" || item.Kind == "video" {
			visualURLs = append(visualURLs, item)
		} else {
			otherURLs = append(otherURLs, item)
		}
	}

	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Send online photos/videos in albums of 2..10, or single calls.
	for start := 0; start < len(visualURLs); start += telegramAlbumMax {
		end := start + telegramAlbumMax
		if end > len(visualURLs) {
			end = len(visualURLs)
		}
		chunk := visualURLs[start:end]
		if len(chunk) == 1 {
			item := chunk[0]
			method, field := "sendPhoto", "photo"
			if item.Kind == "video" {
				method, field = "sendVideo", "video"
			}
			caption := item.Caption
			if caption == "" && start == 0 {
				caption = data.Caption
			}
			body := map[string]any{
				"chat_id": chatID,
				field:     item.URL,
			}
			if threadID != 0 {
				body["message_thread_id"] = threadID
			}
			if replyTo != 0 {
				body["reply_to_message_id"] = replyTo
			}
			if caption != "" {
				body["caption"] = caption
			}
			_, err := SendTelegramRequest(method, body, chatID, threadID)
			recordErr(err)
			continue
		}

		media := make([]map[string]any, 0, len(chunk))
		for idx, item := range chunk {
			t := "photo"
			if item.Kind == "video" {
				t = "video"
			}
			entry := map[string]any{
				"type":  t,
				"media": item.URL,
			}
			caption := item.Caption
			if caption == "" && idx == 0 && start == 0 {
				caption = data.Caption
			}
			if caption != "" {
				entry["caption"] = caption
			}
			media = append(media, entry)
		}
		body := map[string]any{
			"chat_id": chatID,
			"media":   media,
		}
		if threadID != 0 {
			body["message_thread_id"] = threadID
		}
		if replyTo != 0 {
			body["reply_to_message_id"] = replyTo
		}
		_, err := SendTelegramRequest("sendMediaGroup", body, chatID, threadID)
		recordErr(err)
	}

	// Send audio/pdf/file URLs individually.
	for _, item := range otherURLs {
		method, field := "sendDocument", "document"
		if item.Kind == "audio" {
			method, field = "sendAudio", "audio"
		}
		body := map[string]any{
			"chat_id": chatID,
			field:     item.URL,
		}
		if threadID != 0 {
			body["message_thread_id"] = threadID
		}
		if replyTo != 0 {
			body["reply_to_message_id"] = replyTo
		}
		if item.Caption != "" {
			body["caption"] = item.Caption
		} else if item.Title != "" {
			body["caption"] = item.Title
		}
		_, err := SendTelegramRequest(method, body, chatID, threadID)
		recordErr(err)
	}

	return firstErr
}

func sendTelegramExtraButtons(
	db *sql.DB,
	chatID, threadID, replyTo, userID int,
	conversationID string,
	block provider.ExtraBlock,
) error {
	var data channel.ExtraButtonsData
	if err := json.Unmarshal(block.Data, &data); err != nil || len(data.Buttons) == 0 {
		return nil
	}
	rows := buildTelegramButtonRows(db, userID, conversationID, block.ID, "", data.Buttons, data.Layout == "stack")
	text := data.Prompt
	if text == "" {
		text = "Choose an option:"
	}
	body := &TelegramSendMessage{
		Text:             text,
		ChatID:           chatID,
		MessageThreadID:  threadID,
		ReplyToMessageID: replyTo,
		ReplyMarkup:      map[string]any{"inline_keyboard": rows},
	}
	_, err := SendTelegramRequest("sendMessage", body, chatID, threadID)
	return err
}

func sendTelegramExtraQuickReplies(
	db *sql.DB,
	chatID, threadID, replyTo, userID int,
	conversationID string,
	block provider.ExtraBlock,
) error {
	var data channel.ExtraQuickRepliesData
	if err := json.Unmarshal(block.Data, &data); err != nil || len(data.Options) == 0 {
		return nil
	}
	var rows [][]map[string]string
	var cur []map[string]string
	for _, opt := range data.Options {
		key, err := saveExtraCallback(db, chatdb.InteractionCallback{
			ConversationID: conversationID,
			UserID:         userID,
			Channel:        "telegram",
			BlockID:        block.ID,
			Kind:           provider.InteractionQuickReply,
			ActionID:       opt.ID,
			Label:          opt.Label,
			Value:          opt.Value,
		})
		if err != nil {
			continue
		}
		cur = append(cur, map[string]string{
			"text":          opt.Label,
			"callback_data": extraCallbackPrefix + key,
		})
		if len(cur) == 2 {
			rows = append(rows, cur)
			cur = nil
		}
	}
	if len(cur) > 0 {
		rows = append(rows, cur)
	}
	if len(rows) == 0 {
		return nil
	}
	text := data.Prompt
	if text == "" {
		text = "Quick replies:"
	}
	body := &TelegramSendMessage{
		Text:             text,
		ChatID:           chatID,
		MessageThreadID:  threadID,
		ReplyToMessageID: replyTo,
		ReplyMarkup:      map[string]any{"inline_keyboard": rows},
	}
	_, err := SendTelegramRequest("sendMessage", body, chatID, threadID)
	return err
}

func sendTelegramExtraCarousel(
	db *sql.DB,
	chatID, threadID, replyTo, userID int,
	conversationID string,
	block provider.ExtraBlock,
) error {
	var data channel.ExtraCarouselData
	if err := json.Unmarshal(block.Data, &data); err != nil || len(data.Cards) == 0 {
		return nil
	}
	var firstErr error
	recordErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Send up to 5 cards individually (with image_url + inline keyboard when present).
	// For >5 cards, fall back to a compact formatted list + aggregated action buttons.
	if len(data.Cards) <= 5 {
		for _, card := range data.Cards {
			var caption strings.Builder
			caption.WriteString(card.Title)
			if card.Subtitle != "" {
				caption.WriteString("\n" + card.Subtitle)
			}
			if card.Description != "" {
				caption.WriteString("\n\n" + card.Description)
			}
			rows := buildTelegramButtonRows(db, userID, conversationID, block.ID, card.ID, card.Buttons, false)

			if card.ImageURL != "" {
				body := map[string]any{
					"chat_id": chatID,
					"photo":   card.ImageURL,
					"caption": caption.String(),
				}
				if threadID != 0 {
					body["message_thread_id"] = threadID
				}
				if replyTo != 0 {
					body["reply_to_message_id"] = replyTo
				}
				if len(rows) > 0 {
					body["reply_markup"] = map[string]any{"inline_keyboard": rows}
				}
				_, err := SendTelegramRequest("sendPhoto", body, chatID, threadID)
				recordErr(err)
			} else {
				body := &TelegramSendMessage{
					Text:             caption.String(),
					ChatID:           chatID,
					MessageThreadID:  threadID,
					ReplyToMessageID: replyTo,
				}
				if len(rows) > 0 {
					body.ReplyMarkup = map[string]any{"inline_keyboard": rows}
				}
				_, err := SendTelegramRequest("sendMessage", body, chatID, threadID)
				recordErr(err)
			}
		}
		return firstErr
	}

	text := channel.FormatExtraBlockFallback(block)
	var allRows [][]map[string]string
	for _, card := range data.Cards {
		for _, btn := range card.Buttons {
			lbl := card.Title + ": " + btn.Label
			if btn.Type == "link" && btn.URL != "" {
				allRows = append(allRows, []map[string]string{{"text": lbl, "url": btn.URL}})
				continue
			}
			key, err := saveExtraCallback(db, chatdb.InteractionCallback{
				ConversationID: conversationID,
				UserID:         userID,
				Channel:        "telegram",
				BlockID:        block.ID,
				Kind:           provider.InteractionButton,
				ActionID:       btn.ID,
				CardID:         card.ID,
				Label:          lbl,
				Value:          btn.Value,
			})
			if err == nil {
				allRows = append(allRows, []map[string]string{{"text": lbl, "callback_data": extraCallbackPrefix + key}})
			}
		}
	}
	body := &TelegramSendMessage{
		Text:             text,
		ChatID:           chatID,
		MessageThreadID:  threadID,
		ReplyToMessageID: replyTo,
	}
	if len(allRows) > 0 {
		body.ReplyMarkup = map[string]any{"inline_keyboard": allRows}
	}
	_, err := SendTelegramRequest("sendMessage", body, chatID, threadID)
	return err
}

func buildTelegramButtonRows(
	db *sql.DB,
	userID int,
	conversationID, blockID, cardID string,
	buttons []channel.ExtraButton,
	stack bool,
) [][]map[string]string {
	var rows [][]map[string]string
	var cur []map[string]string
	perRow := 2
	if stack {
		perRow = 1
	}
	for _, btn := range buttons {
		var entry map[string]string
		if btn.Type == "link" && btn.URL != "" {
			entry = map[string]string{"text": btn.Label, "url": btn.URL}
		} else {
			key, err := saveExtraCallback(db, chatdb.InteractionCallback{
				ConversationID: conversationID,
				UserID:         userID,
				Channel:        "telegram",
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
			entry = map[string]string{
				"text":          btn.Label,
				"callback_data": extraCallbackPrefix + key,
			}
		}
		cur = append(cur, entry)
		if len(cur) >= perRow {
			rows = append(rows, cur)
			cur = nil
		}
	}
	if len(cur) > 0 {
		rows = append(rows, cur)
	}
	return rows
}

func saveExtraCallback(db *sql.DB, cb chatdb.InteractionCallback) (string, error) {
	if db == nil {
		db = config.DB
	}
	if db == nil {
		return "", fmt.Errorf("sqlite db not initialized")
	}
	return chatdb.SaveInteractionCallback(db, cb)
}

// HandleExtraMessageCallback handles Telegram callback queries created by
// extra_messages (`em:<key>`). It returns (syntheticMessagePayload, true) when
// the update was an extra_messages callback:
//   - If syntheticMessagePayload is non-empty, callers should publish it to the
//     queue so the agent processes the `<interaction>` turn.
//   - If syntheticMessagePayload is nil and handled is true, the callback was
//     rejected (expired or already submitted) and must not be enqueued.
func HandleExtraMessageCallback(payload json.RawMessage) (json.RawMessage, bool) {
	return handleExtraMessageCallbackWithDB(config.DB, payload)
}

func handleExtraMessageCallbackWithDB(db *sql.DB, payload json.RawMessage) (json.RawMessage, bool) {
	var update struct {
		UpdateID      int64                       `json:"update_id"`
		CallbackQuery *telegramExtraCallbackQuery `json:"callback_query"`
	}
	if err := json.Unmarshal(payload, &update); err != nil || update.CallbackQuery == nil {
		return nil, false
	}
	cb := update.CallbackQuery
	if !strings.HasPrefix(cb.Data, extraCallbackPrefix) {
		return nil, false
	}
	key := strings.TrimPrefix(cb.Data, extraCallbackPrefix)

	rec, found, err := chatdb.GetInteractionCallback(db, key)
	if err != nil || !found {
		answerCallbackQuery(cb.ID, "This button has expired.")
		return nil, true
	}

	// Enforce matching user in private DMs when a user_id was recorded.
	if rec.UserID != 0 && cb.From != nil && cb.Message != nil && cb.Message.Chat.Type == "private" && cb.From.Id != rec.UserID {
		answerCallbackQuery(cb.ID, "Only the conversation owner can click this.")
		return nil, true
	}

	interactionPayload := map[string]any{
		"block_id":  rec.BlockID,
		"action_id": rec.ActionID,
		"label":     rec.Label,
		"value":     rec.Value,
	}
	if rec.CardID != "" {
		interactionPayload["card_id"] = rec.CardID
	}
	summary := strings.TrimSpace(rec.Label)
	if summary == "" {
		summary = strings.TrimSpace(rec.Value)
	}

	already, _, lockErr := chatdb.MarkBlockSubmitted(db, rec.ConversationID, rec.BlockID, "telegram", summary, interactionPayload)
	if lockErr != nil {
		log.Printf("telegram extra_messages: lock block %s: %v", rec.BlockID, lockErr)
	}
	if already {
		answerCallbackQuery(cb.ID, "Already submitted.")
		if cb.Message != nil {
			clearTelegramInlineKeyboard(cb.Message.Chat.Id, cb.Message.MessageId)
		}
		return nil, true
	}

	answerCallbackQuery(cb.ID, "✓ "+summary)
	if cb.Message != nil {
		clearTelegramInlineKeyboard(cb.Message.Chat.Id, cb.Message.MessageId)
	}

	interactionText := provider.InteractionBlock(rec.Kind, interactionPayload, summary)
	synth := TelegramIncommingChat{
		UpdateId: update.UpdateID,
		Message: UserMessage{
			Text: interactionText,
		},
	}
	if cb.From != nil {
		synth.Message.From = *cb.From
	}
	if cb.Message != nil {
		synth.Message.Chat = cb.Message.Chat
		synth.Message.MessageId = cb.Message.MessageId
		synth.Message.MessageThreadID = cb.Message.MessageThreadID
	}
	raw, err := json.Marshal(synth)
	if err != nil {
		return nil, true
	}
	return raw, true
}

func clearTelegramInlineKeyboard(chatID, messageID int) {
	if chatID == 0 || messageID == 0 {
		return
	}
	client := resty.New().SetTimeout(10 * time.Second)
	_, _ = client.R().
		SetBody(map[string]any{
			"chat_id":      chatID,
			"message_id":   messageID,
			"reply_markup": map[string]any{"inline_keyboard": [][]any{}},
		}).
		Post(telegramAPIURL("editMessageReplyMarkup"))
}
