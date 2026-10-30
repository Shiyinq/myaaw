package telegram

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	telegramRequestTimeout = 15 * time.Second
	telegramTypingTimeout  = 5 * time.Second
	telegramFileTimeout    = 60 * time.Second
	telegramMaxRetries     = 2
	telegramMaxRetryAfter  = 30 * time.Second
)

// telegramAPIBaseURL is a variable so tests can point the client at a local
// server instead of api.telegram.org.
var telegramAPIBaseURL = "https://api.telegram.org"

func telegramAPIURL(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", telegramAPIBaseURL, config.TelegramBotToken, method)
}

func telegramFileURL(filePath string) string {
	return fmt.Sprintf("%s/file/bot%s/%s", telegramAPIBaseURL, config.TelegramBotToken, filePath)
}

// telegram format message
type UserMessage struct {
	Chat           Chat            `json:"chat"`
	Date           int64           `json:"date"`
	From           From            `json:"from"`
	MessageId      int             `json:"message_id"`
	ReplyToMessage *ReplyToMessage `json:"reply_to_message,omitempty"`
	Text           string          `json:"text,omitempty"`
	Photo          []Photo         `json:"photo,omitempty"`
	Document       *Document       `json:"document,omitempty"`
	Caption        string          `json:"caption,omitempty"`
	Voice          *Voice          `json:"voice,omitempty"`
}

type ReplyToMessage struct {
	Chat      Chat      `json:"chat"`
	Date      int64     `json:"date"`
	From      From      `json:"from"`
	MessageId int       `json:"message_id"`
	Text      string    `json:"text,omitempty"`
	Photo     []Photo   `json:"photo,omitempty"`
	Document  *Document `json:"document,omitempty"`
	Caption   string    `json:"caption,omitempty"`
	Voice     *Voice    `json:"voice,omitempty"`
}

type Photo struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int    `json:"file_size"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
}

type Thumbnail struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int    `json:"file_size"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
}

type Document struct {
	FileName     string    `json:"file_name"`
	MimeType     string    `json:"mime_type"`
	Thumbnail    Thumbnail `json:"thumbnail"`
	Thumb        Thumbnail `json:"thumb"`
	FileID       string    `json:"file_id"`
	FileUniqueID string    `json:"file_unique_id"`
	FileSize     int       `json:"file_size"`
}

type Voice struct {
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type"`
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int    `json:"file_size"`
}

type Chat struct {
	FirstName string `json:"first_name"`
	Id        int    `json:"id"`
	Type      string `json:"type"`
	Username  string `json:"username"`
}

type From struct {
	FirstName    string `json:"first_name"`
	Id           int    `json:"id"`
	IsBot        bool   `json:"is_bot"`
	LanguageCode string `json:"language_code"`
	Username     string `json:"username"`
}

type TelegramIncommingChat struct {
	Message  UserMessage `json:"message"`
	UpdateId int64       `json:"update_id"`
}

type TelegramLinkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled,omitempty"`
}

type TelegramSendMessage struct {
	Text                  string                      `json:"text"`
	ParseMode             string                      `json:"parse_mode,omitempty"`
	ReplyToMessageID      int                         `json:"reply_to_message_id"`
	ChatID                int                         `json:"chat_id"`
	DisableWebPagePreview bool                        `json:"disable_web_page_preview,omitempty"`
	LinkPreviewOptions    *TelegramLinkPreviewOptions `json:"link_preview_options,omitempty"`
	ReplyMarkup           interface{}                 `json:"reply_markup,omitempty"`
}

type TelegramEditMessage struct {
	Text                  string                      `json:"text"`
	ParseMode             string                      `json:"parse_mode,omitempty"`
	MessageID             int                         `json:"message_id"`
	ReplyToMessageID      int                         `json:"reply_to_message_id"`
	ChatID                int                         `json:"chat_id"`
	DisableWebPagePreview bool                        `json:"disable_web_page_preview,omitempty"`
	LinkPreviewOptions    *TelegramLinkPreviewOptions `json:"link_preview_options,omitempty"`
}

type TelegramResponseParameters struct {
	RetryAfter int `json:"retry_after,omitempty"`
}

type TelegramSendMessageStatus struct {
	Ok          bool                        `json:"ok"`
	Result      UserMessage                 `json:"result,omitempty"`
	ErrorCode   int                         `json:"error_code,omitempty"`
	Description string                      `json:"description,omitempty"`
	Parameters  *TelegramResponseParameters `json:"parameters,omitempty"`
}

type TelegramFileResponse struct {
	Ok     bool `json:"ok"`
	Result struct {
		FileID       string `json:"file_id"`
		FileUniqueID string `json:"file_unique_id"`
		FileSize     int    `json:"file_size"`
		FilePath     string `json:"file_path"`
	} `json:"result"`
}

// sendTelegramTypingAction sends the "typing" indicator. It is best-effort: a
// failure here must never take down the process or block the message itself.
func sendTelegramTypingAction(chatId int) error {
	client := resty.New().SetTimeout(telegramTypingTimeout)

	_, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]interface{}{
			"chat_id": chatId,
			"action":  "typing",
		}).
		Post(telegramAPIURL("sendChatAction"))
	if err != nil {
		return fmt.Errorf("send typing indicator: %w", err)
	}
	return nil
}

// sendTelegramPhoto uploads one attachment as a real image message. Telegram's
// sendPhoto expects multipart/form-data.
func sendTelegramPhoto(chatID, replyTo int, att channel.Attachment) error {
	if len(att.Data) == 0 {
		return nil
	}
	name := strings.TrimSpace(att.FileName)
	if name == "" {
		name = "attachment.png"
	}
	form := map[string]string{"chat_id": strconv.Itoa(chatID)}
	if replyTo != 0 {
		form["reply_to_message_id"] = strconv.Itoa(replyTo)
	}

	client := resty.New().SetTimeout(telegramFileTimeout)
	resp, err := client.R().
		SetFileReader("photo", name, bytes.NewReader(att.Data)).
		SetFormData(form).
		Post(telegramAPIURL("sendPhoto"))
	if err != nil {
		return fmt.Errorf("send photo: %w", err)
	}
	if resp.IsError() {
		return fmt.Errorf("send photo failed: %s", resp.Status())
	}
	return nil
}

func telegramSendBody(chatID, replyID int, text, parseMode string) *TelegramSendMessage {
	body := &TelegramSendMessage{
		Text:                  text,
		ChatID:                chatID,
		DisableWebPagePreview: true,
		LinkPreviewOptions:    &TelegramLinkPreviewOptions{IsDisabled: true},
	}
	if replyID != 0 {
		body.ReplyToMessageID = replyID
	}
	if parseMode != "" {
		body.ParseMode = parseMode
	}
	return body
}

func telegramEditBody(chatID, replyID, messageID int, text, parseMode string) *TelegramEditMessage {
	body := &TelegramEditMessage{
		Text:                  text,
		MessageID:             messageID,
		ReplyToMessageID:      replyID,
		ChatID:                chatID,
		DisableWebPagePreview: true,
		LinkPreviewOptions:    &TelegramLinkPreviewOptions{IsDisabled: true},
	}
	if parseMode != "" {
		body.ParseMode = parseMode
	}
	return body
}

func SendTelegramMessage(chatId int, replyId int, text string, parseMode string) (*TelegramSendMessageStatus, error) {
	return SendTelegramRequest("sendMessage", telegramSendBody(chatId, replyId, text, parseMode), chatId)
}

func EditTelegramMessage(chatId int, replyId int, editMessageId int, text string, parseMode string) (*TelegramSendMessageStatus, error) {
	return SendTelegramRequest("editMessageText", telegramEditBody(chatId, replyId, editMessageId, text, parseMode), chatId)
}

func SendTelegramRequest(method string, message interface{}, chatId int) (*TelegramSendMessageStatus, error) {
	if method != "editMessageText" {
		if err := sendTelegramTypingAction(chatId); err != nil {
			log.Printf("telegram: %v", err)
		}
	}

	client := resty.New().SetTimeout(telegramRequestTimeout)
	url := telegramAPIURL(method)

	// Editing a message is idempotent, so a transport failure can be retried.
	// Sending is not: the message may already have been delivered, and a retry
	// would duplicate it. Telegram error responses (429/5xx) are safe for both.
	retryTransport := method == "editMessageText"

	var (
		response TelegramSendMessageStatus
		lastErr  error
		delay    time.Duration
	)
	for attempt := 0; attempt <= telegramMaxRetries; attempt++ {
		if delay > 0 {
			time.Sleep(delay)
		}
		delay = time.Duration(attempt+1) * 500 * time.Millisecond

		response = TelegramSendMessageStatus{}
		resp, err := client.R().
			SetHeader("Accept", "application/json").
			SetHeader("Content-Type", "application/json").
			SetBody(message).
			SetResult(&response).
			SetError(&response).
			Post(url)

		if err != nil {
			lastErr = fmt.Errorf("telegram %s request failed: %w", method, err)
			if !retryTransport || attempt == telegramMaxRetries {
				return &response, lastErr
			}
			continue
		}

		if resp.StatusCode() == 200 {
			return &response, nil
		}

		lastErr = fmt.Errorf("failed to %s message, %s %v", method, resp.Status(), response.Description)
		if !telegramRetryableStatus(resp.StatusCode()) || attempt == telegramMaxRetries {
			return &response, lastErr
		}

		// Telegram rate limits include the delay it expects us to wait.
		if response.Parameters != nil && response.Parameters.RetryAfter > 0 {
			delay = min(time.Duration(response.Parameters.RetryAfter)*time.Second, telegramMaxRetryAfter)
		}
	}

	return &response, lastErr
}

// telegramRetryableStatus reports whether an HTTP status is worth retrying.
func telegramRetryableStatus(status int) bool {
	return status == 429 || (status >= 500 && status <= 599)
}

func GetFilePath(fileID string) (string, error) {
	url := telegramAPIURL("getFile?file_id=" + url.QueryEscape(fileID))
	client := resty.New().SetTimeout(telegramRequestTimeout)
	resp, err := client.R().Get(url)
	if err != nil {
		return "", fmt.Errorf("error while making GET request: %v", err)
	}

	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("failed to get file path, status code: %d", resp.StatusCode())
	}

	var fileResponse TelegramFileResponse
	if err := json.Unmarshal(resp.Body(), &fileResponse); err != nil {
		return "", fmt.Errorf("error unmarshalling JSON response: %v", err)
	}

	if !fileResponse.Ok {
		return "", fmt.Errorf("failed to get file path, API response not OK")
	}

	return fileResponse.Result.FilePath, nil
}

func TelegramImageURL(filePath string) string {
	return telegramFileURL(filePath)
}

func ImageURLToBase64(filePath string) (string, error) {
	url := telegramFileURL(filePath)
	client := resty.New().SetTimeout(telegramFileTimeout)
	resp, err := client.R().Get(url)
	if err != nil {
		return "", fmt.Errorf("error while making GET request: %v", err)
	}

	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("failed to fetch image, status code: %d", resp.StatusCode())
	}

	base64Image := base64.StdEncoding.EncodeToString(resp.Body())
	base64Cleaned := strings.TrimPrefix(base64Image, "data:image/png;base64,")

	return base64Cleaned, nil
}

func DownloadTgFile(filePath string) ([]byte, error) {
	url := telegramFileURL(filePath)
	client := resty.New().SetTimeout(telegramFileTimeout)
	resp, err := client.R().Get(url)
	if err != nil {
		return nil, fmt.Errorf("error while making GET request for file download: %v", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("failed to fetch file, status code: %d, response: %s", resp.StatusCode(), resp.String())
	}

	return resp.Body(), nil
}
