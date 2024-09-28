package middleware

import "github.com/gofiber/fiber/v2"

func NotFoundHandler(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
		"error":   "Not Found",
		"message": "The requested endpoint does not exist",
	})
}
