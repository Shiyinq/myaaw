package routes

import (
	bot_router "teo/internal/services/bot"

	"github.com/gofiber/fiber/v2"
)

func SetupRoutes(app *fiber.App) {
	prefix := ""
	router := app.Group(prefix)
	bot_router.BotRouter(router)
}
