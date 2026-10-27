package handler

import (
	"errors"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListProviders returns the agent providers with secrets masked.
// @Summary      List providers
// @Tags         Providers
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} service.ProvidersView
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/providers [get]
func (h *Handler) ListProviders(c fiber.Ctx) error {
	view, err := h.config.Providers(c.Params("name"))
	if err != nil {
		return configError(c, err)
	}
	return c.JSON(view)
}

// ListChannels returns the agent channels with tokens masked.
// @Summary      List channels
// @Tags         Channels
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} service.ChannelsView
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/channels [get]
func (h *Handler) ListChannels(c fiber.Ctx) error {
	view, err := h.config.Channels(c.Params("name"))
	if err != nil {
		return configError(c, err)
	}
	return c.JSON(view)
}

// GetConfig returns the agent configuration with all secrets masked.
// @Summary      Get agent config
// @Tags         Config
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} service.ConfigView
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/config [get]
func (h *Handler) GetConfig(c fiber.Ctx) error {
	view, err := h.config.Config(c.Params("name"))
	if err != nil {
		return configError(c, err)
	}
	return c.JSON(view)
}

func configError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrConfigNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent config not found"})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
