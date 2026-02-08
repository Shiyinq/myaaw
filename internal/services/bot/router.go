package bot_router

import (
	"myaaw/internal/config"
	"myaaw/internal/services/bot/handler"
	"myaaw/internal/services/bot/repository"
	"myaaw/internal/services/bot/service"

	"github.com/gofiber/fiber/v2"
)

func BotRouter(router fiber.Router) {

	userRepo := repository.NewUserRepository(config.DB, config.RedisClient)
	convRepo := repository.NewConversationRepository(config.DB, config.RedisClient)
	serv := service.NewBotService(userRepo, convRepo)
	hand := handler.NewBotHandler(serv)

	router.Post("/webhook/bot", hand.Webhook)
}
