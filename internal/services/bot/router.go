package bot_router

import (
	"teo/internal/config"
	"teo/internal/services/bot/handler"
	"teo/internal/services/bot/repository"
	"teo/internal/services/bot/service"

	"github.com/gofiber/fiber/v2"
)

func BotRouter(router fiber.Router) {

	userRepo := repository.NewUserRepository(config.DB, config.RedisClient)
	convRepo := repository.NewConversationRepository(config.DB)
	serv := service.NewBotService(userRepo, convRepo)
	hand := handler.NewBotHandler(serv)

	router.Post("/webhook/bot", hand.Webhook)
}
