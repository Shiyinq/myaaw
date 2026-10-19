package handler

import (
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

type Handler struct {
	auth          *service.AuthService
	agents        *service.AgentService
	proxy         *service.ProxyService
	files         *service.FileService
	config        *service.ConfigService
	conversations *service.ConversationService
	logs          *service.LogsService
	server        *service.ServerService
	cron          *service.CronService
}

func New(
	auth *service.AuthService,
	agents *service.AgentService,
	proxy *service.ProxyService,
	files *service.FileService,
	configSvc *service.ConfigService,
	conversations *service.ConversationService,
	logs *service.LogsService,
	server *service.ServerService,
	cronSvc *service.CronService,
) *Handler {
	return &Handler{
		auth:          auth,
		agents:        agents,
		proxy:         proxy,
		files:         files,
		config:        configSvc,
		conversations: conversations,
		logs:          logs,
		server:        server,
		cron:          cronSvc,
	}
}

// Health reports whether the control plane is accepting requests.
// @Summary      Health check
// @Tags         System
// @Produce      json
// @Success      200 {object} HealthResponse
// @Router       /health [get]
func (h *Handler) Health(c fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}
