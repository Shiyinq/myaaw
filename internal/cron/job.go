package cron

import "time"

type Schedule struct {
	Kind string `json:"kind"` // cron, every, at
	Expr string `json:"expr"` // * * * * * or 30m
	Tz   string `json:"tz"`   // Asia/Jakarta
}

type Payload struct {
	Kind    string `json:"kind"`    // agentTurn
	Content string `json:"content"` // prompt
}

type Delivery struct {
	Mode    string `json:"mode"`    // announce
	Channel string `json:"channel"` // telegram, discord
	To      string `json:"to"`      // chat_id
}

type Job struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	AgentID   string    `json:"agent_id"`
	Schedule  Schedule  `json:"schedule"`
	Payload   Payload   `json:"payload"`
	Delivery  Delivery  `json:"delivery"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
