package middleware

import (
	"strings"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// RequireAuth validates the bearer token issued by POST /auth/login.
func RequireAuth(auth *service.AuthService) fiber.Handler {
	return func(c fiber.Ctx) error {
		token := BearerToken(c)
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing bearer token",
			})
		}
		if !auth.Validate(token) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid or expired token",
			})
		}
		return c.Next()
	}
}

func BearerToken(c fiber.Ctx) string {
	const prefix = "Bearer "
	header := c.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}
