package model

import (
	"teo/internal/provider"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	Id        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	UserId    int                `json:"user_id" bson:"userId"`
	Name      string             `json:"name" bson:"name"`
	System    string             `json:"system" bson:"system"`
	Model     string             `json:"model" bson:"model"`
	Messages  []provider.Message `json:"messages" bson:"messages"`
	Role      string             `json:"role" bson:"role"`
	CreatedAt time.Time          `json:"created_at" bson:"createdAt"`
	UpdatedAt time.Time          `json:"updated_at" bson:"updatedAt"`
}

// telegram format message
type UserMessage struct {
	Chat      Chat   `json:"chat"`
	Date      int64  `json:"date"`
	From      From   `json:"from"`
	MessageId int    `json:"message_id"`
	Text      string `json:"text"`
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
	Ok     bool        `json:"ok"`
	Result UserMessage `json:"result"`
}
