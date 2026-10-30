package middleware

import (
	"context"
	"testing"

	"myaaw/internal/config"
)

func TestConfigureTelegramWebhookInvalidFlagReturnsError(t *testing.T) {
	old := config.NgrokActive
	config.NgrokActive = "definitely-not-a-bool"
	t.Cleanup(func() { config.NgrokActive = old })

	if err := configureTelegramWebhook(context.Background()); err == nil {
		t.Fatal("expected an error for an invalid NGROK_ACTIVE value")
	}
}

func TestConfigureTelegramWebhookDisabledIsNoop(t *testing.T) {
	old := config.NgrokActive
	config.NgrokActive = "false"
	t.Cleanup(func() { config.NgrokActive = old })

	if err := configureTelegramWebhook(context.Background()); err != nil {
		t.Fatalf("disabled ngrok should be a no-op, got %v", err)
	}
}
