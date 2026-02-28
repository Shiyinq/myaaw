package middleware

import (
	"myaaw/internal/config"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

func SetupCORS() fiber.Handler {
	allowOrigins := []string{"*"}
	if config.AllowedOrigins != "" && config.AllowedOrigins != "*" {
		allowOrigins = strings.Split(config.AllowedOrigins, ",")
		for i, v := range allowOrigins {
			allowOrigins[i] = strings.TrimSpace(v)
		}
	}

	return cors.New(cors.Config{
		AllowOrigins: allowOrigins,
		AllowMethods: []string{"GET", "POST", "HEAD", "PUT", "DELETE", "PATCH"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept"},
	})
}
