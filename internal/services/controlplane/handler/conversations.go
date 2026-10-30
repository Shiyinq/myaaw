package handler

import (
	"errors"
	"strconv"
	"strings"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListConversations returns a page of conversations for an agent.
// @Summary      List conversations
// @Description  Conversations are scoped to the authenticated user unless user_id is provided. search matches the title only.
// @Tags         Conversations
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        user_id query int false "User ID (defaults to the authenticated user)"
// @Param        search query string false "Case-insensitive title search"
// @Param        limit query int false "Page size (default 50, max 200)"
// @Param        offset query int false "Offset (default 0)"
// @Success      200 {object} service.ConversationPage
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations [get]
func (h *Handler) ListConversations(c fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	search := strings.TrimSpace(c.Query("search"))

	page, err := h.conversations.List(c.Params("name"), h.requestUserID(c), limit, offset, search)
	if err != nil {
		return conversationError(c, err)
	}
	return c.JSON(page)
}

// GetConversation returns one conversation with a page of its transcript.
// @Summary      Get conversation
// @Description  Returns 403 when the conversation belongs to another user.
// @Tags         Conversations
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        user_id query int false "User ID (defaults to the authenticated user)"
// @Param        limit query int false "Message page size (0 = all, max 500)"
// @Param        offset query int false "Message offset (default 0)"
// @Success      200 {object} service.ConversationDetail
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id} [get]
func (h *Handler) GetConversation(c fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))

	detail, err := h.conversations.Get(c.Params("name"), c.Params("id"), h.requestUserID(c), limit, offset)
	if err != nil {
		return conversationError(c, err)
	}
	return c.JSON(detail)
}

// RenameConversation updates a conversation title.
// @Summary      Rename conversation
// @Description  Only the owner can rename a conversation. Titles are trimmed and must not be empty.
// @Tags         Conversations
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        user_id query int false "User ID (defaults to the authenticated user)"
// @Param        request body RenameConversationRequest true "New title"
// @Success      200 {object} RenameConversationResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id} [patch]
func (h *Handler) RenameConversation(c fiber.Ctx) error {
	var req RenameConversationRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid JSON request body",
		})
	}

	id := c.Params("id")
	title, err := h.conversations.Rename(c.Params("name"), id, h.requestUserID(c), req.Title)
	if err != nil {
		return conversationError(c, err)
	}
	return c.JSON(fiber.Map{
		"ok":    true,
		"id":    id,
		"title": title,
	})
}

// DeleteConversation removes a conversation.
// @Summary      Delete conversation
// @Description  Only the owner can delete a conversation.
// @Tags         Conversations
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        user_id query int false "User ID (defaults to the authenticated user)"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id} [delete]
func (h *Handler) DeleteConversation(c fiber.Ctx) error {
	if err := h.conversations.Delete(c.Params("name"), c.Params("id"), h.requestUserID(c)); err != nil {
		return conversationError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// requestUserID resolves the conversation owner. An explicit user_id query
// parameter overrides the authenticated user; otherwise the login's user ID
// (0 by default) is used.
func (h *Handler) requestUserID(c fiber.Ctx) int {
	userID := h.auth.UserID()
	raw := strings.TrimSpace(c.Query("user_id"))
	if raw == "" {
		return userID
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		return userID
	}
	return parsed
}

func conversationError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrConversationNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
	case errors.Is(err, service.ErrConversationForbidden):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "conversation belongs to another user"})
	case errors.Is(err, service.ErrInvalidTitle):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
