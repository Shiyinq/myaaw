package handler

import (
	"encoding/json"
	"fmt"
	_ "teo/internal/common"
	"teo/internal/services/bot/model"
	"teo/internal/services/bot/service"
	"teo/internal/utils"

	"github.com/gofiber/fiber/v2"
)

type BookHandler interface {
	Webhook(c *fiber.Ctx) error
}

type BotHandlerImpl struct {
	botService service.BotService
}

func NewBotHandler(botService service.BotService) BookHandler {
	return &BotHandlerImpl{botService: botService}
}

// Bot
// @Summary		Bot
// @Description	To receive incoming message from RabbitMQ consumer
// @Tags		Bot
// @Produce		json
// @Accept		json
// @Param		book	body		model.TelegramIncommingChat true	"Telegram incoming chat"
// @Success		200		{object}	model.TelegramSendMessageStatus
// @Failure     400    	{object}   	common.ErrorResponse
// @Failure     401     {object}    common.ErrorResponse
// @Failure     500     {object}    common.ErrorResponse
// @Router		/webhook/bot [post]
func (s *BotHandlerImpl) Webhook(c *fiber.Ctx) error {
	data := new(model.TelegramIncommingChat)

	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid JSON",
		})
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
	} else {
		fmt.Println(string(jsonData))
	}

	res, err := s.botService.Bot(data)

	if err != nil {
		return utils.ErrorInternalServer(c, "failed to process incoming chat: "+err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(res)
}
