package handler

import (
	"errors"
	"strings"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// GetApprovals returns the effective approval policy and the current grants.
// @Summary      Get approvals
// @Description  Effective policy (config.json merged with defaults) plus the runtime grants from config/approvals.json. Read-only.
// @Tags         Approvals
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} service.ApprovalsView
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/approvals [get]
func (h *Handler) GetApprovals(c fiber.Ctx) error {
	view, err := h.approvals.View(c.Params("name"))
	if err != nil {
		return approvalError(c, err)
	}
	return c.JSON(view)
}

// RevokeApprovalGrant removes grants for one tool key. With ?channel= and/or
// ?user= only that actor's always grants and sessions are removed.
// @Summary      Revoke an approval grant
// @Description  Removes the key from config/approvals.json (all scopes), or only the scopes matching the optional channel/user filters.
// @Tags         Approvals
// @Produce      json
// @Security     BearerAuth
// @Param        name    path string true "Agent name"
// @Param        key     path string true "Approval key (tool or tool:action)"
// @Param        channel query string false "Only revoke grants for this channel"
// @Param        user    query string false "Only revoke grants for this user id"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/approvals/grants/{key} [delete]
func (h *Handler) RevokeApprovalGrant(c fiber.Ctx) error {
	name, key := c.Params("name"), c.Params("key")
	channel := strings.TrimSpace(c.Query("channel"))
	userID := strings.TrimSpace(c.Query("user"))
	if channel != "" || userID != "" {
		if err := h.approvals.RevokeScope(name, key, channel, userID); err != nil {
			return approvalError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	}
	if err := h.approvals.RevokeKey(name, key); err != nil {
		return approvalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// RevokeApprovalGrantSession removes one conversation grant for a tool key.
// @Summary      Revoke a session grant
// @Description  Removes only the given conversation from the key's session grants.
// @Tags         Approvals
// @Produce      json
// @Security     BearerAuth
// @Param        name         path string true "Agent name"
// @Param        key          path string true "Approval key"
// @Param        conversation path string true "Conversation id"
// @Param        channel      query string false "Restrict to this channel"
// @Param        user         query string false "Restrict to this user id"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/approvals/grants/{key}/sessions/{conversation} [delete]
func (h *Handler) RevokeApprovalGrantSession(c fiber.Ctx) error {
	if err := h.approvals.RevokeSession(
		c.Params("name"),
		c.Params("key"),
		c.Params("conversation"),
		strings.TrimSpace(c.Query("channel")),
		strings.TrimSpace(c.Query("user")),
	); err != nil {
		return approvalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ClearApprovalGrants removes every grant for the agent.
// @Summary      Clear approval grants
// @Description  Empties config/approvals.json. The policy in config.json is untouched.
// @Tags         Approvals
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/approvals/grants [delete]
func (h *Handler) ClearApprovalGrants(c fiber.Ctx) error {
	if err := h.approvals.Clear(c.Params("name")); err != nil {
		return approvalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func approvalError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrGrantNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
