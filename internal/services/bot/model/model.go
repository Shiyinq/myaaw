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
	Id     string `json:"id"`
	UserId int    `json:"userId"`
	// Scope groups conversations by chat target, for example a Telegram
	// topic. Empty means the legacy per-user DM/CLI/API scope.
	Scope string `json:"scope,omitempty"`
	// Channel is the platform that owns the conversation (telegram, discord,
	// api, cli). Empty for rows created before the column existed.
	Channel string `json:"channel,omitempty"`
	// ExternalSessionID and ExternalRuntime track the remote session/thread ID
	// when the agent runs on an external CLI runtime (ACP, Claude Code, Pi/OMP, Codex).
	ExternalSessionID string `json:"external_session_id,omitempty"`
	ExternalRuntime   string `json:"external_runtime,omitempty"`
	// WorkingDir optionally attaches a project or empty directory to this conversation.
	WorkingDir string             `json:"working_dir,omitempty"`
	Title      string             `json:"title"`
	Messages  []provider.Message `json:"messages"`
	Active    bool               `json:"active"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}
