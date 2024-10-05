package handler

import (
	"teo/internal/services/queue/model"
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

func (h *QueueHandlerImpl) HandleTelegramChat(c *fiber.Ctx) error {
	var msg *model.TelegramIncommingChat

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
