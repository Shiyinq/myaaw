package middleware

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"teo/internal/config"

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

func SetTelegramWebhook() {
	if ngrok, err := strconv.ParseBool(config.NgrokActive); err != nil {
		log.Fatalf("Error parsing boolean: %v", err)
	} else if !ngrok {
		return
	}

	ngrok, err := ngrokForwarder(context.Background())
	if err != nil {
		log.Fatalf("Failed to set Ngrok Forwarder: %v", err)
	}
	url := ngrok.URL()
	if config.BotToken == "" {
		log.Fatal("Failed to set Telegram webhook: bot token required")
	}

	trimmedUrl := strings.TrimFunc(url, func(r rune) bool { return r == '/' })
	webhookUrl := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook?url=%s/webhook/telegram", config.BotToken, trimmedUrl)
	resp, err := http.Get(webhookUrl)
	if err != nil {
		log.Fatalf("Error setting Telegram webhook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("Connected to Telegram!")
		log.Printf("Ngrok URL: %s\n", url)
	} else {
		log.Printf("Failed to set Telegram webhook: %s\n", resp.Status)
	}
}
