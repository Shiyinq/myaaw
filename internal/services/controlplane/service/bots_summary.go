package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"myaaw/internal/provider"
)

// BotRunSummary mirrors the subset of /api/chat/runs needed by the dashboard
// to re-attach to active DM streams on boot without per-agent fan-out.
type BotRunSummary struct {
	RequestID      string `json:"request_id"`
	ConversationID string `json:"conversation_id,omitempty"`
	Scope          string `json:"scope,omitempty"`
	Streaming      bool   `json:"streaming"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at,omitempty"`
}

// BotSidebarSummary holds the latest DM preview, recent conversations (for
// unread calculation), and active DM runs for one bot.
type BotSidebarSummary struct {
	Agent                string                `json:"agent"`
	LatestConversationID string                `json:"latest_conversation_id,omitempty"`
	LatestTitle          string                `json:"latest_title,omitempty"`
	LatestPreview        string                `json:"latest_preview,omitempty"`
	LatestUpdatedAt      string                `json:"latest_updated_at,omitempty"`
	LatestReadAt         *string               `json:"latest_read_at,omitempty"`
	RecentConversations  []ConversationSummary `json:"recent_conversations,omitempty"`
	ActiveRuns           []BotRunSummary       `json:"active_runs,omitempty"`
}

const botSummaryRunTimeout = 2 * time.Second

// SummarizeBots collects sidebar previews, recent unread candidates, and active
// DM runs for all provided agents in parallel so the dashboard can hydrate the
// Bots sidebar in a single HTTP request.
func SummarizeBots(
	ctx context.Context,
	conversations *ConversationService,
	agents []AgentInfo,
	userID int,
) []BotSidebarSummary {
	out := make([]BotSidebarSummary, len(agents))
	if len(agents) == 0 {
		return out
	}

	httpClient := &http.Client{Timeout: botSummaryRunTimeout}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	for i, agent := range agents {
		wg.Add(1)
		go func(idx int, info AgentInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			summary := BotSidebarSummary{Agent: info.Name}

			if conversations != nil {
				page, err := conversations.List(info.Name, userID, 10, 0, "", ConversationFilter{})
				if err == nil && len(page.Conversations) > 0 {
					summary.RecentConversations = page.Conversations
					top := page.Conversations[0]
					summary.LatestConversationID = top.ID
					summary.LatestTitle = top.Title
					if !top.UpdatedAt.IsZero() {
						summary.LatestUpdatedAt = top.UpdatedAt.UTC().Format(time.RFC3339)
					}
					if top.ReadAt != nil && !top.ReadAt.IsZero() {
						formatted := top.ReadAt.UTC().Format(time.RFC3339)
						summary.LatestReadAt = &formatted
					}

					if detail, derr := conversations.Get(info.Name, top.ID, userID, 12, -1); derr == nil {
						if detail.Title != "" {
							summary.LatestTitle = detail.Title
						}
						if preview := lastAssistantPreview(detail.Messages); preview != "" {
							summary.LatestPreview = preview
						}
					}
				}
			}

			if info.Running && info.BaseURL != "" {
				summary.ActiveRuns = fetchBotActiveRuns(ctx, httpClient, info.BaseURL)
			}

			out[idx] = summary
		}(i, agent)
	}

	wg.Wait()
	return out
}

func lastAssistantPreview(messages []provider.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "assistant" {
			continue
		}
		if text := strings.TrimSpace(messageContentString(msg.Content)); text != "" {
			return text
		}
	}
	return ""
}

func messageContentString(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if t, ok := m["text"].(string); ok && strings.TrimSpace(t) != "" {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func fetchBotActiveRuns(ctx context.Context, client *http.Client, baseURL string) []BotRunSummary {
	reqCtx, cancel := context.WithTimeout(ctx, botSummaryRunTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/chat/runs", nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var payload struct {
		Runs []BotRunSummary `json:"runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}
	active := make([]BotRunSummary, 0, len(payload.Runs))
	for _, run := range payload.Runs {
		if !run.Streaming || run.RequestID == "" {
			continue
		}
		if strings.HasPrefix(run.Scope, "group:") {
			continue
		}
		active = append(active, run)
	}
	return active
}
