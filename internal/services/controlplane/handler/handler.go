package handler

import (
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

type Handler struct {
	auth   *service.AuthService
	agents *service.AgentService
	proxy  *service.ProxyService
	files  *service.FileService
}

func New(auth *service.AuthService, agents *service.AgentService, proxy *service.ProxyService, files *service.FileService) *Handler {
	return &Handler{
		auth:   auth,
		agents: agents,
		proxy:  proxy,
		files:  files,
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
