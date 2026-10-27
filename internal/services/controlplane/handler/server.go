package handler

import (
	"errors"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// GetServerStatus returns the agent server state.
// @Summary      Server status
// @Tags         Server
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} service.ServerStatus
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/server/status [get]
func (h *Handler) GetServerStatus(c fiber.Ctx) error {
	status, err := h.server.Status(c.Params("name"))
	if err != nil {
		return serverError(c, err)
	}
	return c.JSON(status)
}

// StartServer starts the agent server in the background.
// @Summary      Start server
// @Tags         Server
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      409 {object} common.ErrorResponse
// @Router       /agents/{name}/server/start [post]
func (h *Handler) StartServer(c fiber.Ctx) error {
	if err := h.server.Start(c.Params("name")); err != nil {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// StopServer stops the agent server.
// @Summary      Stop server
// @Tags         Server
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/server/stop [post]
func (h *Handler) StopServer(c fiber.Ctx) error {
	if err := h.server.Stop(c.Params("name")); err != nil {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// RestartServer restarts the agent server.
// @Summary      Restart server
// @Tags         Server
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/server/restart [post]
func (h *Handler) RestartServer(c fiber.Ctx) error {
	if err := h.server.Restart(c.Params("name")); err != nil {
		return serverError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func serverError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrServerRunning):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
