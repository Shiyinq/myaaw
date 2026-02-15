package main

import (
	routes "myaaw/internal"

	"myaaw/internal/config"
	"myaaw/internal/middleware"

	_ "myaaw/docs/swagger"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
)

// @title		Myaaw API
// @version		1.0
// @description Myaaw - Integrate your favorite LLM with a Telegram bot.

// @host		localhost:8080
// @BasePath	/
func main() {
	config.LoadConfig()

	app := fiber.New(fiber.Config{
		EnablePrintRoutes: false,
	})

	app.Use(middleware.SetupCORS())

	app.Use(middleware.NewLogger())

	app.Get("/", middleware.HelloWorldHandler)
	app.Get("/docs/*", swagger.HandlerDefault)
	routes.SetupRoutes(app)

	app.Use(middleware.NotFoundHandler)

	middleware.SetTelegramWebhook()

	app.Listen(config.PORT)
}
