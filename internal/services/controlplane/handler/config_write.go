package handler

import (
	"errors"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// CreateProvider adds a provider to an agent.
// @Summary      Create provider
// @Tags         Providers
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body CreateProviderRequest true "Provider"
// @Success      201 {object} service.ProviderView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/providers [post]
func (h *Handler) CreateProvider(c fiber.Ctx) error {
	var req CreateProviderRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}

	view, err := h.config.CreateProvider(c.Params("name"), service.ProviderInput{
		Name:         req.Name,
		Type:         req.Type,
		BaseURL:      req.BaseURL,
		APIKey:       req.APIKey,
		DefaultModel: req.DefaultModel,
	})
	if err != nil {
		return configWriteError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(view)
}

// UpdateProvider updates provider fields. An empty api_key keeps the current one.
// @Summary      Update provider
// @Tags         Providers
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Provider name"
// @Param        request body UpdateProviderRequest true "Fields to update"
// @Success      200 {object} service.ProviderView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/providers/{id} [put]
func (h *Handler) UpdateProvider(c fiber.Ctx) error {
	var req UpdateProviderRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}

	view, err := h.config.UpdateProvider(c.Params("name"), c.Params("id"), service.ProviderPatch{
		Type:         req.Type,
		BaseURL:      req.BaseURL,
		APIKey:       req.APIKey,
		DefaultModel: req.DefaultModel,
	})
	if err != nil {
		return configWriteError(c, err)
	}
	return c.JSON(view)
}

// DeleteProvider removes a provider.
// @Summary      Delete provider
// @Tags         Providers
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Provider name"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/providers/{id} [delete]
func (h *Handler) DeleteProvider(c fiber.Ctx) error {
	if err := h.config.DeleteProvider(c.Params("name"), c.Params("id")); err != nil {
		return configWriteError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// SetDefaultProvider sets the agent's default provider.
// @Summary      Set default provider
// @Tags         Providers
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body SetDefaultProviderRequest true "Provider name"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/providers/default [post]
func (h *Handler) SetDefaultProvider(c fiber.Ctx) error {
	var req SetDefaultProviderRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	if err := h.config.SetDefaultProvider(c.Params("name"), req.Name); err != nil {
		return configWriteError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// UpsertChannel creates or updates a channel. An empty token keeps the current one.
// @Summary      Create or update channel
// @Tags         Channels
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        channel path string true "telegram or discord"
// @Param        request body ChannelRequest true "Channel settings"
// @Success      200 {object} service.ChannelView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/channels/{channel} [put]
func (h *Handler) UpsertChannel(c fiber.Ctx) error {
	var req ChannelRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}

	view, err := h.config.UpsertChannel(c.Params("name"), c.Params("channel"), service.ChannelInput{
		Active:         req.Active,
		Token:          req.Token,
		Mode:           req.Mode,
		NgrokActive:    req.NgrokActive,
		NgrokAuthToken: req.NgrokAuthToken,
	})
	if err != nil {
		return configWriteError(c, err)
	}
	return c.JSON(view)
}

// DeleteChannel disables a channel.
// @Summary      Disable channel
// @Tags         Channels
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        channel path string true "telegram or discord"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/channels/{channel} [delete]
func (h *Handler) DeleteChannel(c fiber.Ctx) error {
	if err := h.config.DisableChannel(c.Params("name"), c.Params("channel")); err != nil {
		return configWriteError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func configWriteError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrProviderNotFound), errors.Is(err, service.ErrChannelNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrAgentNotFound), errors.Is(err, service.ErrConfigNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidConfig):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
