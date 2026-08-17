package main

import (
	"fmt"
	"log"
	"myaaw/internal/config"
	"myaaw/internal/cron"
	"myaaw/internal/heartbeat"
	"myaaw/internal/services/queue/repository"
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
	client.SetTimeout(120 * time.Second)

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
	log.Println("Waiting for messages from in-memory queue. To exit press CTRL+C")

	for msg := range repository.WebhookQueue {
		log.Printf("Received message: %s", msg)

		err := sendToWebhookBot(msg)
		if err != nil {
			log.Printf("Failed to send message to webhook: %s", err)
		}
	}
}
