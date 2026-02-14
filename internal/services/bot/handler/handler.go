package handler

import (
	"fmt"
	"log"
	_ "myaaw/internal/common"
	"myaaw/internal/pkg"
	"myaaw/internal/services/bot/service"
	"myaaw/internal/utils"

	"github.com/gofiber/fiber/v2"
)

type BookHandler interface {
	Webhook(c *fiber.Ctx) error
	Heartbeat(c *fiber.Ctx) error
}

type BotHandlerImpl struct {
	botService service.BotService
}

type HeartbeatRequest struct {
	Prompt  string `json:"prompt"`
	To      string `json:"to"`
	Channel string `json:"channel"`
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
// @Param		book	body		pkg.TelegramIncommingChat true	"Telegram incoming chat"
// @Success		200		{object}	pkg.TelegramSendMessageStatus
// @Failure     400    	{object}   	common.ErrorResponse
// @Failure     401     {object}    common.ErrorResponse
// @Failure     500     {object}    common.ErrorResponse
// @Router		/webhook/bot [post]
func (s *BotHandlerImpl) Webhook(c *fiber.Ctx) error {
	data := new(pkg.TelegramIncommingChat)

	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid JSON",
		})
	}

	// jsonData, err := json.MarshalIndent(data, "", "  ")
	// if err != nil {
	// 	fmt.Println("Error marshalling JSON:", err)
	// } else {
	// 	fmt.Println(string(jsonData))
	// }

	log.Printf("Received message from user ID %v", data.Message.Chat.Id)

	res, err := s.botService.Bot(data)
	if err != nil {
		log.Printf("Failed to process incoming chat from user ID %v: %v", data.Message.Chat.Id, err.Error())

		formattedError := utils.FormatErrorMessage(err)
		s.botService.NotifyError(data.Message.Chat.Id, 0, fmt.Sprintf("❌ Something went wrong\n\n```JSON\n%v```", formattedError), true)

		return utils.ErrorInternalServer(c, "failed to process incoming chat: "+err.Error())
	}

	log.Printf("Successfully processed incoming chat from user ID %v", data.Message.Chat.Id)

	return c.Status(fiber.StatusCreated).JSON(res)
}

// Heartbeat
// @Summary		Heartbeat
// @Description	To process heartbeat request from consumer
// @Tags		Bot
// @Produce		json
// @Accept		json
// @Param		request	body		HeartbeatRequest	true	"Heartbeat request"
// @Success		200
// @Failure     400    	{object}   	common.ErrorResponse
// @Failure     500     {object}    common.ErrorResponse
// @Router		/heartbeat [post]
func (s *BotHandlerImpl) Heartbeat(c *fiber.Ctx) error {
	var req HeartbeatRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Heartbeat 400: Invalid JSON: %v. Raw Body: %s", err, c.Body())
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid JSON",
		})
	}

	if req.Prompt == "" {
		log.Printf("Heartbeat 400: Empty Prompt")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Prompt is required",
		})
	}

	err := s.botService.ProcessHeartbeat(req.Prompt, req.To, req.Channel)
	if err != nil {
		return utils.ErrorInternalServer(c, "failed to process heartbeat: "+err.Error())
	}
	return c.SendStatus(fiber.StatusOK)
}
