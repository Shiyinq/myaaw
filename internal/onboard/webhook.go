package onboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RegisterTelegramWebhook points Telegram at https://<baseURL>/webhook/telegram.
// It returns an error so onboarding can report a failure without exiting.
func RegisterTelegramWebhook(botToken, baseURL string) error {
	trimmedURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmedURL == "" {
		return fmt.Errorf("webhook URL is required")
	}
	if !strings.HasPrefix(trimmedURL, "https://") {
		return fmt.Errorf("Telegram requires an https:// webhook URL")
	}

	webhookURL := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook?url=%s/webhook/telegram", botToken, trimmedURL)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(webhookURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err == nil && !result.OK {
		return fmt.Errorf("telegram error: %s", result.Description)
	}
	return nil
}
