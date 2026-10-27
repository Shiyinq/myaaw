package handler

import (
	"errors"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// OnboardAgent creates an agent when the name is free and reconfigures it
// otherwise, applying the whole configuration in one call. It reuses the same
// core as `myaaw onboard` / `agent create`.
// @Summary      Create or reconfigure an agent
// @Tags         Agents
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body service.OnboardRequest true "Agent configuration"
// @Success      201 {object} service.OnboardResult
// @Success      200 {object} service.OnboardResult
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /agents/{name}/onboard [post]
func (h *Handler) OnboardAgent(c fiber.Ctx) error {
	var req service.OnboardRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}

	result, err := h.agents.Onboard(c.Params("name"), req)
	if err != nil {
		return onboardError(c, err)
	}

	status := fiber.StatusOK
	if result.Created {
		status = fiber.StatusCreated
	}
	return c.Status(status).JSON(result)
}

func onboardError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidAgentName), errors.Is(err, service.ErrInvalidConfig):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
