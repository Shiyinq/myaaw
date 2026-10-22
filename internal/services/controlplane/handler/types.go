package handler

import (
	"encoding/json"
	"myaaw/internal/agent/skills"
	api "myaaw/internal/channel/api"
	"myaaw/internal/cron"
	"myaaw/internal/services/controlplane/service"
)

type LoginRequest struct {
	Username string `json:"username" example:"myaaw"`
	Password string `json:"password" example:"myaawadmin"`
}

type LoginResponse struct {
	Token     string `json:"token" example:"a1b2c3"`
	ExpiresAt string `json:"expires_at" example:"2026-10-20T20:00:00Z"`
	UserID    int    `json:"user_id" example:"0"`
}

type UsernameResponse struct {
	Username string `json:"username" example:"myaaw"`
	UserID   int    `json:"user_id" example:"0"`
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

// SkillView mirrors skills.Skill for documentation purposes.
type SkillView = skills.Skill

type SkillsResponse struct {
	Skills []skills.Skill `json:"skills"`
}

type SkillToggleRequest struct {
	Enabled *bool `json:"enabled" example:"true"`
}

type SkillToggleResponse struct {
	OK      bool   `json:"ok" example:"true"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type BuiltinToggleRequest struct {
	Enabled *bool `json:"enabled" example:"true"`
}

type CreateCustomToolRequest struct {
	Name    string          `json:"name" example:"hello_helper"`
	Command string          `json:"command" example:"node"`
	Args    []string        `json:"args,omitempty"`
	Schema  json.RawMessage `json:"schema,omitempty"`
	Enabled *bool           `json:"enabled,omitempty" example:"true"`
}

type PatchCustomToolRequest struct {
	Enabled *bool            `json:"enabled,omitempty"`
	Command *string          `json:"command,omitempty"`
	Args    *[]string        `json:"args,omitempty"`
	Schema  *json.RawMessage `json:"schema,omitempty"`
}

type CreateMCPRequest struct {
	Name    string            `json:"name" example:"playwright"`
	Command string            `json:"command" example:"npx"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Enabled *bool             `json:"enabled,omitempty" example:"true"`
}

type PatchMCPRequest struct {
	Enabled *bool              `json:"enabled,omitempty"`
	Command *string            `json:"command,omitempty"`
	Args    *[]string          `json:"args,omitempty"`
	Env     *map[string]string `json:"env,omitempty"`
}

// ChatRequest mirrors the agent chat request for documentation purposes.
type ChatRequest struct {
	UserID         int      `json:"user_id" example:"0"`
	Text           string   `json:"text" example:"Check my workspace"`
	Images         []string `json:"images,omitempty"`
	ConversationID string   `json:"conversation_id,omitempty" example:"conv-id or NEW"`
}

// ChatResponse mirrors the agent server response for documentation purposes.
type ChatResponse struct {
	Text           string             `json:"text"`
	ConversationID string             `json:"conversation_id,omitempty"`
	Timeline       []api.TimelineItem `json:"timeline,omitempty"`
	Usage          *UsageResponse     `json:"usage,omitempty"`
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

// WriteContextRequest updates a context file by home-relative path.
type WriteContextRequest struct {
	Path    string `json:"path" example:"memory/RULES.md"`
	Content string `json:"content" example:"# Daily rules"`
}

// ContextFilesResponse is the dedicated context file listing.
type ContextFilesResponse struct {
	Files []service.ContextFile `json:"files"`
}

// CreateContactRequest registers another agent as a contact.
type CreateContactRequest struct {
	Name        string `json:"name" example:"research"`
	URL         string `json:"url" example:"https://other-host/a2a"`
	Token       string `json:"token,omitempty"`
	Description string `json:"description,omitempty" example:"Research agent"`
}

type ContactsResponse struct {
	Contacts []service.ContactView `json:"contacts"`
}

type MkdirRequest struct {
	Path string `json:"path" example:"/home/notes"`
}

type RenameRequest struct {
	From string `json:"from" example:"/home/AGENTS.md"`
	To   string `json:"to" example:"/home/AGENTS.old.md"`
}

type RenameConversationRequest struct {
	Title string `json:"title" example:"Workspace cleanup"`
}

type RenameConversationResponse struct {
	OK    bool   `json:"ok" example:"true"`
	ID    string `json:"id"`
	Title string `json:"title"`
}

type FileListResponse struct {
	Path    string              `json:"path"`
	Entries []service.FileEntry `json:"entries"`
}

type SearchResponse struct {
	Matches []service.SearchMatch `json:"matches"`
}

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
