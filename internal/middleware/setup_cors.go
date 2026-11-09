package middleware

import (
	"strings"

	"myaaw/internal/config"
	"myaaw/internal/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

// SetupCORS allows the local dashboards by default. A browser on another origin
// (a remote dashboard or a tunnel) must opt in through ALLOWED_ORIGINS; native
// desktop/mobile clients do not send an Origin and are unaffected.
func SetupCORS() fiber.Handler {
	return cors.New(corsConfig(config.AllowedOrigins))
}

func corsConfig(raw string) cors.Config {
	cfg := cors.Config{
		AllowMethods: []string{"GET", "POST", "HEAD", "PUT", "DELETE", "PATCH"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
	}

	switch raw = strings.TrimSpace(raw); {
	case raw == "*":
		cfg.AllowOrigins = []string{"*"}
	case raw != "":
		cfg.AllowOrigins = utils.ParseAllowedOrigins(raw)
	default:
		cfg.AllowOriginsFunc = utils.IsLoopbackOrigin
	}
	return cfg
}
