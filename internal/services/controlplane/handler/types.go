package handler

import (
	api "myaaw/internal/channel/api"
	"myaaw/internal/cron"
	"myaaw/internal/services/bot/model"
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

type WriteFileRequest struct {
	Path    string `json:"path" example:"/home/AGENTS.md"`
	Content string `json:"content" example:"# Instructions"`
}

type MkdirRequest struct {
	Path string `json:"path" example:"/home/notes"`
}

type RenameRequest struct {
	From string `json:"from" example:"/home/AGENTS.md"`
	To   string `json:"to" example:"/home/AGENTS.old.md"`
}

type FileListResponse struct {
	Path    string              `json:"path"`
	Entries []service.FileEntry `json:"entries"`
}

type SearchResponse struct {
	Matches []service.SearchMatch `json:"matches"`
}

type ConversationsResponse struct {
	Conversations []service.ConversationSummary `json:"conversations"`
}

type ConversationResponse = model.Conversation

type LogsResponse struct {
	Files []service.LogFile `json:"files"`
}

type CreateAgentRequest struct {
	Name string `json:"name" example:"myfriend"`
}

type CreateProviderRequest struct {
	Name         string `json:"name" example:"openai"`
	Type         string `json:"type" example:"openai"`
	BaseURL      string `json:"base_url,omitempty" example:"https://api.openai.com"`
	APIKey       string `json:"api_key,omitempty"`
	DefaultModel string `json:"default_model,omitempty" example:"gpt-4o"`
}

type UpdateProviderRequest struct {
	Type         *string `json:"type,omitempty"`
	BaseURL      *string `json:"base_url,omitempty"`
	APIKey       *string `json:"api_key,omitempty"`
	DefaultModel *string `json:"default_model,omitempty"`
}

type SetDefaultProviderRequest struct {
	Name string `json:"name" example:"openai"`
}

type ChannelRequest struct {
	Active         *bool  `json:"active,omitempty"`
	Token          string `json:"token,omitempty"`
	Mode           string `json:"mode,omitempty" example:"polling"`
	NgrokActive    *bool  `json:"ngrok_active,omitempty"`
	NgrokAuthToken string `json:"ngrok_auth_token,omitempty"`
}

type CronJobsResponse struct {
	Jobs []cron.Job `json:"jobs"`
}

type CronHistoryResponse struct {
	History []cron.ExecutionResult `json:"history"`
}

type CronJobResponse = cron.Job
