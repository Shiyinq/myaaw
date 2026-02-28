package handler

import (
	"fmt"
	"log"
	"myaaw/internal/channel"
	"myaaw/internal/channel/discord"
	telegram "myaaw/internal/channel/telegram"
	_ "myaaw/internal/common"
	"myaaw/internal/config"
	"myaaw/internal/services/bot/service"
	"myaaw/internal/utils"
	"strings"

	"github.com/gofiber/fiber/v3"
)

func telegramMeta(userID int) telegram.TelegramMeta {
	return telegram.TelegramMeta{
		ChatID:    userID,
		MessageID: 0,
	}
}

type BotHandler interface {
	Webhook(c fiber.Ctx) error
	Heartbeat(c fiber.Ctx) error
	APIChat(c fiber.Ctx) error
	APIChatStream(c fiber.Ctx) error
}

type BotHandlerImpl struct {
	botService      service.BotService
	channelRegistry *channel.Registry
}

type HeartbeatRequest struct {
	Prompt  string `json:"prompt"`
	To      string `json:"to"`
	Channel string `json:"channel"`
}

func NewBotHandler(botService service.BotService, channelRegistry *channel.Registry) BotHandler {
	return &BotHandlerImpl{
		botService:      botService,
		channelRegistry: channelRegistry,
	}
}

// Webhook
// @Summary		Webhook
// @Description	To receive incoming message from RabbitMQ consumer (channel-agnostic)
// @Tags		Bot
// @Produce		json
// @Accept		json
// @Param		envelope	body		channel.QueueEnvelope	true	"Queue envelope with channel and payload"
// @Success		200
// @Failure     400    	{object}   	common.ErrorResponse
// @Failure     401     {object}    common.ErrorResponse
// @Failure     500     {object}    common.ErrorResponse
// @Router		/webhook/bot [post]
func (s *BotHandlerImpl) Webhook(c fiber.Ctx) error {
	var envelope channel.QueueEnvelope
	if err := c.Bind().JSON(&envelope); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid JSON",
		})
	}

	adapter := s.channelRegistry.Get(envelope.Channel)
	if adapter == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Unknown channel: %s", envelope.Channel),
		})
	}

	msg, err := adapter.ParseIncoming(envelope.Payload)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Failed to parse channel payload: " + err.Error(),
		})
	}

	log.Printf("Received message from user ID %v (channel: %s)", msg.UserID, msg.Channel)

	if !s.botService.IsAllowed(msg.UserID) {
		log.Printf("Access denied for user %d (channel: %s)", msg.UserID, msg.Channel)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"ok": true})
	}

	if config.StreamResponse {
		out, err := adapter.SendStream(msg, func(onChunk func(chunk channel.StreamChunk)) error {
			_, err := s.botService.BotStream(msg, onChunk)
			return err
		})
		if err != nil {
			log.Printf("Failed to process/stream chat from user ID %v: %v", msg.UserID, err.Error())
			formattedError := utils.FormatErrorMessage(err)
			adapter.SendError(msg, fmt.Sprintf("❌ Something went wrong\n\n```JSON\n%v```", formattedError))
			return utils.ErrorInternalServer(c, "failed to process incoming chat: "+err.Error())
		}
		_ = out
	} else {
		out, err := s.botService.Bot(msg)
		if err != nil {
			log.Printf("Failed to process incoming chat from user ID %v: %v", msg.UserID, err.Error())
			formattedError := utils.FormatErrorMessage(err)
			adapter.SendError(msg, fmt.Sprintf("❌ Something went wrong\n\n```JSON\n%v```", formattedError))
			return utils.ErrorInternalServer(c, "failed to process incoming chat: "+err.Error())
		}

		err = adapter.Send(msg, out)
		if err != nil {
			log.Printf("Failed to deliver response to user ID %v: %v", msg.UserID, err.Error())
			return utils.ErrorInternalServer(c, "failed to deliver response: "+err.Error())
		}
	}

	log.Printf("Successfully processed incoming chat from user ID %v", msg.UserID)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"ok": true,
	})
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
func (s *BotHandlerImpl) Heartbeat(c fiber.Ctx) error {
	var req HeartbeatRequest

	if err := c.Bind().JSON(&req); err != nil {
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

	msg, out, err := s.botService.ProcessHeartbeat(req.Prompt, req.To, req.Channel)
	if err != nil {
		return utils.ErrorInternalServer(c, "failed to process heartbeat: "+err.Error())
	}

	if out != nil && req.Channel != "" && strings.TrimSpace(out.Text) != "HEARTBEAT_OK" {
		adapter := s.channelRegistry.Get(req.Channel)
		if adapter != nil {
			if msg.RawMeta == nil {
				switch req.Channel {
				case "telegram":
					msg.RawMeta = telegramMeta(msg.UserID)
				case "discord":
					msg.RawMeta = discord.DiscordMeta{
						ChannelID: req.To,
					}
				}
			}
			if deliverErr := adapter.Send(msg, out); deliverErr != nil {
				log.Printf("Failed to deliver heartbeat response: %v", deliverErr)
			}
		}
	}

	return c.SendStatus(fiber.StatusOK)
}
