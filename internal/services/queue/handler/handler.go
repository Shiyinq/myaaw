package handler

import (
	_ "teo/internal/common"
	"teo/internal/pkg"
	"teo/internal/services/queue/service"
	"teo/internal/utils"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
)

type QueueHandler interface {
	HandleTelegramChat(c *fiber.Ctx) error
}

type QueueHandlerImpl struct {
	queueService service.QueueService
}

func NewQueueHandler(queueService service.QueueService) QueueHandler {
	return &QueueHandlerImpl{queueService: queueService}
}

// Queue
// @Summary		Queue
// @Description	To receive incoming message from Telegram and push to Queue
// @Tags		Bot
// @Produce		json
// @Accept		json
// @Param		book	body		pkg.TelegramIncommingChat true	"Telegram incoming chat"
// @Failure     400    	{object}   	common.ErrorResponse
// @Failure     401     {object}    common.ErrorResponse
// @Failure     500     {object}    common.ErrorResponse
// @Router		/webhook/telegram [post]
func (h *QueueHandlerImpl) HandleTelegramChat(c *fiber.Ctx) error {
	var msg *pkg.TelegramIncommingChat

	if err := c.BodyParser(&msg); err != nil {
		return utils.ErrorBadRequest(c, "Failed to parse message")
	}

	err := h.queueService.ProcessAndPublishMessage(msg)
	if err != nil {
		return utils.ErrorInternalServer(c, "Failed to publish message")
	}

	return c.Status(fiber.StatusCreated).JSON(bson.M{
		"message": "Message published successfully",
	})
}
