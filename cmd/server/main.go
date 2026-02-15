package main

import (
	"log"
	routes "myaaw/internal"

	"myaaw/internal/channel/discord"
	"myaaw/internal/config"
	"myaaw/internal/middleware"
	"myaaw/internal/services/queue/repository"

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

	// Start Discord listener if token is present
	if config.DiscordBotToken != "" {
		queueRepo := repository.NewQueueRepository(config.MQ)
		adapter, err := discord.NewDiscordAdapter(config.DiscordBotToken)
		if err != nil {
			log.Printf("Failed to create Discord adapter: %v", err)
		} else {
			go func() {
				if err := adapter.StartListener(queueRepo); err != nil {
					log.Printf("Discord listener error: %v", err)
				}
			}()
		}
	}

	app.Listen(config.PORT)
}
