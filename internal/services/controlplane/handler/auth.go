package handler

import (
	"errors"
	"time"

	"myaaw/internal/services/controlplane/middleware"
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// Login authenticates the single control-plane user.
// @Summary      Login
// @Description  Authenticate with username and password and receive a bearer token.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body LoginRequest true "Credentials"
// @Success      200 {object} LoginResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /auth/login [post]
func (h *Handler) Login(c fiber.Ctx) error {
	var req LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid JSON request body",
		})
	}

	token, expiresAt, err := h.auth.Login(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid username or password",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"token":      token,
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
}

// Me returns the authenticated user.
// @Summary      Current user
// @Tags         Auth
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} UsernameResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /auth/me [get]
func (h *Handler) Me(c fiber.Ctx) error {
	return c.JSON(fiber.Map{"username": h.auth.Username()})
}

// Logout invalidates the current bearer token.
// @Summary      Logout
// @Tags         Auth
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /auth/logout [post]
func (h *Handler) Logout(c fiber.Ctx) error {
	h.auth.Logout(middleware.BearerToken(c))
	return c.JSON(fiber.Map{"ok": true})
}

// ChangePassword changes the control-plane password and invalidates sessions.
// @Summary      Change password
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body ChangePasswordRequest true "Passwords"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /auth/change-password [post]
func (h *Handler) ChangePassword(c fiber.Ctx) error {
	var req ChangePasswordRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid JSON request body",
		})
	}

	if err := h.auth.ChangePassword(req.CurrentPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, service.ErrWeakPassword):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, service.ErrInvalidCredentials):
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid current password"})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}

	return c.JSON(fiber.Map{"ok": true})
}

// ChangeUsername changes the control-plane username.
// @Summary      Change username
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body ChangeUsernameRequest true "Password and new username"
// @Success      200 {object} LoginOKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /auth/username [put]
func (h *Handler) ChangeUsername(c fiber.Ctx) error {
	var req ChangeUsernameRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid JSON request body",
		})
	}

	if err := h.auth.ChangeUsername(req.Password, req.NewUsername); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid password"})
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
	}

	return c.JSON(fiber.Map{"ok": true, "username": h.auth.Username()})
}
