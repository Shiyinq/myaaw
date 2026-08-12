package heartbeat

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type HeartbeatService struct {
	client     *resty.Client
	stopChan   chan struct{}
	reloadChan chan struct{}
}

type HeartbeatConfig struct {
	Heartbeat struct {
		Active  bool   `json:"active"`
		Every   string `json:"every"`
		To      string `json:"to"`
		Channel string `json:"channel"`
	} `json:"heartbeat"`
}

func NewHeartbeatService() *HeartbeatService {
	return &HeartbeatService{
		client:     resty.New(),
		stopChan:   make(chan struct{}),
		reloadChan: make(chan struct{}),
	}
}

func (s *HeartbeatService) Start() {
	go s.runLoop()
}

func (s *HeartbeatService) Stop() {
	close(s.stopChan)
}

func (s *HeartbeatService) ReloadConfig() {
	// Signal runLoop to reload
	select {
	case s.reloadChan <- struct{}{}:
	default:
		// If channel is full (e.g. reload happening), skip
	}
}

func (s *HeartbeatService) runLoop() {
	baseURL := os.Getenv("MYAAW_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	for {
		// 1. Load Config & Determine Interval
		interval := 30 * time.Minute
		config, err := s.readConfig()
		if err == nil && config != nil && config.Heartbeat.Every != "" {
			parsed, parseErr := time.ParseDuration(config.Heartbeat.Every)
			if parseErr == nil {
				interval = parsed
			}
		}

		log.Printf("Heartbeat scheduler active. Interval: %s", interval)

		// 2. Create Ticker
		ticker := time.NewTicker(interval)

		// 3. Wait for Tick, Reload, or Stop
		select {
		case <-ticker.C:
			// Time to trigger
			log.Println("Triggering heartbeat...")
			if err := s.process(); err != nil {
				log.Printf("Heartbeat trigger failed: %v", err)
			}
			ticker.Stop() // Stop current ticker to loop and potentially reload config/interval next time?
			// Actually, if we want strict interval, we stay in loop.
			// But if we want hot reload support, breaking the loop and restarting ticker is easier
			// OR we can select on reloadChan inside the ticker loop.
		case <-s.reloadChan:
			log.Println("Heartbeat config reloading...")
			ticker.Stop()
			// Loop restarts, reading new config and setting new interval
			continue
		case <-s.stopChan:
			ticker.Stop()
			log.Println("Heartbeat scheduler stopped.")
			return
		}
	}
}

func (s *HeartbeatService) process() error {
	config, err := s.readConfig()
	if err != nil {
		return err
	}

	if config != nil {
		log.Printf("Debug: Config loaded. Active: %v, To: '%s', Channel: '%s'", config.Heartbeat.Active, config.Heartbeat.To, config.Heartbeat.Channel)
	}

	if config == nil || !config.Heartbeat.Active {
		log.Println("Heartbeat is inactive in config.json, skipping.")
		return nil
	}

	prompt := `SYSTEM: You are in HEARTBEAT checking mode.

INSTRUCTION:
1. Read the file "HEARTBEAT.md".
2. Check the items listed in that file.

DECISION LOGIC:
A. If you need to perform actions or send a message to the user:
   - Perform the actions using tools.
   - Do NOT output "HEARTBEAT_OK".

B. If everything is fine and NO action is needed:
   - Reply ONLY with the exact string "HEARTBEAT_OK".
   - Do not output anything else.

CRITICAL: "HEARTBEAT_OK" acts as a "silent skip". If you output it, the system assumes you did nothing. If you did something, DO NOT output it.`

	payload := map[string]interface{}{
		"prompt":  prompt,
		"to":      config.Heartbeat.To,
		"channel": config.Heartbeat.Channel,
		"trigger": "heartbeat",
	}

	baseURL := os.Getenv("MYAAW_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	resp, err := s.client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(fmt.Sprintf("%s/heartbeat", baseURL))

	if err != nil {
		return fmt.Errorf("failed to send heartbeat request: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("heartbeat endpoint returned error: %s", resp.Status())
	}

	log.Println("Heartbeat triggered successfully.")
	return nil
}

func (s *HeartbeatService) readConfig() (*HeartbeatConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configPath := strings.Join([]string{homeDir, ".myaaw", "config", "config.json"}, string(os.PathSeparator))

	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("config.json not found, skipping heartbeat.")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read config.json: %w", err)
	}

	var config HeartbeatConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config.json: %w", err)
	}

	return &config, nil
}
