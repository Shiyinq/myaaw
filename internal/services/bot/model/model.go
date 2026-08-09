package model

import (
	"myaaw/internal/provider"
	"time"
)

type User struct {
	Id        string    `json:"id"`
	UserId    int       `json:"user_id"`
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Conversation struct {
	Id        string             `json:"id"`
	UserId    int                `json:"userId"`
	Title     string             `json:"title"`
	Messages  []provider.Message `json:"messages"`
	Active    bool               `json:"active"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}
