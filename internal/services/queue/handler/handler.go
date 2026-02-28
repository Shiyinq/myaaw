package handler

import (
	"encoding/json"
	"myaaw/internal/channel"
	_ "myaaw/internal/common"
	"myaaw/internal/services/queue/service"
	"myaaw/internal/utils"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/bson"
)

type QueueHandler interface {
	HandleTelegramChat(c fiber.Ctx) error
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
// @Param		book	body		any	true	"Telegram incoming chat"
// @Failure     400    	{object}   	common.ErrorResponse
// @Failure     401     {object}    common.ErrorResponse
// @Failure     500     {object}    common.ErrorResponse
// @Router		/webhook/telegram [post]
func (h *QueueHandlerImpl) HandleTelegramChat(c fiber.Ctx) error {
	// Wrap raw Telegram payload in a QueueEnvelope
	rawPayload := json.RawMessage(c.Body())
	envelope := &channel.QueueEnvelope{
		Channel: "telegram",
		Payload: rawPayload,
	}

	err := h.queueService.ProcessAndPublishMessage(envelope)
	if err != nil {
		return utils.ErrorInternalServer(c, "Failed to publish message")
	}

	return c.Status(fiber.StatusCreated).JSON(bson.M{
		"message": "Message published successfully",
	})
}
