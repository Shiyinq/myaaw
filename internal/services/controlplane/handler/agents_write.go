package handler

import (
	"errors"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// CreateAgent creates an isolated agent.
// @Summary      Create agent
// @Tags         Agents
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body CreateAgentRequest true "Agent name"
// @Success      201 {object} service.AgentInfo
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      409 {object} common.ErrorResponse
// @Router       /agents [post]
func (h *Handler) CreateAgent(c fiber.Ctx) error {
	var req CreateAgentRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}

	agent, err := h.agents.Create(req.Name)
	if err != nil {
		return agentWriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(agent)
}

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
	case errors.Is(err, service.ErrAgentExists):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrInvalidAgentName), errors.Is(err, service.ErrDefaultAgent):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
