package bot_router

import (
	"teo/internal/config"
	"teo/internal/services/bot/handler"
	"teo/internal/services/bot/repository"
	"teo/internal/services/bot/service"

	"github.com/gofiber/fiber/v2"
)

func BotRouter(router fiber.Router) {

	repo := repository.NewBotRepository(config.DB)
	serv := service.NewBotService(repo)
	hand := handler.NewBotHandler(serv)

	router.Post("/webhook", hand.Webhook)
}
