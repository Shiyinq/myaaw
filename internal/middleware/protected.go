package middleware

import (
	"fmt"
	"strconv"
	"teo/internal/config"
	"teo/internal/services/bot/model"
	"teo/internal/utils"

	"github.com/gofiber/fiber/v2"
)

func Protected(c *fiber.Ctx) error {
	data := new(model.TelegramIncommingChat)

	if err := c.BodyParser(&data); err != nil {
		return c.Next()
	}

	if config.BotType == "private" {
		owner, err := strconv.Atoi(config.OwnerId)
		if err != nil {
			fmt.Println("Invalid owner id")
			return utils.ErrorBadRequest(c, "Invalid owner id")
		}

		if data.Message.From.Id != owner {
			fmt.Println("Only the owner is allowed to chat")
			return utils.ErrorBadRequest(c, "Only the owner is allowed to chat")
		}
	}

	return c.Next()
}
