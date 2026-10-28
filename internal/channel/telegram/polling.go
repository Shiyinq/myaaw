package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	"myaaw/internal/services/queue/service"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// telegramPollTimeout must exceed the long-poll timeout sent to getUpdates.
const telegramPollTimeout = 70 * time.Second

type TelegramBaseResponse struct {
	Ok          bool   `json:"ok"`
	Result      bool   `json:"result"`
	Description string `json:"description,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
}

type TelegramUpdatesResponse struct {
	Ok          bool              `json:"ok"`
	Result      []json.RawMessage `json:"result"`
	Description string            `json:"description,omitempty"`
	ErrorCode   int               `json:"error_code,omitempty"`
}

func StartLongPolling(ctx context.Context, q service.QueueService) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🔥 Telegram Polling Panicked: %v. Restarting in 10s...", r)
			time.Sleep(10 * time.Second)
			StartLongPolling(ctx, q)
		}
	}()

	log.Println("ℹ️  Telegram Mode: Long Polling (No Ngrok required)")

	client := resty.New().SetTimeout(telegramPollTimeout)

	// --- CRITICAL: Delete Webhook before polling ---
	// Telegram doesn't allow getUpdates if a webhook is set.
	if err := deleteWebhook(client); err != nil {
		log.Printf("⚠️  Warning: Failed to clear Telegram webhook: %v. Polling might fail.", err)
	} else {
		log.Println("✅ Telegram webhook cleared (required for Long Polling).")
	}

	store := newOffsetStore()
	offset := store.Load()
	if offset > 0 {
		log.Printf("ℹ️  Resuming Telegram polling at offset %d", offset)
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping Telegram Long Polling...")
			return
		default:
			offset = pollUpdates(client, offset, store, q)
		}
	}
}

// pollUpdates fetches one batch of updates, enqueues them, and returns the new
// offset. The offset is persisted after every handled update so a restart does
// not replay the whole backlog.
func pollUpdates(client *resty.Client, offset int64, store *offsetStore, q service.QueueService) int64 {
	updates, err := fetchUpdates(client, offset)
	if err != nil {
		log.Printf("⚠️  Telegram Polling Error: %v. Retrying in 5s...", err)
		time.Sleep(5 * time.Second)
		return offset
	}

	for _, rawUpdate := range updates {
		var u struct {
			UpdateId int64 `json:"update_id"`
		}
		if err := json.Unmarshal(rawUpdate, &u); err != nil {
			log.Printf("⚠️  Failed to parse update_id: %v", err)
			continue
		}
		if u.UpdateId < offset {
			// Telegram re-delivers updates after a restart; skip what we handled.
			continue
		}

		processUpdate(rawUpdate, q)
		offset = u.UpdateId + 1
		store.Save(offset)
	}
	return offset
}

// offsetStore persists the long-polling offset so restarts resume instead of
// replaying updates Telegram still has queued.
type offsetStore struct {
	path string
}

func newOffsetStore() *offsetStore {
	path, err := agentdir.TelegramOffsetPath(agentdir.Active())
	if err != nil {
		return &offsetStore{}
	}
	return &offsetStore{path: path}
}

// Load returns the stored offset, or 0 when it is missing or unreadable.
func (s *offsetStore) Load() int64 {
	if s == nil || s.path == "" {
		return 0
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return 0
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

// Save writes the offset atomically; failures are logged, never fatal.
func (s *offsetStore) Save(offset int64) {
	if s == nil || s.path == "" || offset <= 0 {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		log.Printf("⚠️  Failed to create Telegram state directory: %v", err)
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.FormatInt(offset, 10)), 0o644); err != nil {
		log.Printf("⚠️  Failed to write Telegram offset: %v", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		log.Printf("⚠️  Failed to replace Telegram offset: %v", err)
		_ = os.Remove(tmp)
	}
}

func fetchUpdates(client *resty.Client, offset int64) ([]json.RawMessage, error) {
	url := telegramAPIURL("getUpdates")

	var resp TelegramUpdatesResponse
	_, err := client.R().
		SetQueryParams(map[string]string{
			"offset":  fmt.Sprintf("%d", offset),
			"timeout": "30",
		}).
		SetResult(&resp).
		Get(url)

	if err != nil {
		return nil, err
	}

	if !resp.Ok {
		return nil, fmt.Errorf("telegram API error (%d): %s", resp.ErrorCode, resp.Description)
	}

	return resp.Result, nil
}

func deleteWebhook(client *resty.Client) error {
	url := telegramAPIURL("deleteWebhook")
	var resp TelegramBaseResponse
	var err error

	for i := 1; i <= 3; i++ {
		_, err = client.R().
			SetResult(&resp).
			Get(url)

		if err == nil {
			if !resp.Ok {
				return fmt.Errorf("API error (%d): %s", resp.ErrorCode, resp.Description)
			}
			return nil
		}

		log.Printf("⚠️  Attempt %d: Failed to delete Telegram webhook: %v. Retrying in 2s...", i, err)
		time.Sleep(2 * time.Second)
	}

	return err
}

func processUpdate(payload json.RawMessage, q service.QueueService) {
	// Send the FULL update object to mirror Webhook behavior
	envelope := channel.QueueEnvelope{
		Channel: "telegram",
		Payload: payload,
	}

	err := q.ProcessAndPublishMessage(&envelope)
	if err != nil {
		log.Printf("❌ Failed to push polled message to RabbitMQ: %v", err)
	}
}
