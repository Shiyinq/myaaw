package main

import (
	routes "teo/internal"

	"teo/internal/config"
	"teo/internal/middleware"

	_ "teo/docs/swagger"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
)

// @title		TEO API
// @version		1.0
// @description TEO - Telegram Ollama Integration

// @host		localhost:8080
// @BasePath	/
func main() {
	config.LoadConfig()

	app := fiber.New(fiber.Config{
		EnablePrintRoutes: false,
	})

	app.Use(middleware.SetupCORS())

	app.Use(middleware.NewLogger())

	app.Get("/docs/*", swagger.HandlerDefault)
	routes.SetupRoutes(app)

	app.Use(middleware.NotFoundHandler)

	app.Listen(config.PORT)
}
