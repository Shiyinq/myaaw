package queue_router

import (
	"teo/internal/config"
	"teo/internal/services/queue/handler"
	"teo/internal/services/queue/repository"
	"teo/internal/services/queue/service"

	"github.com/gofiber/fiber/v2"
)

func QueueRouter(router fiber.Router) {

	repo := repository.NewQueueRepository(config.MQ)
	serv := service.NewQueueService(repo)
	hand := handler.NewQueueHandler(serv)

	router.Post("/receive_messages", hand.HandleTelegramChat)
}
