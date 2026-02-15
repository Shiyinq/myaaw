package routes

import (
	bot_router "myaaw/internal/services/bot"
	queue_router "myaaw/internal/services/queue"

	"github.com/gofiber/fiber/v2"
)

func SetupRoutes(app *fiber.App) {
	prefix := ""
	router := app.Group(prefix)

	bot_router.BotRouter(router)
	queue_router.QueueRouter(router)
}
