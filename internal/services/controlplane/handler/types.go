package handler

import (
	api "myaaw/internal/channel/api"
	"myaaw/internal/services/controlplane/service"
)

type LoginRequest struct {
	Username string `json:"username" example:"myaaw"`
	Password string `json:"password" example:"myaawadmin"`
}

type LoginResponse struct {
	Token     string `json:"token" example:"a1b2c3"`
	ExpiresAt string `json:"expires_at" example:"2026-10-20T20:00:00Z"`
}

type UsernameResponse struct {
	Username string `json:"username" example:"myaaw"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type ChangeUsernameRequest struct {
	Password    string `json:"password"`
	NewUsername string `json:"new_username"`
}

type OKResponse struct {
	OK bool `json:"ok" example:"true"`
}

type LoginOKResponse struct {
	OK       bool   `json:"ok" example:"true"`
	Username string `json:"username,omitempty" example:"myaaw"`
}

type AgentsResponse struct {
	Agents []service.AgentInfo `json:"agents"`
}

type HealthResponse struct {
	Status string `json:"status" example:"ok"`
}

// ChatRequest mirrors the agent chat request for documentation purposes.
type ChatRequest struct {
	UserID int      `json:"user_id" example:"12345"`
	Text   string   `json:"text" example:"Check my workspace"`
	Images []string `json:"images,omitempty"`
}

// ChatResponse mirrors the agent server response for documentation purposes.
// Chat requests are proxied, so this type is not used at runtime.
type ChatResponse struct {
	Text     string             `json:"text"`
	Timeline []api.TimelineItem `json:"timeline,omitempty"`
	Usage    *UsageResponse     `json:"usage,omitempty"`
}

type UsageResponse struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
