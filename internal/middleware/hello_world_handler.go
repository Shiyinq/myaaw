package middleware

import "github.com/gofiber/fiber/v2"

func HelloWorldHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"message": "Hello World!",
	})
}
