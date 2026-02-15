package bot_router

import (
	"log"
	"myaaw/internal/channel"
	apiAdapter "myaaw/internal/channel/api"
	"myaaw/internal/channel/discord"
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

	registry := channel.NewRegistry()

	var ttsProvider provider.TTSProvider
	ttsProvider, err := provider.CreateTTSProvider(config.TTSProviderName, config.TTSProviderAPIKey)
	if err != nil {
		log.Printf("Warning: TTS provider not available for Telegram adapter: %v", err)
	}
	telegramAdapter := telegram.NewTelegramAdapter(ttsProvider)
	registry.Register(telegramAdapter)

	registry.Register(apiAdapter.NewAPIAdapter())

	if config.DiscordBotToken != "" {
		adapter, err := discord.NewDiscordAdapter(config.DiscordBotToken)
		if err != nil {
			log.Printf("Failed to init Discord adapter: %v", err)
		} else {
			registry.Register(adapter)
		}
	}

	hand := handler.NewBotHandler(serv, registry)

	router.Post("/webhook/bot", hand.Webhook)
	router.Post("/heartbeat", hand.Heartbeat)

	api := router.Group("/api")
	api.Post("/chat", hand.APIChat)
	api.Post("/chat/stream", hand.APIChatStream)
}
