package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/services/queue/service"
	"time"

	"github.com/go-resty/resty/v2"
)

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

	client := resty.New()

	// --- CRITICAL: Delete Webhook before polling ---
	// Telegram doesn't allow getUpdates if a webhook is set.
	if err := deleteWebhook(client); err != nil {
		log.Printf("⚠️  Warning: Failed to clear Telegram webhook: %v. Polling might fail.", err)
	} else {
		log.Println("✅ Telegram webhook cleared (required for Long Polling).")
	}

	offset := int64(0)

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping Telegram Long Polling...")
			return
		default:
			updates, err := fetchUpdates(client, offset)
			if err != nil {
				log.Printf("⚠️  Telegram Polling Error: %v. Retrying in 5s...", err)
				time.Sleep(5 * time.Second)
				continue
			}

			for _, rawUpdate := range updates {
				var u struct {
					UpdateId int64 `json:"update_id"`
				}
				if err := json.Unmarshal(rawUpdate, &u); err != nil {
					log.Printf("⚠️  Failed to parse update_id: %v", err)
					continue
				}

				processUpdate(rawUpdate, q)
				offset = u.UpdateId + 1
			}
		}
	}
}

func fetchUpdates(client *resty.Client, offset int64) ([]json.RawMessage, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates", config.TelegramBotToken)

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
	url := fmt.Sprintf("https://api.telegram.org/bot%s/deleteWebhook", config.TelegramBotToken)
	var resp TelegramBaseResponse
	_, err := client.R().
		SetResult(&resp).
		Get(url)

	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("API error (%d): %s", resp.ErrorCode, resp.Description)
	}
	return nil
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
