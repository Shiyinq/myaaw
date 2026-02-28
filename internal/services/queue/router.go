package queue_router

import (
	"myaaw/internal/config"
	"myaaw/internal/services/queue/handler"
	"myaaw/internal/services/queue/repository"
	"myaaw/internal/services/queue/service"

	"github.com/gofiber/fiber/v3"
)

func QueueRouter(router fiber.Router) {

	repo := repository.NewQueueRepository(config.MQ)
	serv := service.NewQueueService(repo)
	hand := handler.NewQueueHandler(serv)

	router.Post("/webhook/telegram", hand.HandleTelegramChat)
}
