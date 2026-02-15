package bot_router

import (
	"log"
	"myaaw/internal/channel"
	"myaaw/internal/channel/telegram"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/handler"
	"myaaw/internal/services/bot/repository"
	"myaaw/internal/services/bot/service"

	"github.com/gofiber/fiber/v2"
)

func BotRouter(router fiber.Router) {
	userRepo := repository.NewUserRepository(config.DB, config.RedisClient)
	convRepo := repository.NewConversationRepository(config.DB, config.RedisClient)
	serv := service.NewBotService(userRepo, convRepo)

	// Create channel registry and register adapters
	registry := channel.NewRegistry()

	// Create TTS provider for Telegram voice support
	var ttsProvider provider.TTSProvider
	ttsProvider, err := provider.CreateTTSProvider(config.TTSProviderName, config.TTSProviderAPIKey)
	if err != nil {
		log.Printf("Warning: TTS provider not available for Telegram adapter: %v", err)
	}

	telegramAdapter := telegram.NewTelegramAdapter(ttsProvider)
	registry.Register(telegramAdapter)

	hand := handler.NewBotHandler(serv, registry)

	router.Post("/webhook/bot", hand.Webhook)
	router.Post("/heartbeat", hand.Heartbeat)
}
