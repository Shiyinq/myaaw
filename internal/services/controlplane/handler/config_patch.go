package handler

import (
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// PatchConfig applies a partial update to the agent config. Only the fields
// present in the request change; absent fields are left untouched and no key
// is ever removed.
// @Summary      Update agent config
// @Tags         Config
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body service.ConfigPatch true "Fields to update"
// @Success      200 {object} service.ConfigView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/config [patch]
func (h *Handler) PatchConfig(c fiber.Ctx) error {
	var patch service.ConfigPatch
	if err := c.Bind().JSON(&patch); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}

	view, err := h.config.Patch(c.Params("name"), patch)
	if err != nil {
		return configWriteError(c, err)
	}
	return c.JSON(view)
}

// ResetConfig restores the whole config.json to the embedded template default.
// This is destructive: providers and channels are reset too.
// @Summary      Reset agent config to defaults
// @Tags         Config
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} service.ConfigView
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/config/reset [post]
func (h *Handler) ResetConfig(c fiber.Ctx) error {
	view, err := h.config.ResetConfig(c.Params("name"))
	if err != nil {
		return configWriteError(c, err)
	}
	return c.JSON(view)
}

// ResetConfigFields restores only the requested sections and bot fields.
// @Summary      Reset specific config fields to defaults
// @Tags         Config
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body service.ConfigResetRequest true "Fields to reset"
// @Success      200 {object} service.ConfigView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/config/reset [patch]
func (h *Handler) ResetConfigFields(c fiber.Ctx) error {
	var req service.ConfigResetRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}

	view, err := h.config.ResetFields(c.Params("name"), req)
	if err != nil {
		return configWriteError(c, err)
	}
	return c.JSON(view)
}
