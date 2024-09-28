package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type OllamaRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role" bson:"role"`
	Content string `json:"content" bson:"content"`
}

type OllamaResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Message            Message   `json:"message"`
	DoneReason         string    `json:"done_reason"`
	Done               bool      `json:"done"`
	TotalDuration      int64     `json:"total_duration"`
	LoadDuration       int64     `json:"load_duration"`
	PromptEvalCount    int       `json:"prompt_eval_count"`
	PromptEvalDuration int64     `json:"prompt_eval_duration"`
	EvalCount          int       `json:"eval_count"`
	EvalDuration       int64     `json:"eval_duration"`
}

type User struct {
	Id        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	UserId    int                `json:"user_id" bson:"userId"`
	Name      string             `json:"name" bson:"name"`
	Messages  []Message          `json:"messages" bson:"messages"`
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

type TelegramTextMessage struct {
	Text             string `json:"text"`
	ReplyToMessageID *int   `json:"reply_to_message_id"`
	ChatID           int    `json:"chat_id"`
}

type TelegramSendMessageStatus struct {
	Ok bool `json:"ok"`
}
