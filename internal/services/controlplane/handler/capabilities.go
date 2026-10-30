package handler

import (
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// GetCapabilities lists the provider types, embedded skills, and built-in
// tools an agent can be created with. The catalog is global because a
// not-yet-created agent has no skills or tools directory to read.
// @Summary      Onboarding capabilities
// @Tags         Agents
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} service.CapabilitiesView
// @Failure      401 {object} common.ErrorResponse
// @Router       /capabilities [get]
func (h *Handler) GetCapabilities(c fiber.Ctx) error {
	return c.JSON(service.BuiltinCapabilities())
}
