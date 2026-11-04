package main

import (
	"encoding/json"
	"fmt"
	"log"
	"myaaw/internal/config"
	"myaaw/internal/cron"
	"myaaw/internal/heartbeat"
	"myaaw/internal/services/queue/repository"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

func startBackgroundWorkers() {
	config.LoadConfig()

	var hb *heartbeat.HeartbeatService
	if config.Heartbeat.Active {
		hb = heartbeat.NewHeartbeatService()
		go hb.Start()
	}

	// Start Cron Scheduler & Watcher
	scheduler, err := cron.NewDefaultScheduler()
	if err != nil {
		log.Printf("Failed to initialize cron scheduler: %v", err)
	} else {
		if err := scheduler.Start(); err != nil {
			log.Printf("Failed to start cron scheduler: %v", err)
		}
		defer scheduler.Stop()
		// Start Hot-Reload Watcher for jobs.json
		go scheduler.Watch()
	}

	// Start Global Config Watcher for config.json
	config.WatchConfig(func() {
		log.Println("Global config changed. Reloading components...")

		// Reload Cron Scheduler config (Active/Inactive)
		if scheduler != nil {
			scheduler.ReloadConfig()
		}

		// Reload Heartbeat config (Interval/Active)
		if hb != nil {
			hb.ReloadConfig()
		}
	})

	consumeMessages()
}

func sendToWebhookBot(jsonBody []byte) error {
	client := resty.New()
	// The webhook handles a whole agent run before responding, so the request
	// timeout must outlive the run timeout; otherwise a healthy run looks like a
	// failed delivery and frees its worker slot early.
	timeout := time.Duration(config.AgentRunTimeoutSeconds+60) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	client.SetTimeout(timeout)

	baseURL := config.MYAAWBaseURL
	if baseURL == "" {
		baseURL = "http://localhost" + config.PORT
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(jsonBody).
		Post(fmt.Sprintf("%s/webhook/bot", baseURL))

	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("received error response from webhook: %s %v", resp.Status(), resp.String())
	}

	log.Println("Message forwarded to webhook bot successfully")
	return nil
}

func consumeMessages() {
	workers := config.QueueWorkers
	if workers < 1 {
		workers = 1
	}
	if workers > config.MaxQueueWorkers {
		workers = config.MaxQueueWorkers
	}

	dispatch := newKeyedDispatcher(workers, func(msg []byte) {
		if err := sendToWebhookBot(msg); err != nil {
			log.Printf("Failed to send message to webhook: %s", err)
		}
	})

	log.Printf("Waiting for messages from in-memory queue (%d worker(s), per-user order kept). To exit press CTRL+C", workers)

	for msg := range repository.WebhookQueue {
		log.Printf("Received message: %s", msg)
		dispatch.Submit(messageQueueKey(msg), msg)
	}
}

// messageQueueKey groups work by conversation target so messages that share a
// session are handled in order. Direct chats keep the per-user key; group chats
// (including forum topics) use the same scope as the conversation lookup, so a
// user's DM and a group topic never serialize each other.
func messageQueueKey(body []byte) string {
	var envelope struct {
		Channel string `json:"channel"`
		Payload struct {
			Message struct {
				MessageThreadID json.RawMessage `json:"message_thread_id"`
				Chat            struct {
					ID   json.RawMessage `json:"id"`
					Type string          `json:"type"`
				} `json:"chat"`
				From struct {
					ID json.Number `json:"id"`
				} `json:"from"`
			} `json:"message"`
			DiscordAuthor struct {
				ID string `json:"id"`
			} `json:"discord_author"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "unparsed"
	}

	if envelope.Channel == "telegram" {
		chatType := strings.ToLower(strings.TrimSpace(envelope.Payload.Message.Chat.Type))
		if chatType != "" && chatType != "private" {
			if key := telegramScopeKey(envelope.Payload.Message.Chat.ID, envelope.Payload.Message.MessageThreadID); key != "" {
				return "scope:" + key
			}
		}
	}

	if envelope.Channel == "discord" {
		chatType := strings.ToLower(strings.TrimSpace(envelope.Payload.Message.Chat.Type))
		if chatType != "" && chatType != "dm" {
			if channelID := rawIDString(envelope.Payload.Message.Chat.ID); channelID != "" {
				return "scope:dc:" + channelID
			}
		}
	}

	userID := strings.TrimSpace(envelope.Payload.Message.From.ID.String())
	if userID == "" || userID == "0" {
		userID = strings.TrimSpace(envelope.Payload.DiscordAuthor.ID)
	}
	if userID == "" {
		// Unknown sender: keep it serialized instead of losing ordering.
		return "channel:" + envelope.Channel
	}
	return "user:" + userID
}

// telegramScopeKey mirrors the conversation scope: threads 0 and 1 are the
// chat-level bucket (non-forum groups and the General topic).
func telegramScopeKey(chatID, threadID json.RawMessage) string {
	chat := rawIDString(chatID)
	if chat == "" || chat == "0" {
		return ""
	}
	thread, err := strconv.Atoi(rawIDString(threadID))
	if err != nil || thread <= 1 {
		return "tg:" + chat
	}
	return fmt.Sprintf("tg:%s:%d", chat, thread)
}

// rawIDString reads an ID that may be encoded as a JSON number or a string.
// Discord uses string channel IDs, Telegram uses numbers.
func rawIDString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if strings.HasPrefix(s, `"`) {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return ""
		}
		return value
	}
	return s
}
