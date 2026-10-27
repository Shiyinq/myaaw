package handler

import (
	"errors"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// DeleteAgent deletes an agent and all its data.
// @Summary      Delete agent
// @Tags         Agents
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name} [delete]
func (h *Handler) DeleteAgent(c fiber.Ctx) error {
	if err := h.agents.Delete(c.Params("name")); err != nil {
		return agentWriteError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// SetDefaultAgent marks an agent as the registry default.
// @Summary      Set default agent
// @Tags         Agents
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/default [post]
func (h *Handler) SetDefaultAgent(c fiber.Ctx) error {
	if err := h.agents.SetDefault(c.Params("name")); err != nil {
		return agentWriteError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func agentWriteError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrInvalidAgentName), errors.Is(err, service.ErrDefaultAgent):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
