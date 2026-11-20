package handler

import (
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// GetSettings returns the synced dashboard settings from controlplane.json.
// @Summary      Get dashboard settings
// @Description  Returns the UI settings stored under the "settings" key of controlplane.json.
// @Tags         Settings
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} service.Settings
// @Failure      401 {object} common.ErrorResponse
// @Router       /settings [get]
func (h *Handler) GetSettings(c fiber.Ctx) error {
	return c.JSON(h.auth.Settings())
}

// UpdateSettings merges a partial update into the synced dashboard settings.
// @Summary      Update dashboard settings
// @Description  Merges changed fields into the "settings" key of controlplane.json and returns the normalized settings.
// @Tags         Settings
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body service.SettingsPatch true "Settings patch"
// @Success      200 {object} service.Settings
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      500 {object} common.ErrorResponse
// @Router       /settings [patch]
func (h *Handler) UpdateSettings(c fiber.Ctx) error {
	var patch service.SettingsPatch
	if err := c.Bind().JSON(&patch); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid JSON request body",
		})
	}

	updated, err := h.auth.UpdateSettings(patch)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(updated)
}
