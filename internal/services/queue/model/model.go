package model

type UserMessage struct {
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

type TelegramSendMessage struct {
	Text             string `json:"text"`
	ParseMode        string `json:"parse_mode,omitempty"`
	ReplyToMessageID int    `json:"reply_to_message_id"`
	ChatID           int    `json:"chat_id"`
}

type TelegramEditMessage struct {
	Text             string `json:"text"`
	ParseMode        string `json:"parse_mode,omitempty"`
	MessageID        int    `json:"message_id"`
	ReplyToMessageID int    `json:"reply_to_message_id"`
	ChatID           int    `json:"chat_id"`
}

type TelegramSendMessageStatus struct {
	Ok          bool        `json:"ok"`
	Result      UserMessage `json:"result,omitempty"`
	ErrorCode   int         `json:"error_code,omitempty"`
	Description string      `json:"description,omitempty"`
}
