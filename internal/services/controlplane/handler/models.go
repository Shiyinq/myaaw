package handler

import (
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListModels lists models for a provider that is not saved yet. It is not
// scoped to an agent, so the create-agent wizard can fill its model picker
// before calling onboard.
// @Summary      List models for a provider
// @Tags         Providers
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body service.ModelLookupInput true "Provider credentials"
// @Success      200 {object} service.ModelListView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /providers/models [post]
func (h *Handler) ListModels(c fiber.Ctx) error {
	var in service.ModelLookupInput
	if err := c.Bind().JSON(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}

	view, err := h.config.ModelsForCredentials(in)
	if err != nil {
		return configWriteError(c, err)
	}
	return c.JSON(view)
}

// GetProviderModels lists models for an already saved provider, using the
// stored API key.
// @Summary      List models for a saved provider
// @Tags         Providers
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Provider name"
// @Success      200 {object} service.ModelListView
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/providers/{id}/models [get]
func (h *Handler) GetProviderModels(c fiber.Ctx) error {
	view, err := h.config.ProviderModels(c.Params("name"), c.Params("id"))
	if err != nil {
		return configWriteError(c, err)
	}
	return c.JSON(view)
}
