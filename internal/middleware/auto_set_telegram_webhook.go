package middleware

import (
	"context"
	"errors"
	"fmt"
	"log"
	"myaaw/internal/config"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.ngrok.com/ngrok"
	cfNgrok "golang.ngrok.com/ngrok/config"
)

func ngrokForwarder(ctx context.Context) (ngrok.Forwarder, error) {
	backendUrl, err := url.Parse(fmt.Sprintf("%s%s", config.HOST, config.PORT))
	if err != nil {
		return nil, err
	}

	authToken := config.NgrokAuthToken
	if authToken == "" {
		return nil, errors.New("ngrok auth token required")
	}

	return ngrok.ListenAndForward(ctx,
		backendUrl,
		cfNgrok.HTTPEndpoint(),
		ngrok.WithAuthtoken(authToken),
	)
}

// SetTelegramWebhook configures the Telegram webhook through ngrok when it is
// enabled. A failure is logged, not fatal: an outage at boot must not crash-loop
// the agent through its supervisor.
func SetTelegramWebhook() {
	if err := configureTelegramWebhook(context.Background()); err != nil {
		log.Printf("⚠️  Telegram webhook was not configured: %v", err)
	}
}

func configureTelegramWebhook(ctx context.Context) error {
	enabled, err := strconv.ParseBool(config.NgrokActive)
	if err != nil {
		return fmt.Errorf("invalid NGROK_ACTIVE value %q: %w", config.NgrokActive, err)
	}
	if !enabled {
		return nil
	}
	if config.TelegramBotToken == "" {
		return errors.New("telegram bot token is required when ngrok is active")
	}

	forwarder, err := ngrokForwarder(ctx)
	if err != nil {
		return fmt.Errorf("start ngrok forwarder: %w", err)
	}
	url := forwarder.URL()

	trimmedUrl := strings.TrimFunc(url, func(r rune) bool { return r == '/' })
	webhookUrl := fmt.Sprintf("%s/bot%s/setWebhook?url=%s/webhook/telegram",
		"https://api.telegram.org", config.TelegramBotToken, trimmedUrl)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(webhookUrl)
	if err != nil {
		return fmt.Errorf("set telegram webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("set telegram webhook: %s", resp.Status)
	}

	log.Printf("Connected to Telegram!")
	log.Printf("Ngrok URL: %s\n", url)
	return nil
}
