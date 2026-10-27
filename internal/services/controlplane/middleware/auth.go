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
	header := strings.TrimSpace(c.Get("Authorization"))
	if header == "" {
		return ""
	}

	// Swagger UI's apiKey auth sends the raw value. Accept both forms so the
	// same token works with "Bearer <token>" and with the plain token.
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return header
}
