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
	Channel string `json:"channel"` // telegram, discord, api, cli
	To      string `json:"to"`      // user id (dm/api/cli), chat id (telegram), channel id (discord)
	// ThreadID is the Telegram message_thread_id for a forum topic.
	ThreadID string `json:"thread_id,omitempty"`
	// SessionID pins the exact conversation (required for api/cli targets).
	SessionID string `json:"session_id,omitempty"`
	// ChatType distinguishes a Discord DM from a guild channel ("dm" vs
	// "guild"). Telegram derives it from the sign of To.
	ChatType string `json:"chat_type,omitempty"`
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
