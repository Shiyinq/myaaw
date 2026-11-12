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
			// Media elements cannot send a header; accept a valid per-path signature
			// instead of a bearer token for GET /fs/raw.
			if mediaSignatureAllowed(c, auth) {
				return c.Next()
			}
			if isRawMediaPath(c) && (c.Query("exp") != "" || c.Query("sig") != "") {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": "invalid or expired media signature",
				})
			}
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing bearer token",
			})
		}
		if token != "" && auth.Validate(token) {
			return c.Next()
		}
		if mediaSignatureAllowed(c, auth) {
			return c.Next()
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "invalid or expired token",
		})
	}
}

// isRawMediaPath reports whether the request targets GET /agents/:name/fs/raw.
func isRawMediaPath(c fiber.Ctx) bool {
	if c.Method() != fiber.MethodGet {
		return false
	}
	parts := strings.Split(strings.Trim(c.Path(), "/"), "/")
	return len(parts) == 4 && parts[0] == "agents" && parts[2] == "fs" && parts[3] == "raw"
}

// mediaSignatureAllowed accepts a per-path signature on GET
// /agents/:name/fs/raw, so media elements can load without the session token.
func mediaSignatureAllowed(c fiber.Ctx, auth *service.AuthService) bool {
	if !isRawMediaPath(c) {
		return false
	}
	parts := strings.Split(strings.Trim(c.Path(), "/"), "/")
	return auth.ValidateMediaSignature(parts[1], c.Query("path"), c.Query("exp"), c.Query("sig"))
}

func BearerToken(c fiber.Ctx) string {
	header := strings.TrimSpace(c.Get("Authorization"))
	if header == "" {
		// Subresources (<img>, <video>, <iframe>, <a download>) cannot send an
		// Authorization header, so the file endpoints accept the session token as
		// a query parameter. The token is a 24-hour local session, and these
		// requests never become history entries.
		return strings.TrimSpace(c.Query("token"))
	}

	// Swagger UI's apiKey auth sends the raw value. Accept both forms so the
	// same token works with "Bearer <token>" and with the plain token.
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return header
}
